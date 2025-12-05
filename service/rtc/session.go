// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"

	"github.com/mattermost/rtcd/service/rtc/dc"
	"github.com/mattermost/rtcd/service/rtc/vad"

	"github.com/pion/interceptor/pkg/cc"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"

	"github.com/mattermost/mattermost/server/public/shared/mlog"
)

const (
	signalChSize         = 20
	tracksChSize         = 100
	signalingLockTimeout = 5 * time.Second
)

// offerMessage is a wrapper struct to tie offers to a given answerCh
// This channel could be backed by either WebSocket or DataChannel
type offerMessage struct {
	sdp      webrtc.SessionDescription
	answerCh chan<- Message
}

type dcMessage struct {
	msgType dc.MessageType
	payload any
}

// session contains all the state necessary to connect a user to a call.
type session struct {
	cfg SessionConfig

	// WebRTC
	rtcConn        *webrtc.PeerConnection
	tracksCh       chan trackActionContext
	iceInCh        chan []byte
	sdpOfferInCh   chan offerMessage
	sdpAnswerInCh  chan webrtc.SessionDescription
	dcSDPCh        chan Message
	dcOutCh        chan dcMessage
	dcOpenCh       chan struct{}
	signalingLock  *dc.Lock
	startLockTime  atomic.Pointer[time.Time]
	tracksInitDone atomic.Bool

	// Sender (publishing side)
	outVoiceTrack        *webrtc.TrackLocalStaticRTP
	outVoiceTrackEnabled bool
	remoteVoiceTrack     *webrtc.TrackRemote
	// Screen sharing
	screenStreamID         string
	outScreenTracks        map[string][]*webrtc.TrackLocalStaticRTP
	outScreenAudioTrack    *webrtc.TrackLocalStaticRTP
	remoteScreenAudioTrack *webrtc.TrackRemote
	remoteScreenTracks     map[string]*webrtc.TrackRemote
	screenRateMonitors     map[string]*RateMonitor
	// Video
	videoStreamID     string
	outVideoTracks    map[string][]*webrtc.TrackLocalStaticRTP
	remoteVideoTracks map[string]*webrtc.TrackRemote
	videoRateMonitors map[string]*RateMonitor

	// Receiver
	bwEstimator       cc.BandwidthEstimator
	screenTrackSender *webrtc.RTPSender
	rxTracks          map[string]webrtc.TrackLocal

	closeCh chan struct{}
	closeCb func() error
	doneCh  chan struct{}

	vadMonitor *vad.Monitor

	makingOffer bool

	log  mlog.LoggerIFace
	call *call

	mut sync.RWMutex
}

func (s *Server) addSession(cfg SessionConfig, peerConn *webrtc.PeerConnection, closeCb func() error, sessionLog mlog.LoggerIFace) (*session, error) {
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
		// call is missing, creating one - extract callID from sessionLog
		callLog := loggerWith(s.log, mlog.String("callID", cfg.CallID))
		c = &call{
			id:          cfg.CallID,
			sessions:    map[string]*session{},
			pliLimiters: map[webrtc.SSRC]*rate.Limiter{},
			metrics:     s.metrics,
			log:         callLog,
		}
		g.calls[c.id] = c
	}
	g.mut.Unlock()

	us, ok := c.addSession(cfg, peerConn, closeCb, sessionLog)
	if !ok {
		return nil, fmt.Errorf("user session already exists")
	}
	s.mut.Lock()
	s.sessions[cfg.SessionID] = cfg
	s.mut.Unlock()

	return us, nil
}

func (s *Server) handleDCNegotiation(us *session, call *call) {
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

	start := time.Now()

	// Wait for DC to be open before doing more signaling (e.g. sending out tracks). This ensures
	// the DC can be used to synchronize any further signaling through dc.Lock
	select {
	case <-us.dcOpenCh:
		s.metrics.ObserveRTCDataChannelOpenTime(us.cfg.GroupID, time.Since(start).Seconds())
		us.log.Debug("DC is open, starting to handle tracks")
		s.handleTracks(call, us)
	case <-us.closeCh:
	}

	<-iceDoneCh
}

func (s *session) getScreenStreamID() string {
	s.mut.RLock()
	defer s.mut.RUnlock()
	return s.screenStreamID
}

func (s *session) getVideoStreamID() string {
	s.mut.RLock()
	defer s.mut.RUnlock()
	return s.videoStreamID
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
				s.log.Error("failed to encode ice candidate", mlog.Err(err))
				continue
			}

			if candidate.Candidate == "" {
				continue
			}

			s.log.Debug("setting ICE candidate for remote")

			if err := s.rtcConn.AddICECandidate(candidate); err != nil {
				s.log.Error("failed to add ice candidate", mlog.Err(err))
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
				s.log.Error("failed to read RTCP packet", mlog.Err(err))
			}
			return
		}
	}
}

// handleSenderRTCP is used to listen for for RTCP packets such as PLI (Picture Loss Indication)
// from a peer receiving a video track (e.g. screen).
func (s *session) handleSenderRTCP(sender *webrtc.RTPSender, remoteTrack *webrtc.TrackRemote, senderSession *session) {
	for {
		pkts, _, err := sender.ReadRTCP()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				s.log.Error("failed to read RTCP packet", mlog.Err(err))
			}
			return
		}
		for _, pkt := range pkts {
			if p, ok := pkt.(*rtcp.PictureLossIndication); ok {
				// When a PLI is received the request is forwarded
				// to the peer generating the track (e.g. presenter).

				for _, dstSSRC := range p.DestinationSSRC() {
					s.log.Debug("received PLI request for track", mlog.Uint("SSRC", dstSSRC))
				}

				s.call.mut.Lock()
				// We allow at most one PLI request per second for a given SSRC to avoid overloading the sender.
				// If a receiving client were to miss it due to rate limiting (e.g. joining right in the second of backoff),
				// it will request it again and eventually get it.
				limiter, ok := s.call.pliLimiters[remoteTrack.SSRC()]
				if !ok {
					s.log.Debug("creating new PLI limiter for track", mlog.Uint("SSRC", remoteTrack.SSRC()))
					limiter = rate.NewLimiter(1, 1)
					s.call.pliLimiters[remoteTrack.SSRC()] = limiter
				}
				s.call.mut.Unlock()

				if limiter.Allow() {
					s.log.Debug("forwarding PLI request for track", mlog.Uint("SSRC", remoteTrack.SSRC()))
					if err := senderSession.rtcConn.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: uint32(remoteTrack.SSRC())}}); err != nil {
						s.log.Error("failed to write RTCP packet", mlog.Err(err))
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

	if err := s.sendMediaMapping(); err != nil {
		s.log.Error("failed to send media mapping", mlog.Err(err), mlog.String("sessionID", s.cfg.SessionID))
	}

	select {
	case sdpOutCh <- newMessage(s, SDPMessage, sdp):
		return nil
	default:
		return fmt.Errorf("failed to send SDP message: channel is full")
	}
}

// addTrack adds the given track to the peer and starts negotiation.
func (s *session) addTrack(sdpOutCh chan<- Message, localTrack webrtc.TrackLocal, remoteTrack *webrtc.TrackRemote, senderSession *session) (errRet error) {
	if localTrack == nil {
		return fmt.Errorf("invalid nil localTrack")
	}

	if remoteTrack == nil {
		return fmt.Errorf("invalid nil remoteTrack")
	}

	if senderSession == nil {
		return fmt.Errorf("invalid nil senderSession")
	}

	s.log.Debug("addTrack", mlog.String("trackID", localTrack.ID()))

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
		if sender.Track() == localTrack {
			s.mut.Unlock()
			return fmt.Errorf("sender for track already exists")
		}
	}

	if getTrackType(localTrack.ID()) == trackTypeScreen && s.screenTrackSender != nil {
		s.mut.Unlock()
		return fmt.Errorf("screen track sender is already set")
	}

	sender, err := s.rtcConn.AddTrack(localTrack)
	if err != nil {
		s.mut.Unlock()
		return fmt.Errorf("failed to add track %s: %w", localTrack.ID(), err)
	}
	s.call.metrics.IncRTPTracks(s.cfg.GroupID, "out", getTrackMimeType(localTrack))
	s.mut.Unlock()

	defer func() {
		if errRet == nil {
			return
		}

		s.mut.Lock()
		if err := sender.ReplaceTrack(nil); err != nil {
			s.log.Error("failed to replace track",
				mlog.String("trackID", localTrack.ID()))
		} else {
			s.call.metrics.DecRTPTracks(s.cfg.GroupID, "out", getTrackMimeType(localTrack))
			delete(s.rxTracks, localTrack.ID())
		}
		s.mut.Unlock()
	}()

	go s.handleSenderRTCP(sender, remoteTrack, senderSession)

	if err := s.sendOffer(sdpOutCh); err != nil {
		return fmt.Errorf("failed to send offer for track %s: %w", localTrack.ID(), err)
	}

	select {
	case answer, ok := <-s.sdpAnswerInCh:
		if !ok {
			return nil
		}
		if err := s.rtcConn.SetRemoteDescription(answer); err != nil {
			return fmt.Errorf("failed to set remote description for track %s: %w", localTrack.ID(), err)
		}

		s.mut.Lock()
		if getTrackType(localTrack.ID()) == trackTypeScreen {
			s.screenTrackSender = sender
		}
		s.rxTracks[localTrack.ID()] = localTrack
		s.mut.Unlock()
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

	s.log.Debug("removeTrack", mlog.String("trackID", track.ID()))

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
	s.call.metrics.DecRTPTracks(s.cfg.GroupID, "out", getTrackMimeType(track))
	delete(s.rxTracks, track.ID())
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
		s.log.Debug("vad", mlog.Bool("voice", voice))

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

func (s *session) sendMediaMapping() error {
	mediaMap := dc.MediaMap{}

	for _, trx := range s.rtcConn.GetTransceivers() {
		if trx.Sender() == nil {
			continue
		}
		track := trx.Sender().Track()
		if track == nil {
			s.log.Warn("track is nil", mlog.String("sessionID", s.cfg.SessionID))
			continue
		}
		trackID := track.ID()
		trackType := getTrackType(trackID)
		if trackType == "" {
			s.log.Warn("track type is empty", mlog.String("sessionID", s.cfg.SessionID), mlog.String("trackID", trackID))
			continue
		}
		mediaMap[trx.Mid()] = dc.TrackInfo{
			Type:     string(trackType),
			SenderID: s.cfg.SessionID,
		}
	}

	select {
	case s.dcOutCh <- dcMessage{msgType: dc.MessageTypeMediaMap, payload: mediaMap}:
	default:
		return fmt.Errorf("failed to send MediaMap message: channel is full")
	}

	return nil
}
