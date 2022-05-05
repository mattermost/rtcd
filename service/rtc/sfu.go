// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/mattermost/rtcd/service/random"

	"github.com/mattermost/mattermost-server/v6/shared/mlog"

	"github.com/pion/ice/v2"
	"github.com/pion/interceptor"
	"github.com/pion/interceptor/pkg/nack"
	"github.com/pion/webrtc/v3"
)

var (
	rtpAudioCodec = webrtc.RTPCodecCapability{
		MimeType:     "audio/opus",
		ClockRate:    48000,
		Channels:     2,
		SDPFmtpLine:  "minptime=10;useinbandfec=1",
		RTCPFeedback: nil,
	}
	rtpVideoCodecVP8 = webrtc.RTPCodecCapability{
		MimeType:    "video/VP8",
		ClockRate:   90000,
		Channels:    0,
		SDPFmtpLine: "",
		RTCPFeedback: []webrtc.RTCPFeedback{
			{Type: "goog-remb", Parameter: ""},
			{Type: "ccm", Parameter: "fir"},
			{Type: "nack", Parameter: ""},
			{Type: "nack", Parameter: "pli"},
		},
	}
)

const (
	nackResponderBufferSize = 256
)

func initMediaEngine() (*webrtc.MediaEngine, error) {
	var m webrtc.MediaEngine
	if err := m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: rtpAudioCodec,
		PayloadType:        111,
	}, webrtc.RTPCodecTypeAudio); err != nil {
		return nil, err
	}
	if err := m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: rtpVideoCodecVP8,
		PayloadType:        96,
	}, webrtc.RTPCodecTypeVideo); err != nil {
		return nil, err
	}
	return &m, nil
}

func initInterceptors(m *webrtc.MediaEngine) (*interceptor.Registry, error) {
	var i interceptor.Registry
	generator, err := nack.NewGeneratorInterceptor()
	if err != nil {
		return nil, err
	}

	// NACK
	responder, err := nack.NewResponderInterceptor(nack.ResponderSize(nackResponderBufferSize))
	if err != nil {
		return nil, err
	}
	m.RegisterFeedback(webrtc.RTCPFeedback{Type: "nack"}, webrtc.RTPCodecTypeVideo)
	m.RegisterFeedback(webrtc.RTCPFeedback{Type: "nack", Parameter: "pli"}, webrtc.RTPCodecTypeVideo)
	i.Add(responder)
	i.Add(generator)

	// RTCP Reports
	if err := webrtc.ConfigureRTCPReports(&i); err != nil {
		return nil, err
	}

	return &i, nil
}

func (s *Server) InitSession(cfg SessionConfig, closeCb func() error) error {
	s.metrics.IncRTCSessions(cfg.GroupID, cfg.CallID)

	peerConnConfig := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{},
			},
		},
		SDPSemantics: webrtc.SDPSemanticsUnifiedPlanWithFallback,
	}

	m, err := initMediaEngine()
	if err != nil {
		return fmt.Errorf("failed to init media engine: %w", err)
	}

	i, err := initInterceptors(m)
	if err != nil {
		return fmt.Errorf("failed to init interceptors: %w", err)
	}

	sEngine := webrtc.SettingEngine{}
	sEngine.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)
	sEngine.SetICEUDPMux(s.udpMux)
	if s.cfg.ICEHostOverride != "" {
		hostIP, err := resolveHost(s.cfg.ICEHostOverride, time.Second)
		if err != nil {
			return fmt.Errorf("failed to resolve host: %w", err)
		}
		sEngine.SetNAT1To1IPs([]string{hostIP}, webrtc.ICECandidateTypeHost)
	}

	api := webrtc.NewAPI(webrtc.WithMediaEngine(m), webrtc.WithSettingEngine(sEngine), webrtc.WithInterceptorRegistry(i))
	peerConn, err := api.NewPeerConnection(peerConnConfig)
	if err != nil {
		return fmt.Errorf("failed to create peer connection: %w", err)
	}

	us, err := s.addSession(cfg, peerConn, closeCb)
	if err != nil {
		// TODO: handle case session exists
		return fmt.Errorf("failed to add session: %w", err)
	}
	group := s.getGroup(cfg.GroupID)
	call := group.getCall(cfg.CallID)

	peerConn.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			return
		}
		msg, err := newICEMessage(us, candidate)
		if err != nil {
			s.log.Error("failed to create ICE message", mlog.Err(err))
			return
		}
		select {
		case s.receiveCh <- msg:
		default:
			s.log.Error("failed to send ICE message: channel is full")
		}
	})

	peerConn.OnICEGatheringStateChange(func(state webrtc.ICEGathererState) {
		if state == webrtc.ICEGathererStateComplete {
			s.log.Debug("ice gathering complete")
		}
	})

	peerConn.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		if state == webrtc.PeerConnectionStateConnected {
			s.log.Debug("rtc connected!", mlog.String("sessionID", cfg.SessionID))
			s.metrics.IncRTCConnState("connected")
		} else if state == webrtc.PeerConnectionStateDisconnected {
			s.log.Debug("peer connection disconnected", mlog.String("sessionID", cfg.SessionID))
			s.metrics.IncRTCConnState("disconnected")
		} else if state == webrtc.PeerConnectionStateFailed {
			s.log.Debug("peer connection failed", mlog.String("sessionID", cfg.SessionID))
			s.metrics.IncRTCConnState("failed")
		} else if state == webrtc.PeerConnectionStateClosed {
			s.log.Debug("peer connection closed", mlog.String("sessionID", cfg.SessionID))
			s.metrics.IncRTCConnState("closed")
		}
		switch state {
		case webrtc.PeerConnectionStateClosed, webrtc.PeerConnectionStateFailed:
			if err := s.CloseSession(cfg.SessionID); err != nil {
				s.log.Error("failed to close RTC session", mlog.Err(err), mlog.Any("sessionCfg", cfg))
			}
		}
	})

	peerConn.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		if state == webrtc.ICEConnectionStateDisconnected {
			s.log.Debug("ice disconnected", mlog.String("sessionID", cfg.SessionID))
		} else if state == webrtc.ICEConnectionStateFailed {
			s.log.Debug("ice failed", mlog.String("sessionID", cfg.SessionID))
		} else if state == webrtc.ICEConnectionStateClosed {
			s.log.Debug("ice closed", mlog.String("sessionID", cfg.SessionID))
		}
	})

	peerConn.OnTrack(func(remoteTrack *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		s.log.Debug("Got remote track!!!")
		s.log.Debug(fmt.Sprintf("%+v", remoteTrack.Codec().RTPCodecCapability))
		s.log.Debug(fmt.Sprintf("Track has started, of type %d: %s", remoteTrack.PayloadType(), remoteTrack.Codec().MimeType))

		streamID := remoteTrack.StreamID()

		var screenStreamID string
		if screenSession := call.getScreenSession(); screenSession != nil {
			screenStreamID = screenSession.getScreenStreamID()
		}

		if remoteTrack.Codec().MimeType == rtpAudioCodec.MimeType {
			trackType := "voice"
			if streamID == screenStreamID {
				s.log.Debug("received screen sharing audio track")
				trackType = "screen-audio"
			}

			outAudioTrack, err := webrtc.NewTrackLocalStaticRTP(rtpAudioCodec, trackType, random.NewID())
			if err != nil {
				s.log.Error("failed to create local track", mlog.Err(err))
				return
			}

			us.mut.Lock()
			if trackType == "voice" {
				us.outVoiceTrack = outAudioTrack
				us.outVoiceTrackEnabled = true
			} else {
				us.outScreenAudioTrack = outAudioTrack
			}
			us.mut.Unlock()

			call.iterSessions(func(ss *session) {
				if ss.cfg.UserID == us.cfg.UserID {
					return
				}
				select {
				case ss.tracksCh <- outAudioTrack:
				default:
					s.log.Error("failed to send audio track: channel is full",
						mlog.String("UserID", us.cfg.UserID), mlog.String("TrackUserID", ss.cfg.UserID))
				}
			})

			for {
				rtp, _, readErr := remoteTrack.ReadRTP()
				if readErr != nil {
					s.log.Error("failed to read RTP packet", mlog.Err(readErr))
					s.metrics.IncRTCErrors(us.cfg.GroupID, "rtp")
					return
				}

				s.metrics.IncRTPPackets("in", trackType)
				s.metrics.AddRTPPacketBytes("in", trackType, len(rtp.Payload))

				if trackType == "voice" {
					us.mut.RLock()
					isEnabled := us.outVoiceTrackEnabled
					us.mut.RUnlock()
					if !isEnabled {
						continue
					}
				}

				if err := outAudioTrack.WriteRTP(rtp); err != nil && !errors.Is(err, io.ErrClosedPipe) {
					s.log.Error("failed to write RTP packet", mlog.Err(err))
					s.metrics.IncRTCErrors(us.cfg.GroupID, "rtp")
					return
				}

				call.iterSessions(func(ss *session) {
					if ss.cfg.UserID == us.cfg.UserID {
						return
					}
					s.metrics.IncRTPPackets("out", trackType)
					s.metrics.AddRTPPacketBytes("out", trackType, len(rtp.Payload))
				})
			}
		} else if remoteTrack.Codec().MimeType == rtpVideoCodecVP8.MimeType {
			if screenStreamID != "" && screenStreamID != streamID {
				s.log.Error("received unexpected video track", mlog.String("streamID", streamID))
				return
			}

			s.log.Debug("received screen sharing stream", mlog.String("streamID", streamID))

			outScreenTrack, err := webrtc.NewTrackLocalStaticRTP(rtpVideoCodecVP8, "screen", random.NewID())
			if err != nil {
				s.log.Error("failed to create local track", mlog.Err(err))
				return
			}
			us.mut.Lock()
			us.outScreenTrack = outScreenTrack
			us.remoteScreenTrack = remoteTrack
			us.mut.Unlock()

			call.iterSessions(func(ss *session) {
				if ss.cfg.UserID == us.cfg.UserID {
					return
				}
				select {
				case ss.tracksCh <- outScreenTrack:
				default:
					s.log.Error("failed to send screen track: channel is full",
						mlog.String("UserID", us.cfg.UserID), mlog.String("trackUserID", ss.cfg.UserID))
				}
			})

			for {
				rtp, _, readErr := remoteTrack.ReadRTP()
				if readErr != nil {
					s.log.Error("failed to read RTP packet", mlog.Err(readErr))
					s.metrics.IncRTCErrors(us.cfg.GroupID, "rtp")
					return
				}

				s.metrics.IncRTPPackets("in", "screen")
				s.metrics.AddRTPPacketBytes("in", "screen", len(rtp.Payload))

				if err := outScreenTrack.WriteRTP(rtp); err != nil && !errors.Is(err, io.ErrClosedPipe) {
					s.log.Error("failed to write RTP packet", mlog.Err(err))
					s.metrics.IncRTCErrors(us.cfg.GroupID, "rtp")
					return
				}

				call.iterSessions(func(ss *session) {
					if ss.cfg.UserID == us.cfg.UserID {
						return
					}
					s.metrics.IncRTPPackets("out", "screen")
					s.metrics.AddRTPPacketBytes("out", "screen", len(rtp.Payload))
				})
			}
		}
	})

	go func() {
		select {
		case msg, ok := <-us.sdpInCh:
			if !ok {
				return
			}
			sdp, err := us.signaling(msg)
			if err != nil {
				s.metrics.IncRTCErrors(cfg.GroupID, "signaling")
				s.log.Error("failed to signal", mlog.Err(err), mlog.Any("sessionCfg", us.cfg))
				return
			}

			select {
			case s.receiveCh <- newMessage(us, SDPMessage, sdp):
			default:
				s.log.Error("failed to send SDP message: channel is full")
				return
			}
		case <-time.After(signalingTimeout):
			s.log.Error("timed out signaling", mlog.Any("sessionCfg", us.cfg))
			s.metrics.IncRTCErrors(cfg.GroupID, "signaling")
			return
		}

		go us.handleICE(s.log, s.metrics)

		go func() {
			if err := s.handleTracks(call, us); err != nil {
				s.log.Error("handleTracks failed", mlog.Err(err))
			}
		}()
	}()

	return nil
}

func (s *Server) CloseSession(sessionID string) error {
	s.mut.Lock()
	cfg, ok := s.sessions[sessionID]
	delete(s.sessions, sessionID)
	s.mut.Unlock()
	if !ok {
		return nil
	}

	s.metrics.DecRTCSessions(cfg.GroupID, cfg.CallID)

	group := s.getGroup(cfg.GroupID)
	if group == nil {
		return fmt.Errorf("group not found: %s", cfg.GroupID)
	}
	call := group.getCall(cfg.CallID)
	if call == nil {
		return fmt.Errorf("call not found: %s", cfg.CallID)
	}
	session := call.getSession(cfg.SessionID)
	if session == nil {
		return fmt.Errorf("session not found: %s", cfg.SessionID)
	}

	call.mut.Lock()
	if session == call.screenSession {
		call.screenSession = nil
	}
	delete(call.sessions, cfg.SessionID)
	if len(call.sessions) == 0 {
		group.mut.Lock()
		delete(group.calls, cfg.CallID)
		if len(group.calls) == 0 {
			s.mut.Lock()
			delete(s.groups, cfg.GroupID)
			s.mut.Unlock()
		}
		group.mut.Unlock()
	}
	call.mut.Unlock()

	session.rtcConn.Close()
	close(session.closeCh)

	if session.closeCb != nil {
		return session.closeCb()
	}

	return nil
}

// handleTracks adds new a/v tracks to the peer associated with the session.
// It will listen for track events (e.g. mute/unmute) and disable/enable
// tracks accordingly.
func (s *Server) handleTracks(call *call, us *session) error {
	call.iterSessions(func(ss *session) {
		if ss.cfg.UserID == us.cfg.UserID {
			return
		}

		ss.mut.RLock()
		outVoiceTrack := ss.outVoiceTrack
		isEnabled := ss.outVoiceTrackEnabled
		outScreenTrack := ss.outScreenTrack
		outScreenAudioTrack := ss.outScreenAudioTrack
		ss.mut.RUnlock()

		if outVoiceTrack != nil {
			if err := us.addTrack(s.log, call, s.receiveCh, outVoiceTrack, isEnabled); err != nil {
				s.metrics.IncRTCErrors(us.cfg.GroupID, "track")
				s.log.Error("failed to add voice track", mlog.Err(err), mlog.String("sessionID", us.cfg.SessionID))
			}
		}
		if outScreenTrack != nil {
			if err := us.addTrack(s.log, call, s.receiveCh, outScreenTrack, true); err != nil {
				s.metrics.IncRTCErrors(us.cfg.GroupID, "track")
				s.log.Error("failed to add screen track", mlog.Err(err), mlog.String("sessionID", us.cfg.SessionID))
			}
		}
		if outScreenAudioTrack != nil {
			if err := us.addTrack(s.log, call, s.receiveCh, outScreenAudioTrack, true); err != nil {
				s.metrics.IncRTCErrors(us.cfg.GroupID, "track")
				s.log.Error("failed to add screen audio track", mlog.Err(err), mlog.String("sessionID", us.cfg.SessionID))
			}
		}

	})

	for {
		select {
		case track, ok := <-us.tracksCh:
			if !ok {
				return nil
			}
			if err := us.addTrack(s.log, call, s.receiveCh, track, true); err != nil {
				s.metrics.IncRTCErrors(us.cfg.GroupID, "track")
				s.log.Error("failed to add track", mlog.Err(err), mlog.String("sessionID", us.cfg.SessionID))
				continue
			}
		case msg, ok := <-us.sdpInCh:
			if !ok {
				return nil
			}

			sdp, err := us.signaling(msg)
			if err != nil {
				s.metrics.IncRTCErrors(us.cfg.GroupID, "signaling")
				s.log.Error("failed to signal", mlog.Err(err))
				continue
			}

			select {
			case s.receiveCh <- newMessage(us, SDPMessage, sdp):
			default:
				s.log.Error("failed to send SDP message: channel is full")
			}
		case muted, ok := <-us.trackEnableCh:
			if !ok {
				return nil
			}

			us.mut.RLock()
			track := us.outVoiceTrack
			us.mut.RUnlock()

			if track == nil {
				continue
			}

			us.mut.Lock()
			us.outVoiceTrackEnabled = !muted
			us.mut.Unlock()

			dummyTrack, err := webrtc.NewTrackLocalStaticRTP(rtpAudioCodec, "voice", random.NewID())
			if err != nil {
				s.log.Error("failed to create local track", mlog.Err(err))
				continue
			}

			call.iterSessions(func(ss *session) {
				if ss.cfg.UserID == us.cfg.UserID {
					return
				}

				ss.mut.RLock()
				sender := ss.rtpSendersMap[track]
				ss.mut.RUnlock()

				var replacingTrack *webrtc.TrackLocalStaticRTP
				if muted {
					replacingTrack = dummyTrack
				} else {
					replacingTrack = track
				}

				if sender != nil {
					s.log.Debug("replacing track on sender")
					if err := sender.ReplaceTrack(replacingTrack); err != nil {
						s.log.Error("failed to replace track", mlog.Err(err), mlog.String("sessionID", ss.cfg.SessionID))
					}
				}
			})
		case <-us.closeCh:
			return nil
		}
	}
}
