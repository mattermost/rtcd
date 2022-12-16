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
	"github.com/pion/rtp"
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

	iceServers := make([]webrtc.ICEServer, 0, len(s.cfg.ICEServers))
	for _, iceCfg := range s.cfg.ICEServers {
		// generating short-lived TURN credentials if needed.
		if iceCfg.IsTURN() && s.cfg.TURNConfig.StaticAuthSecret == "" {
			continue
		}
		if iceCfg.IsTURN() && iceCfg.Username == "" && iceCfg.Credential == "" {
			ts := time.Now().Add(time.Duration(s.cfg.TURNConfig.CredentialsExpirationMinutes) * time.Minute).Unix()
			username, password, err := genTURNCredentials(cfg.SessionID, s.cfg.TURNConfig.StaticAuthSecret, ts)
			if err != nil {
				s.log.Error("failed to generate TURN credentials", mlog.Err(err))
				continue
			}
			iceCfg.Username = username
			iceCfg.Credential = password
		}
		iceServers = append(iceServers, webrtc.ICEServer{
			URLs:       iceCfg.URLs,
			Username:   iceCfg.Username,
			Credential: iceCfg.Credential,
		})
	}

	peerConnConfig := webrtc.Configuration{
		ICEServers:   iceServers,
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

	api := webrtc.NewAPI(
		webrtc.WithMediaEngine(m),
		webrtc.WithSettingEngine(sEngine),
		webrtc.WithInterceptorRegistry(i),
	)
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
			s.log.Error("failed to create ICE message", mlog.Err(err), mlog.String("sessionID", cfg.SessionID))
			return
		}
		select {
		case s.receiveCh <- msg:
		default:
			s.log.Error("failed to send ICE message: channel is full", mlog.String("sessionID", cfg.SessionID))
		}
	})

	peerConn.OnICEGatheringStateChange(func(state webrtc.ICEGathererState) {
		if state == webrtc.ICEGathererStateComplete {
			s.log.Debug("ice gathering complete", mlog.String("sessionID", cfg.SessionID))
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
		streamID := remoteTrack.StreamID()
		trackType := remoteTrack.Codec().MimeType

		s.log.Debug("new track received",
			mlog.Any("codec", remoteTrack.Codec().RTPCodecCapability),
			mlog.Int("payload", int(remoteTrack.PayloadType())),
			mlog.String("type", trackType),
			mlog.String("streamID", streamID),
			mlog.String("remoteTrackID", remoteTrack.ID()),
			mlog.Int("SSRC", int(remoteTrack.SSRC())),
			mlog.String("sessionID", us.cfg.SessionID),
		)

		var screenStreamID string
		if screenSession := call.getScreenSession(); screenSession != nil {
			screenStreamID = screenSession.getScreenStreamID()
		}

		go us.handleReceiverRTCP(s.log, call, receiver)

		if trackType == rtpAudioCodec.MimeType {
			trackType := "voice"
			if streamID == screenStreamID {
				s.log.Debug("received screen sharing audio track", mlog.String("sessionID", us.cfg.SessionID))
				trackType = "screen-audio"
			}

			outAudioTrack, err := webrtc.NewTrackLocalStaticRTP(rtpAudioCodec, genTrackID(trackType, us.cfg.SessionID), random.NewID())
			if err != nil {
				s.log.Error("failed to create local track", mlog.Err(err), mlog.String("sessionID", us.cfg.SessionID))
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
				buf := s.bufPool.Get().([]byte)
				i, _, readErr := remoteTrack.Read(buf)
				if readErr != nil {
					if !errors.Is(readErr, io.EOF) {
						s.log.Error("failed to read RTP packet",
							mlog.Err(readErr), mlog.String("sessionID", us.cfg.SessionID))
						s.metrics.IncRTCErrors(us.cfg.GroupID, "rtp")
					}
					return
				}

				rtp := &rtp.Packet{}
				if err := rtp.Unmarshal(buf[:i]); err != nil {
					s.log.Error("failed to unmarshal RTP packet",
						mlog.Err(err), mlog.String("sessionID", us.cfg.SessionID))
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
						s.bufPool.Put(buf)
						continue
					}
				}

				if err := outAudioTrack.WriteRTP(rtp); err != nil && !errors.Is(err, io.ErrClosedPipe) {
					s.log.Error("failed to write RTP packet",
						mlog.Err(err), mlog.String("sessionID", us.cfg.SessionID))
					s.metrics.IncRTCErrors(us.cfg.GroupID, "rtp")
					return
				}
				pLen := len(rtp.Payload)
				s.bufPool.Put(buf)

				call.iterSessions(func(ss *session) {
					if ss.cfg.UserID == us.cfg.UserID {
						return
					}
					s.metrics.IncRTPPackets("out", trackType)
					s.metrics.AddRTPPacketBytes("out", trackType, pLen)
				})
			}
		} else if trackType == rtpVideoCodecVP8.MimeType {
			if screenStreamID != "" && screenStreamID != streamID {
				s.log.Error("received unexpected video track",
					mlog.String("streamID", streamID), mlog.String("sessionID", us.cfg.SessionID))
				return
			}

			s.log.Debug("received screen sharing stream", mlog.String("streamID", streamID), mlog.String("sessionID", us.cfg.SessionID))

			outScreenTrack, err := webrtc.NewTrackLocalStaticRTP(rtpVideoCodecVP8, genTrackID("screen", us.cfg.SessionID), random.NewID())
			if err != nil {
				s.log.Error("failed to create local track",
					mlog.Err(err), mlog.String("sessionID", us.cfg.SessionID))
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
						mlog.String("UserID", us.cfg.UserID),
						mlog.String("sessionID", us.cfg.SessionID),
						mlog.String("trackUserID", ss.cfg.UserID),
						mlog.String("trackSessionID", ss.cfg.SessionID),
					)
				}
			})

			for {
				rtp, _, readErr := remoteTrack.ReadRTP()
				if readErr != nil {
					if !errors.Is(readErr, io.EOF) {
						s.log.Error("failed to read RTP packet",
							mlog.Err(readErr), mlog.String("sessionID", us.cfg.SessionID))
						s.metrics.IncRTCErrors(us.cfg.GroupID, "rtp")
					}
					return
				}

				s.metrics.IncRTPPackets("in", "screen")
				s.metrics.AddRTPPacketBytes("in", "screen", len(rtp.Payload))

				if err := outScreenTrack.WriteRTP(rtp); err != nil && !errors.Is(err, io.ErrClosedPipe) {
					s.log.Error("failed to write RTP packet",
						mlog.Err(err), mlog.String("sessionID", us.cfg.SessionID))
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
		case offer, ok := <-us.sdpOfferInCh:
			if !ok {
				return
			}
			if err := us.signaling(offer, s.receiveCh); err != nil {
				s.metrics.IncRTCErrors(cfg.GroupID, "signaling")
				s.log.Error("failed to signal", mlog.Err(err), mlog.Any("sessionCfg", us.cfg))
				return
			}
		case <-time.After(signalingTimeout):
			s.log.Error("timed out signaling", mlog.Any("sessionCfg", us.cfg))
			s.metrics.IncRTCErrors(cfg.GroupID, "signaling")
			if err := s.CloseSession(cfg.SessionID); err != nil {
				s.log.Error("failed to close session", mlog.Any("sessionCfg", us.cfg))
			}
			return
		}

		go us.handleICE(s.log, s.metrics)

		go func() {
			if err := s.handleTracks(call, us); err != nil {
				s.log.Error("handleTracks failed", mlog.Err(err), mlog.String("sessionID", us.cfg.SessionID))
			}
		}()
	}()

	return nil
}

func (s *Server) CloseSession(sessionID string) error {
	s.mut.Lock()
	cfg, ok := s.sessions[sessionID]
	delete(s.sessions, sessionID)

	if len(s.sessions) == 0 && s.drainCh != nil {
		s.log.Debug("closing drain channel")
		close(s.drainCh)
		s.drainCh = nil
	}

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
		outScreenTrack := ss.outScreenTrack
		outScreenAudioTrack := ss.outScreenAudioTrack
		ss.mut.RUnlock()

		if outVoiceTrack != nil {
			if err := us.addTrack(s.log, call, s.receiveCh, outVoiceTrack); err != nil {
				s.metrics.IncRTCErrors(us.cfg.GroupID, "track")
				s.log.Error("failed to add voice track", mlog.Err(err), mlog.String("sessionID", us.cfg.SessionID))
			}
		}
		if outScreenTrack != nil {
			if err := us.addTrack(s.log, call, s.receiveCh, outScreenTrack); err != nil {
				s.metrics.IncRTCErrors(us.cfg.GroupID, "track")
				s.log.Error("failed to add screen track", mlog.Err(err), mlog.String("sessionID", us.cfg.SessionID))
			}
		}
		if outScreenAudioTrack != nil {
			if err := us.addTrack(s.log, call, s.receiveCh, outScreenAudioTrack); err != nil {
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
			if err := us.addTrack(s.log, call, s.receiveCh, track); err != nil {
				s.metrics.IncRTCErrors(us.cfg.GroupID, "track")
				s.log.Error("failed to add track", mlog.Err(err), mlog.String("sessionID", us.cfg.SessionID))
				continue
			}
		case offer, ok := <-us.sdpOfferInCh:
			if !ok {
				return nil
			}

			if err := us.signaling(offer, s.receiveCh); err != nil {
				s.metrics.IncRTCErrors(us.cfg.GroupID, "signaling")
				s.log.Error("failed to signal", mlog.Err(err), mlog.String("sessionID", us.cfg.SessionID))
				continue
			}
		case <-us.closeCh:
			return nil
		}
	}
}
