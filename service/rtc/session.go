// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"

	"github.com/mattermost/mattermost-server/v6/shared/mlog"
)

const (
	signalChSize = 20
	tracksChSize = 10
)

// session contains all the state necessary to connect a user to a call.
type session struct {
	cfg SessionConfig

	// WebRTC
	screenStreamID       string
	outVoiceTrack        *webrtc.TrackLocalStaticRTP
	outVoiceTrackEnabled bool
	outScreenTrack       *webrtc.TrackLocalStaticRTP
	outScreenAudioTrack  *webrtc.TrackLocalStaticRTP
	remoteScreenTrack    *webrtc.TrackRemote
	rtcConn              *webrtc.PeerConnection
	tracksCh             chan *webrtc.TrackLocalStaticRTP
	iceInCh              chan []byte
	sdpInCh              chan []byte

	closeCh chan struct{}
	closeCb func() error

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
			id:       cfg.CallID,
			sessions: map[string]*session{},
		}
		g.calls[c.id] = c
	}
	g.mut.Unlock()

	us, ok := c.addSession(cfg, peerConn, closeCb)
	if !ok {
		return nil, fmt.Errorf("user session already exists")
	}
	s.mut.Lock()
	s.sessions[cfg.SessionID] = cfg
	s.mut.Unlock()

	return us, nil
}

func (s *session) getScreenStreamID() string {
	s.mut.RLock()
	defer s.mut.RUnlock()
	return s.screenStreamID
}

func (s *session) getRemoteScreenTrack() *webrtc.TrackRemote {
	s.mut.RLock()
	defer s.mut.RUnlock()
	return s.remoteScreenTrack
}

// handleICE deals with trickle ICE candidates.
func (s *session) handleICE(log mlog.LoggerIFace, m Metrics) {
	for {
		select {
		case data, ok := <-s.iceInCh:
			if !ok {
				return
			}

			var candidate webrtc.ICECandidateInit
			if err := json.Unmarshal(data, &candidate); err != nil {
				log.Error("failed to encode ice candidate", mlog.Err(err), mlog.String("sessionID", s.cfg.SessionID))
				continue
			}

			if candidate.Candidate == "" {
				continue
			}

			log.Debug("setting ICE candidate for remote", mlog.String("sessionID", s.cfg.SessionID))

			if err := s.rtcConn.AddICECandidate(candidate); err != nil {
				log.Error("failed to add ice candidate", mlog.Err(err), mlog.String("sessionID", s.cfg.SessionID))
				m.IncRTCErrors(s.cfg.GroupID, "ice")
				continue
			}
		case <-s.closeCh:
			return
		}
	}
}

// handlePLI is used to listen for for PLI (Picture Loss Indication) packet requests
// from a peer receiving a video track (e.g. screen). When one is received
// the request is forwarded to the peer generating the track (e.g. presenter).
func (s *session) handlePLI(log mlog.LoggerIFace, call *call, sender *webrtc.RTPSender) {
	for {
		pkts, _, err := sender.ReadRTCP()
		if err != nil {
			log.Error("failed to read RTCP packet",
				mlog.Err(err), mlog.String("sessionID", s.cfg.SessionID))
			return
		}
		for _, pkt := range pkts {
			if _, ok := pkt.(*rtcp.PictureLossIndication); ok {
				screenSession := call.getScreenSession()
				if screenSession == nil {
					log.Error("screenSession should not be nil", mlog.String("sessionID", s.cfg.SessionID))
					return
				}

				screenTrack := screenSession.getRemoteScreenTrack()
				if screenTrack == nil {
					log.Error("screenTrack should not be nil", mlog.String("sessionID", s.cfg.SessionID))
					return
				}

				if err := screenSession.rtcConn.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: uint32(screenTrack.SSRC())}}); err != nil {
					log.Error("failed to write RTCP packet", mlog.Err(err), mlog.String("sessionID", s.cfg.SessionID))
					return
				}
			}
		}
	}
}

// addTrack adds the given track to the peer and starts negotiation.
func (s *session) addTrack(log mlog.LoggerIFace, c *call, sdpOutCh chan<- Message, track *webrtc.TrackLocalStaticRTP) error {
	sender, err := s.rtcConn.AddTrack(track)
	if err != nil {
		return fmt.Errorf("failed to add track: %w", err)
	} else if track.Kind() == webrtc.RTPCodecTypeVideo {
		go s.handlePLI(log, c, sender)
	}

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
	default:
		return fmt.Errorf("failed to send SDP message: channel is full")
	}

	var answer webrtc.SessionDescription
	select {
	case msg, ok := <-s.sdpInCh:
		if !ok {
			return nil
		}
		if err := json.Unmarshal(msg, &answer); err != nil {
			return fmt.Errorf("failed to unmarshal answer: %w", err)
		}
	case <-time.After(signalingTimeout):
		return fmt.Errorf("timed out signaling")
	}

	if err := s.rtcConn.SetRemoteDescription(answer); err != nil {
		return fmt.Errorf("failed to set remote description: %w", err)
	}

	return nil
}

// signaling handles incoming SDP offers.
func (s *session) signaling(msg []byte) ([]byte, error) {
	var offer webrtc.SessionDescription
	if err := json.Unmarshal(msg, &offer); err != nil {
		return nil, err
	}

	if err := s.rtcConn.SetRemoteDescription(offer); err != nil {
		return nil, err
	}

	answer, err := s.rtcConn.CreateAnswer(nil)
	if err != nil {
		return nil, err
	}

	if err := s.rtcConn.SetLocalDescription(answer); err != nil {
		return nil, err
	}

	sdp, err := json.Marshal(s.rtcConn.LocalDescription())
	if err != nil {
		return nil, err
	}

	return sdp, nil
}
