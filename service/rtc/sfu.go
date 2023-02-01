// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"errors"
	"fmt"
	"io"
	"time"

	"golang.org/x/time/rate"

	"github.com/mattermost/rtcd/service/random"

	"github.com/mattermost/mattermost-server/v6/shared/mlog"

	"github.com/pion/ice/v2"
	"github.com/pion/interceptor"
	"github.com/pion/interceptor/pkg/cc"
	"github.com/pion/interceptor/pkg/gcc"
	"github.com/pion/interceptor/pkg/nack"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
)

var (
	videoRTCPFeedback = []webrtc.RTCPFeedback{
		{Type: "goog-remb", Parameter: ""},
		{Type: "ccm", Parameter: "fir"},
		{Type: "nack", Parameter: ""},
		{Type: "nack", Parameter: "pli"},
	}
	rtpAudioCodec = webrtc.RTPCodecCapability{
		MimeType:     "audio/opus",
		ClockRate:    48000,
		Channels:     2,
		SDPFmtpLine:  "minptime=10;useinbandfec=1",
		RTCPFeedback: nil,
	}
	rtpVideoCodecs = map[string]webrtc.RTPCodecParameters{
		"video/VP8": {
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:     "video/VP8",
				ClockRate:    90000,
				SDPFmtpLine:  "",
				RTCPFeedback: videoRTCPFeedback,
			},
			PayloadType: 96,
		},
	}
	rtpVideoExtensions = []string{
		"urn:ietf:params:rtp-hdrext:sdes:mid",
		"urn:ietf:params:rtp-hdrext:sdes:rtp-stream-id",
		"urn:ietf:params:rtp-hdrext:sdes:repaired-rtp-stream-id",
	}
)

const (
	nackResponderBufferSize = 256
	audioLevelExtensionURI  = "urn:ietf:params:rtp-hdrext:ssrc-audio-level"
)

func initMediaEngine() (*webrtc.MediaEngine, error) {
	var m webrtc.MediaEngine
	if err := m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: rtpAudioCodec,
		PayloadType:        111,
	}, webrtc.RTPCodecTypeAudio); err != nil {
		return nil, err
	}
	for _, params := range rtpVideoCodecs {
		if err := m.RegisterCodec(params, webrtc.RTPCodecTypeVideo); err != nil {
			return nil, err
		}
	}

	if err := m.RegisterHeaderExtension(webrtc.RTPHeaderExtensionCapability{
		URI: audioLevelExtensionURI,
	}, webrtc.RTPCodecTypeAudio); err != nil {
		return nil, fmt.Errorf("failed to register header extension: %w", err)
	}

	for _, ext := range rtpVideoExtensions {
		if err := m.RegisterHeaderExtension(webrtc.RTPHeaderExtensionCapability{URI: ext}, webrtc.RTPCodecTypeVideo); err != nil {
			return nil, fmt.Errorf("failed to register header extension: %w", err)
		}
	}

	return &m, nil
}

func initInterceptors(m *webrtc.MediaEngine) (*interceptor.Registry, <-chan cc.BandwidthEstimator, error) {
	var i interceptor.Registry
	generator, err := nack.NewGeneratorInterceptor()
	if err != nil {
		return nil, nil, err
	}

	// NACK
	responder, err := nack.NewResponderInterceptor(nack.ResponderSize(nackResponderBufferSize))
	if err != nil {
		return nil, nil, err
	}
	m.RegisterFeedback(webrtc.RTCPFeedback{Type: "nack"}, webrtc.RTPCodecTypeVideo)
	m.RegisterFeedback(webrtc.RTCPFeedback{Type: "nack", Parameter: "pli"}, webrtc.RTPCodecTypeVideo)
	i.Add(responder)
	i.Add(generator)

	// RTCP Reports
	if err := webrtc.ConfigureRTCPReports(&i); err != nil {
		return nil, nil, err
	}

	// TWCC
	if err := webrtc.ConfigureTWCCSender(m, &i); err != nil {
		return nil, nil, err
	}

	// Congestion Controller
	bwEstimatorCh := make(chan cc.BandwidthEstimator, 1)
	congestionController, err := cc.NewInterceptor(func() (cc.BandwidthEstimator, error) {
		return gcc.NewSendSideBWE(
			gcc.SendSideBWEInitialBitrate(int(float32(getRateForSimulcastLevel(SimulcastLevelLow))*0.50)),
			gcc.SendSideBWEMinBitrate(int(float32(getRateForSimulcastLevel(SimulcastLevelLow))*0.50)),
			gcc.SendSideBWEMaxBitrate(int(float32(getRateForSimulcastLevel(SimulcastLevelHigh))*1.50)),
		)
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to init congestion controller: %w", err)
	}
	congestionController.OnNewPeerConnection(func(id string, estimator cc.BandwidthEstimator) {
		bwEstimatorCh <- estimator
	})
	i.Add(congestionController)
	if err = webrtc.ConfigureTWCCHeaderExtensionSender(m, &i); err != nil {
		return nil, nil, fmt.Errorf("failed to add TWCC extensions: %w", err)
	}

	return &i, bwEstimatorCh, nil
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
		SDPSemantics: webrtc.SDPSemanticsUnifiedPlan,
	}

	m, err := initMediaEngine()
	if err != nil {
		return fmt.Errorf("failed to init media engine: %w", err)
	}

	i, bwEstimatorCh, err := initInterceptors(m)
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

	us.initBWEstimator(<-bwEstimatorCh)

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
			mlog.Int("ssrc", int(remoteTrack.SSRC())),
			mlog.String("sessionID", us.cfg.SessionID),
			mlog.String("rid", remoteTrack.RID()),
		)

		var screenStreamID string
		if screenSession := call.getScreenSession(); screenSession != nil {
			screenStreamID = screenSession.getScreenStreamID()
		}

		go us.handleReceiverRTCP(receiver, remoteTrack.RID())

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
				case ss.tracksCh <- trackActionContext{action: trackActionAdd, track: outAudioTrack}:
				default:
					s.log.Error("failed to send audio track: channel is full",
						mlog.String("UserID", us.cfg.UserID), mlog.String("TrackUserID", ss.cfg.UserID))
				}
			})

			var audioLevelExtensionID int
			for _, ext := range receiver.GetParameters().HeaderExtensions {
				if ext.URI == audioLevelExtensionURI {
					s.log.Debug("found audio level extension", mlog.Any("ext", ext), mlog.String("sessionID", us.cfg.SessionID))
					audioLevelExtensionID = ext.ID
					break
				}
			}

			if audioLevelExtensionID > 0 {
				if err := us.InitVAD(s.log, s.receiveCh); err != nil {
					s.log.Error("failed to init VAD", mlog.Err(err), mlog.String("sessionID", us.cfg.SessionID))
				}
			}

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

				packet := &rtp.Packet{}
				if err := packet.Unmarshal(buf[:i]); err != nil {
					s.log.Error("failed to unmarshal RTP packet",
						mlog.Err(err), mlog.String("sessionID", us.cfg.SessionID))
					s.metrics.IncRTCErrors(us.cfg.GroupID, "rtp")
					return
				}

				if us.vadMonitor != nil && audioLevelExtensionID > 0 {
					var ext rtp.AudioLevelExtension
					audioExtData := packet.GetExtension(uint8(audioLevelExtensionID))
					if audioExtData != nil {
						if err := ext.Unmarshal(audioExtData); err != nil {
							s.log.Error("failed to unmarshal audio level extension",
								mlog.Err(err), mlog.String("sessionID", us.cfg.SessionID))
						}
						us.vadMonitor.PushAudioLevel(ext.Level)
					}
				}

				s.metrics.IncRTPPackets("in", trackType)
				s.metrics.AddRTPPacketBytes("in", trackType, len(packet.Payload))

				if trackType == "voice" {
					us.mut.RLock()
					isEnabled := us.outVoiceTrackEnabled
					us.mut.RUnlock()
					if !isEnabled {
						s.bufPool.Put(buf)
						continue
					}
				}

				if err := outAudioTrack.WriteRTP(packet); err != nil && !errors.Is(err, io.ErrClosedPipe) {
					s.log.Error("failed to write RTP packet",
						mlog.Err(err), mlog.String("sessionID", us.cfg.SessionID))
					s.metrics.IncRTCErrors(us.cfg.GroupID, "rtp")
					return
				}
				pLen := len(packet.Payload)
				s.bufPool.Put(buf)

				call.iterSessions(func(ss *session) {
					if ss.cfg.UserID == us.cfg.UserID {
						return
					}
					s.metrics.IncRTPPackets("out", trackType)
					s.metrics.AddRTPPacketBytes("out", trackType, pLen)
				})
			}
		} else if params, ok := rtpVideoCodecs[trackType]; ok {
			if screenStreamID != "" && screenStreamID != streamID {
				s.log.Error("received unexpected video track",
					mlog.String("streamID", streamID), mlog.String("sessionID", us.cfg.SessionID))
				return
			}

			s.log.Debug("received screen sharing stream", mlog.String("streamID", streamID), mlog.String("sessionID", us.cfg.SessionID))

			outScreenTrack, err := webrtc.NewTrackLocalStaticRTP(params.RTPCodecCapability, genTrackID("screen", us.cfg.SessionID), random.NewID(), webrtc.WithRTPStreamID(remoteTrack.RID()))
			if err != nil {
				s.log.Error("failed to create local track",
					mlog.Err(err), mlog.String("sessionID", us.cfg.SessionID))
				return
			}

			rid := remoteTrack.RID()
			if rid == "" {
				rid = SimulcastLevelDefault
			}

			rm, err := NewRateMonitor(250, nil)
			if err != nil {
				s.log.Error("failed to create rate monitor", mlog.Err(err))
				return
			}

			us.mut.Lock()
			us.outScreenTracks[rid] = outScreenTrack
			us.remoteScreenTracks[rid] = remoteTrack
			us.screenRateMonitors[rid] = rm
			us.mut.Unlock()

			call.iterSessions(func(ss *session) {
				if ss.cfg.UserID == us.cfg.UserID {
					return
				}

				expectedLevel := SimulcastLevelDefault
				if remoteTrack.RID() != "" {
					expectedLevel = ss.getExpectedSimulcastLevel()
				}

				if rid != expectedLevel {
					return
				}

				s.log.Debug("received track matches expected level, sending",
					mlog.String("lvl", expectedLevel),
					mlog.String("sessionID", us.cfg.SessionID),
				)

				select {
				case ss.tracksCh <- trackActionContext{action: trackActionAdd, track: outScreenTrack}:
				default:
					s.log.Error("failed to send screen track: channel is full",
						mlog.String("userID", us.cfg.UserID),
						mlog.String("sessionID", us.cfg.SessionID),
						mlog.String("trackUserID", ss.cfg.UserID),
						mlog.String("trackSessionID", ss.cfg.SessionID),
					)
				}
			})

			limiter := rate.NewLimiter(0.25, 1)

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

				rm.PushSample(i)
				if limiter.Allow() {
					s.log.Debug("rate monitor", mlog.String("RID", rid), mlog.Int("rate", rm.GetRate()), mlog.Float64("time", rm.GetSamplesDuration().Seconds()))
				}

				s.metrics.IncRTPPackets("in", "screen")
				s.metrics.AddRTPPacketBytes("in", "screen", len(rtp.Payload))

				if err := outScreenTrack.WriteRTP(rtp); err != nil && !errors.Is(err, io.ErrClosedPipe) {
					s.log.Error("failed to write RTP packet",
						mlog.Err(err), mlog.String("sessionID", us.cfg.SessionID))
					s.metrics.IncRTCErrors(us.cfg.GroupID, "rtp")
					return
				}

				s.bufPool.Put(buf)

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

		go us.handleICE(s.metrics)

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
		outScreenTrack := ss.outScreenTracks[SimulcastLevelDefault]
		outScreenAudioTrack := ss.outScreenAudioTrack
		ss.mut.RUnlock()

		if outVoiceTrack != nil {
			if err := us.addTrack(s.receiveCh, outVoiceTrack); err != nil {
				s.metrics.IncRTCErrors(us.cfg.GroupID, "track")
				s.log.Error("failed to add voice track", mlog.Err(err), mlog.String("sessionID", us.cfg.SessionID))
			}
		}
		if outScreenTrack != nil {
			if err := us.addTrack(s.receiveCh, outScreenTrack); err != nil {
				s.metrics.IncRTCErrors(us.cfg.GroupID, "track")
				s.log.Error("failed to add screen track", mlog.Err(err), mlog.String("sessionID", us.cfg.SessionID))
			}
		}
		if outScreenAudioTrack != nil {
			if err := us.addTrack(s.receiveCh, outScreenAudioTrack); err != nil {
				s.metrics.IncRTCErrors(us.cfg.GroupID, "track")
				s.log.Error("failed to add screen audio track", mlog.Err(err), mlog.String("sessionID", us.cfg.SessionID))
			}
		}
	})

	for {
		select {
		case ctx, ok := <-us.tracksCh:
			if !ok {
				return nil
			}

			if ctx.action == trackActionAdd {
				if err := us.addTrack(s.receiveCh, ctx.track); err != nil {
					s.metrics.IncRTCErrors(us.cfg.GroupID, "track")
					s.log.Error("failed to add track", mlog.Err(err), mlog.String("sessionID", us.cfg.SessionID))
					continue
				}
			} else if ctx.action == trackActionRemove {
				if err := us.removeTrack(s.receiveCh, ctx.track); err != nil {
					s.metrics.IncRTCErrors(us.cfg.GroupID, "track")
					s.log.Error("failed to remove track", mlog.Err(err), mlog.String("sessionID", us.cfg.SessionID))
					continue
				}
			} else {
				s.log.Error("invalid track action", mlog.Int("action", int(ctx.action)), mlog.String("sessionID", us.cfg.SessionID))
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
