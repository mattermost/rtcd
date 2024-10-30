// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/mattermost/rtcd/service/rtc/vad"

	"github.com/pion/interceptor/pkg/cc"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"

	"github.com/mattermost/mattermost/server/public/shared/mlog"
)

const (
	signalChSize = 20
	tracksChSize = 100
)

// offerMessage is a wrapper struct to tie offers to a given answerCh
// This channel could be backed by either WebSocket or DataChannel
type offerMessage struct {
	sdp      webrtc.SessionDescription
	answerCh chan<- Message
}

// session contains all the state necessary to connect a user to a call.
type session struct {
	cfg SessionConfig

	// WebRTC
	rtcConn       *webrtc.PeerConnection
	tracksCh      chan trackActionContext
	iceInCh       chan []byte
	sdpOfferInCh  chan offerMessage
	sdpAnswerInCh chan webrtc.SessionDescription
	dcSDPCh       chan Message

	// Sender (publishing side)
	outVoiceTrack        *webrtc.TrackLocalStaticRTP
	outVoiceTrackEnabled bool
	screenStreamID       string
	outScreenTracks      map[string][]*webrtc.TrackLocalStaticRTP
	outScreenAudioTrack  *webrtc.TrackLocalStaticRTP
	remoteScreenTracks   map[string]*webrtc.TrackRemote
	screenRateMonitors   map[string]*RateMonitor

	// Receiver
	bwEstimator       cc.BandwidthEstimator
	screenTrackSender *webrtc.RTPSender

	closeCh chan struct{}
	closeCb func() error
	doneCh  chan struct{}

	vadMonitor *vad.Monitor

	makingOffer bool

	log  mlog.LoggerIFace
	call *call

	mut sync.RWMutex
}

func (s *Server) addSession(cfg SessionConfig, peerConn *webrtc.PeerConnection, closeCb func() error) (*session, error) {
	if err := cfg.IsValid(); err != nil {
		return nil, err
	}

	if peerConn == nil {
		return nil, fmt.Errorf("peerConn should not be nil")
	}

	s.mut.Lock()
	g := s.groups[cfg.GroupID]
	if g == nil {
		// group is missing, creating one
		g = &group{
			id:    cfg.GroupID,
			calls: map[string]*call{},
		}
		s.groups[g.id] = g
	}
	s.mut.Unlock()

	g.mut.Lock()
	c := g.calls[cfg.CallID]
	if c == nil {
		// call is missing, creating one
		c = &call{
			id:          cfg.CallID,
			sessions:    map[string]*session{},
			pliLimiters: map[webrtc.SSRC]*rate.Limiter{},
			metrics:     s.metrics,
		}
		g.calls[c.id] = c
	}
	g.mut.Unlock()

	us, ok := c.addSession(cfg, peerConn, closeCb, s.log)
	if !ok {
		return nil, fmt.Errorf("user session already exists")
	}
	s.mut.Lock()
	s.sessions[cfg.SessionID] = cfg
	s.mut.Unlock()

	return us, nil
}

func (s *Server) handleNegotiations(us *session, call *call) {
	defer func() {
		// Only close channel if not already closed. This can happen in case of a failure
		// during the initial negotiation (e.g. timeout).
		select {
		case <-us.doneCh:
			return
		default:
			close(us.doneCh)
		}
	}()

	select {
	case offerMsg, ok := <-us.sdpOfferInCh:
		if !ok {
			return
		}
		if err := us.signaling(offerMsg.sdp, offerMsg.answerCh); err != nil {
			s.metrics.IncRTCErrors(us.cfg.GroupID, "signaling")
			s.log.Error("failed to signal", mlog.Err(err), mlog.Any("sessionCfg", us.cfg))

			// We need to preemptively close doneCh to avoid CloseSession from blocking indefinitely on it.
			close(us.doneCh)
			if err := s.CloseSession(us.cfg.SessionID); err != nil {
				s.log.Error("failed to close session", mlog.Any("sessionCfg", us.cfg))
			}

			return
		}
	case <-time.After(signalingTimeout):
		s.log.Error("timed out signaling", mlog.Any("sessionCfg", us.cfg))
		s.metrics.IncRTCErrors(us.cfg.GroupID, "signaling")

		// We need to preemptively close doneCh to avoid CloseSession from blocking indefinitely on it.
		close(us.doneCh)
		if err := s.CloseSession(us.cfg.SessionID); err != nil {
			s.log.Error("failed to close session", mlog.Any("sessionCfg", us.cfg))
		}

		return
	case <-us.closeCh:
		s.log.Debug("closeCh closed during signaling", mlog.Any("sessionCfg", us.cfg))
		return
	}

	iceDoneCh := make(chan struct{})
	go func() {
		defer close(iceDoneCh)
		us.handleICE(s.metrics)
	}()

	s.handleTracks(call, us)

	<-iceDoneCh
}

func (s *session) getScreenStreamID() string {
	s.mut.RLock()
	defer s.mut.RUnlock()
	return s.screenStreamID
}

func (s *session) getRemoteScreenTrack(mimeType, rid string) *webrtc.TrackRemote {
	s.mut.RLock()
	defer s.mut.RUnlock()

	if rid == "" {
		rid = SimulcastLevelDefault
	}

	return s.remoteScreenTracks[getTrackIndex(mimeType, rid)]
}

func (s *session) getSourceRate(mimeType, rid string) int {
	s.mut.RLock()
	defer s.mut.RUnlock()

	if rid == "" {
		rid = SimulcastLevelDefault
	}

	rm := s.screenRateMonitors[getTrackIndex(mimeType, rid)]

	if rm == nil {
		s.log.Warn("rate monitor should not be nil", mlog.String("sessionID", s.cfg.SessionID))
		return -1
	}

	rate, _ := rm.GetRate()

	return rate
}

func (s *session) getOutScreenTrack(mimeType, rid string) *webrtc.TrackLocalStaticRTP {
	s.mut.RLock()
	defer s.mut.RUnlock()

	return pickRandom(s.outScreenTracks[getTrackIndex(mimeType, rid)])
}

func (s *session) getExpectedSimulcastLevel() string {
	s.mut.RLock()
	defer s.mut.RUnlock()

	if s.bwEstimator == nil {
		return SimulcastLevelDefault
	}

	return getSimulcastLevelForRate(s.bwEstimator.GetTargetBitrate())
}

// handleICE deals with trickle ICE candidates.
func (s *session) handleICE(m Metrics) {
	for {
		select {
		case data, ok := <-s.iceInCh:
			if !ok {
				return
			}

			var candidate webrtc.ICECandidateInit
			if err := json.Unmarshal(data, &candidate); err != nil {
				s.log.Error("failed to encode ice candidate", mlog.Err(err), mlog.String("sessionID", s.cfg.SessionID))
				continue
			}

			if candidate.Candidate == "" {
				continue
			}

			s.log.Debug("setting ICE candidate for remote", mlog.String("sessionID", s.cfg.SessionID))

			if err := s.rtcConn.AddICECandidate(candidate); err != nil {
				s.log.Error("failed to add ice candidate", mlog.Err(err), mlog.String("sessionID", s.cfg.SessionID))
				m.IncRTCErrors(s.cfg.GroupID, "ice")
				continue
			}
		case <-s.closeCh:
			return
		}
	}
}

func (s *session) handleReceiverRTCP(receiver *webrtc.RTPReceiver, rid string) {
	var err error
	for {
		// TODO: consider using a pool to optimize allocations.
		rtcpBuf := make([]byte, receiveMTU)
		if rid != "" {
			_, _, err = receiver.ReadSimulcast(rtcpBuf, rid)
		} else {
			_, _, err = receiver.Read(rtcpBuf)
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				s.log.Error("failed to read RTCP packet",
					mlog.Err(err), mlog.String("sessionID", s.cfg.SessionID))
			}
			return
		}
	}
}

// handleSenderRTCP is used to listen for for RTCP packets such as PLI (Picture Loss Indication)
// from a peer receiving a video track (e.g. screen).
func (s *session) handleSenderRTCP(sender *webrtc.RTPSender) {
	for {
		pkts, _, err := sender.ReadRTCP()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				s.log.Error("failed to read RTCP packet",
					mlog.Err(err), mlog.String("sessionID", s.cfg.SessionID))
			}
			return
		}
		for _, pkt := range pkts {
			if p, ok := pkt.(*rtcp.PictureLossIndication); ok {
				// When a PLI is received the request is forwarded
				// to the peer generating the track (e.g. presenter).

				for _, dstSSRC := range p.DestinationSSRC() {
					s.log.Debug("received PLI request for track", mlog.String("sessionID", s.cfg.SessionID), mlog.Uint("SSRC", dstSSRC))
				}

				screenSession := s.call.getScreenSession()
				if screenSession == nil {
					s.log.Error("screenSession should not be nil", mlog.String("sessionID", s.cfg.SessionID))
					return
				}

				senderTrack, ok := sender.Track().(*webrtc.TrackLocalStaticRTP)
				if !ok {
					s.log.Error("track conversion failed", mlog.String("sessionID", s.cfg.SessionID))
					return
				}

				if senderTrack == nil {
					s.log.Error("senderTrack should not be nil", mlog.String("sessionID", s.cfg.SessionID))
					return
				}

				screenTrack := screenSession.getRemoteScreenTrack(senderTrack.Codec().MimeType, sender.Track().RID())
				if screenTrack == nil {
					s.log.Error("screenTrack should not be nil", mlog.String("sessionID", s.cfg.SessionID))
					return
				}

				s.call.mut.Lock()
				// We allow at most one PLI request per second for a given SSRC to avoid overloading the sender.
				// If a receiving client were to miss it due to rate limiting (e.g. joining right in the second of backoff),
				// it will request it again and eventually get it.
				limiter, ok := s.call.pliLimiters[screenTrack.SSRC()]
				if !ok {
					s.log.Debug("creating new PLI limiter for track", mlog.Uint("SSRC", screenTrack.SSRC()))
					limiter = rate.NewLimiter(1, 1)
					s.call.pliLimiters[screenTrack.SSRC()] = limiter
				}
				s.call.mut.Unlock()

				if limiter.Allow() {
					s.log.Debug("forwarding PLI request for track", mlog.String("sessionID", s.cfg.SessionID), mlog.Uint("SSRC", screenTrack.SSRC()))
					if err := screenSession.rtcConn.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: uint32(screenTrack.SSRC())}}); err != nil {
						s.log.Error("failed to write RTCP packet", mlog.Err(err), mlog.String("sessionID", s.cfg.SessionID))
						return
					}
				}
			}
		}
	}
}

// sendOffer creates and sends out a new SDP offer.
func (s *session) sendOffer(sdpOutCh chan<- Message) error {
	offer, err := s.rtcConn.CreateOffer(nil)
	if err != nil {
		return fmt.Errorf("failed to create offer: %w", err)
	}

	err = s.rtcConn.SetLocalDescription(offer)
	if err != nil {
		return fmt.Errorf("failed to set local description: %w", err)
	}

	sdp, err := json.Marshal(s.rtcConn.LocalDescription())
	if err != nil {
		return fmt.Errorf("failed to marshal sdp: %w", err)
	}

	select {
	case sdpOutCh <- newMessage(s, SDPMessage, sdp):
		return nil
	default:
		return fmt.Errorf("failed to send SDP message: channel is full")
	}
}

// addTrack adds the given track to the peer and starts negotiation.
func (s *session) addTrack(sdpOutCh chan<- Message, track webrtc.TrackLocal) (errRet error) {
	if track == nil {
		return fmt.Errorf("trying to add a nil track")
	}

	s.log.Debug("addTrack", mlog.String("sessionID", s.cfg.SessionID),
		mlog.String("trackID", track.ID()))

	s.mut.Lock()
	s.makingOffer = true
	s.mut.Unlock()
	defer func() {
		s.mut.Lock()
		s.makingOffer = false
		s.mut.Unlock()
	}()

	s.mut.Lock()
	for _, sender := range s.rtcConn.GetSenders() {
		if sender.Track() == track {
			s.mut.Unlock()
			return fmt.Errorf("sender for track already exists")
		}
	}

	if track.Kind() == webrtc.RTPCodecTypeVideo && s.screenTrackSender != nil {
		s.mut.Unlock()
		return fmt.Errorf("screen track sender is already set")
	}

	sender, err := s.rtcConn.AddTrack(track)
	if err != nil {
		s.mut.Unlock()
		return fmt.Errorf("failed to add track %s: %w", track.ID(), err)
	}
	s.call.metrics.IncRTPTracks(s.cfg.GroupID, "out", getTrackType(track.Kind()))
	s.mut.Unlock()

	defer func() {
		if errRet == nil {
			return
		}

		s.mut.Lock()
		if err := sender.ReplaceTrack(nil); err != nil {
			s.log.Error("failed to replace track",
				mlog.String("sessionID", s.cfg.SessionID),
				mlog.String("trackID", track.ID()))
		} else {
			s.call.metrics.DecRTPTracks(s.cfg.GroupID, "out", getTrackType(track.Kind()))
		}
		s.mut.Unlock()
	}()

	go s.handleSenderRTCP(sender)

	if err := s.sendOffer(sdpOutCh); err != nil {
		return fmt.Errorf("failed to send offer for track %s: %w", track.ID(), err)
	}

	select {
	case answer, ok := <-s.sdpAnswerInCh:
		if !ok {
			return nil
		}
		if err := s.rtcConn.SetRemoteDescription(answer); err != nil {
			return fmt.Errorf("failed to set remote description for track %s: %w", track.ID(), err)
		}
		if track.Kind() == webrtc.RTPCodecTypeVideo {
			s.mut.Lock()
			s.screenTrackSender = sender
			s.mut.Unlock()
		}
	case <-time.After(signalingTimeout):
		return fmt.Errorf("timed out signaling")
	case <-s.closeCh:
		s.log.Debug("closeCh closed during signaling", mlog.Any("sessionCfg", s.cfg))
		return nil
	}

	return nil
}

// removeTrack removes the given track to the peer and starts (re)negotiation.
func (s *session) removeTrack(sdpOutCh chan<- Message, track webrtc.TrackLocal) error {
	if track == nil {
		return fmt.Errorf("trying to remove a nil track")
	}

	s.log.Debug("removeTrack", mlog.String("sessionID", s.cfg.SessionID),
		mlog.String("trackID", track.ID()))

	var sender *webrtc.RTPSender

	s.mut.Lock()
	for _, snd := range s.rtcConn.GetSenders() {
		if snd.Track() == track {
			sender = snd
			break
		}
	}

	if sender == nil {
		s.mut.Unlock()
		return fmt.Errorf("failed to find sender for track")
	}

	if err := s.rtcConn.RemoveTrack(sender); err != nil {
		s.mut.Unlock()
		return fmt.Errorf("failed to remove track: %w", err)
	}
	s.call.metrics.DecRTPTracks(s.cfg.GroupID, "out", getTrackType(track.Kind()))

	if s.screenTrackSender == sender {
		s.screenTrackSender = nil
	}
	s.mut.Unlock()

	if err := s.sendOffer(sdpOutCh); err != nil {
		return fmt.Errorf("failed to send offer: %w", err)
	}

	select {
	case answer, ok := <-s.sdpAnswerInCh:
		if !ok {
			return nil
		}
		if err := s.rtcConn.SetRemoteDescription(answer); err != nil {
			return fmt.Errorf("failed to set remote description: %w", err)
		}
	case <-time.After(signalingTimeout):
		return fmt.Errorf("timed out signaling")
	case <-s.closeCh:
		s.log.Debug("closeCh closed during signaling", mlog.Any("sessionCfg", s.cfg))
		return nil
	}

	return nil
}

// signaling handles incoming SDP offers.
func (s *session) signaling(offer webrtc.SessionDescription, answerCh chan<- Message) error {
	if s.hasSignalingConflict() {
		s.log.Debug("signaling conflict detected, ignoring offer", mlog.Any("session", s.cfg))
		return nil
	}

	if err := s.rtcConn.SetRemoteDescription(offer); err != nil {
		return err
	}

	answer, err := s.rtcConn.CreateAnswer(nil)
	if err != nil {
		return err
	}

	if err := s.rtcConn.SetLocalDescription(answer); err != nil {
		return err
	}

	sdp, err := json.Marshal(s.rtcConn.LocalDescription())
	if err != nil {
		return err
	}

	select {
	case answerCh <- newMessage(s, SDPMessage, sdp):
	default:
		return fmt.Errorf("failed to send SDP message: channel is full")
	}

	return nil
}

func (s *session) hasSignalingConflict() bool {
	s.mut.RLock()
	defer s.mut.RUnlock()
	if s.rtcConn == nil {
		return false
	}
	return s.makingOffer || s.rtcConn.SignalingState() != webrtc.SignalingStateStable
}

func (s *session) InitVAD(log mlog.LoggerIFace, msgCh chan<- Message) error {
	monitor, err := vad.NewMonitor((vad.MonitorConfig{}).SetDefaults(), func(voice bool) {
		log.Debug("vad", mlog.Bool("voice", voice), mlog.String("sessionID", s.cfg.SessionID))

		var msgType MessageType
		if voice {
			msgType = VoiceOnMessage
		} else {
			msgType = VoiceOffMessage
		}

		select {
		case msgCh <- newMessage(s, msgType, nil):
		default:
			log.Error("failed to send VAD message: channel is full")
		}
	})

	if err != nil {
		return fmt.Errorf("failed to create vad monitor: %w", err)
	}

	s.mut.Lock()
	s.vadMonitor = monitor
	s.mut.Unlock()

	return nil
}

func (s *session) clearScreenState() {
	s.screenStreamID = ""
	s.outScreenTracks = make(map[string][]*webrtc.TrackLocalStaticRTP)
	s.outScreenAudioTrack = nil
	s.remoteScreenTracks = make(map[string]*webrtc.TrackRemote)
	s.screenRateMonitors = make(map[string]*RateMonitor)
}

func (s *session) supportsAV1() bool {
	if s.cfg.Props == nil {
		return false
	}

	return s.cfg.Props.AV1Support()
}

func (s *session) dcSignaling() bool {
	if s.cfg.Props == nil {
		return false
	}

	return s.cfg.Props.DCSignaling()
}
