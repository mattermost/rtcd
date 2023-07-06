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

	"github.com/mattermost/mattermost/server/public/shared/mlog"

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
	responder, err := nack.NewResponderInterceptor(nack.ResponderSize(nackResponderBufferSize), nack.DisableCopy())
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

	// Congestion Control
	minRate := int(float32(getRateForSimulcastLevel(SimulcastLevelLow)) * 0.5)
	maxRate := int(float32(getRateForSimulcastLevel(SimulcastLevelHigh)) * 1.5)
	pacer := gcc.NewNoOpPacer()
	bwEstimatorCh := make(chan cc.BandwidthEstimator, 1)
	congestionController, err := cc.NewInterceptor(func() (cc.BandwidthEstimator, error) {
		return gcc.NewSendSideBWE(
			gcc.SendSideBWEInitialBitrate(minRate),
			gcc.SendSideBWEMinBitrate(minRate),
			gcc.SendSideBWEMaxBitrate(maxRate),
			gcc.SendSideBWEPacer(pacer),
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
	networkTypes := []webrtc.NetworkType{
		webrtc.NetworkTypeUDP4,
		webrtc.NetworkTypeTCP4,
	}
	if s.cfg.EnableIPv6 {
		networkTypes = append(networkTypes, webrtc.NetworkTypeUDP6, webrtc.NetworkTypeTCP6)
	}
	sEngine.SetNetworkTypes(networkTypes)
	sEngine.SetICEUDPMux(s.udpMux)
	sEngine.SetICETCPMux(s.tcpMux)
	sEngine.SetIncludeLoopbackCandidate(true)

	pairs, err := generateAddrsPairs(s.localIPs, s.publicAddrsMap, s.cfg.ICEHostOverride, s.cfg.EnableIPv6)
	if err != nil {
		return fmt.Errorf("failed to generate addresses pairs: %w", err)
	} else if len(pairs) > 0 {
		s.log.Debug("generated remote/local addrs pairs", mlog.Any("pairs", pairs))
		sEngine.SetNAT1To1IPs(pairs, webrtc.ICECandidateTypeHost)
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

	peerConn.OnDataChannel(func(dc *webrtc.DataChannel) {
		s.log.Debug("data channel open", mlog.String("sessionID", cfg.SessionID))

		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			if string(msg.Data) == "ping" {
				if err := dc.SendText("pong"); err != nil {
					s.log.Error("failed to send message", mlog.Err(err))
					return
				}
			}
		})
	})

	peerConn.OnTrack(func(remoteTrack *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		streamID := remoteTrack.StreamID()
		trackMimeType := remoteTrack.Codec().MimeType

		s.log.Debug("new track received",
			mlog.Any("codec", remoteTrack.Codec().RTPCodecCapability),
			mlog.Int("payload", int(remoteTrack.PayloadType())),
			mlog.String("type", trackMimeType),
			mlog.String("streamID", streamID),
			mlog.String("remoteTrackID", remoteTrack.ID()),
			mlog.Int("ssrc", int(remoteTrack.SSRC())),
			mlog.String("rid", remoteTrack.RID()),
			mlog.String("sessionID", us.cfg.SessionID),
		)

		s.metrics.IncRTPTracks(us.cfg.GroupID, us.cfg.CallID, "in", getTrackType(remoteTrack.Kind()))
		defer func() {
			s.log.Debug("exiting track handler",
				mlog.String("streamID", streamID),
				mlog.String("remoteTrackID", remoteTrack.ID()),
				mlog.Int("ssrc", int(remoteTrack.SSRC())),
				mlog.String("rid", remoteTrack.RID()),
				mlog.String("sessionID", us.cfg.SessionID))
			s.metrics.DecRTPTracks(us.cfg.GroupID, us.cfg.CallID, "in", getTrackType(remoteTrack.Kind()))
		}()

		var screenStreamID string
		if screenSession := call.getScreenSession(); screenSession != nil {
			screenStreamID = screenSession.getScreenStreamID()
		}

		go us.handleReceiverRTCP(receiver, remoteTrack.RID())

		if trackMimeType == rtpAudioCodec.MimeType {
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
				if ss.cfg.SessionID == us.cfg.SessionID {
					return
				}
				select {
				case ss.tracksCh <- trackActionContext{action: trackActionAdd, track: outAudioTrack}:
				default:
					s.log.Error("failed to send voice track: channel is full",
						mlog.String("userID", ss.cfg.UserID),
						mlog.String("sessionID", ss.cfg.SessionID),
						mlog.String("trackUserID", us.cfg.UserID),
						mlog.String("trackSessionID", us.cfg.SessionID),
					)
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

			var hasVAD bool
			if audioLevelExtensionID > 0 {
				if err := us.InitVAD(s.log, s.receiveCh); err != nil {
					s.log.Error("failed to init VAD", mlog.Err(err), mlog.String("sessionID", us.cfg.SessionID))
				} else {
					hasVAD = true
				}
			}

			for {
				packet, _, readErr := remoteTrack.ReadRTP()
				if readErr != nil {
					if !errors.Is(readErr, io.EOF) {
						s.log.Error("failed to read RTP packet",
							mlog.Err(readErr), mlog.String("sessionID", us.cfg.SessionID))
						s.metrics.IncRTCErrors(us.cfg.GroupID, "rtp")
					}
					return
				}

				if hasVAD {
					var ext rtp.AudioLevelExtension
					audioExtData := packet.GetExtension(uint8(audioLevelExtensionID))
					if audioExtData != nil {
						if err := ext.Unmarshal(audioExtData); err != nil {
							s.log.Error("failed to unmarshal audio level extension",
								mlog.Err(err), mlog.String("sessionID", us.cfg.SessionID))
						}
						us.mut.RLock()
						us.vadMonitor.PushAudioLevel(ext.Level)
						us.mut.RUnlock()
					}
				}

				if trackType == "voice" {
					us.mut.RLock()
					isEnabled := us.outVoiceTrackEnabled
					us.mut.RUnlock()
					if !isEnabled {
						continue
					}
				}

				if err := outAudioTrack.WriteRTP(packet); err != nil && !errors.Is(err, io.ErrClosedPipe) {
					s.log.Error("failed to write RTP packet",
						mlog.Err(err), mlog.String("sessionID", us.cfg.SessionID))
					s.metrics.IncRTCErrors(us.cfg.GroupID, "rtp")
					return
				}
			}
		} else if params, ok := rtpVideoCodecs[trackMimeType]; ok {
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

			rm, err := NewRateMonitor(simulcastRateMonitorSampleSizes[rid], nil)
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
				if ss.cfg.SessionID == us.cfg.SessionID {
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
					mlog.String("sessionID", ss.cfg.SessionID),
				)

				select {
				case ss.tracksCh <- trackActionContext{action: trackActionAdd, track: outScreenTrack}:
				default:
					s.log.Error("failed to send screen track: channel is full",
						mlog.String("userID", ss.cfg.UserID),
						mlog.String("sessionID", ss.cfg.SessionID),
						mlog.String("trackUserID", us.cfg.UserID),
						mlog.String("trackSessionID", us.cfg.SessionID),
					)
				}
			})

			limiter := rate.NewLimiter(0.25, 1)

			for {
				packet, _, readErr := remoteTrack.ReadRTP()
				if readErr != nil {
					if !errors.Is(readErr, io.EOF) {
						s.log.Error("failed to read RTP packet",
							mlog.Err(readErr), mlog.String("sessionID", us.cfg.SessionID))
						s.metrics.IncRTCErrors(us.cfg.GroupID, "rtp")
					}
					return
				}

				rm.PushSample(packet.MarshalSize())
				if limiter.Allow() {
					rate, dur := rm.GetRate()
					s.log.Debug("rate monitor",
						mlog.String("sessionID", us.cfg.SessionID),
						mlog.String("RID", rid),
						mlog.Int("rate", rate),
						mlog.Float64("duration", dur.Seconds()),
						mlog.Float64("totalDuration", rm.GetSamplesDuration().Seconds()),
					)
				}

				if err := outScreenTrack.WriteRTP(packet); err != nil && !errors.Is(err, io.ErrClosedPipe) {
					s.log.Error("failed to write RTP packet",
						mlog.Err(err), mlog.String("sessionID", us.cfg.SessionID))
					s.metrics.IncRTCErrors(us.cfg.GroupID, "rtp")
					return
				}
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
		case <-us.closeCh:
			s.log.Debug("closeCh closed during signaling", mlog.Any("sessionCfg", us.cfg))
			return
		}

		go us.handleICE(s.metrics)

		s.handleTracks(call, us)
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
	us := call.getSession(cfg.SessionID)
	if us == nil {
		return fmt.Errorf("session not found: %s", cfg.SessionID)
	}

	call.mut.Lock()

	call.handleSessionClose(us)

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

	us.rtcConn.Close()
	close(us.closeCh)

	if us.closeCb != nil {
		return us.closeCb()
	}

	return nil
}

// handleTracks manages (adds and removes) a/v tracks for the peer associated with the session.
func (s *Server) handleTracks(call *call, us *session) {
	call.iterSessions(func(ss *session) {
		if ss.cfg.SessionID == us.cfg.SessionID {
			return
		}

		ss.mut.RLock()
		outVoiceTrack := ss.outVoiceTrack
		outScreenTrack := ss.outScreenTracks[SimulcastLevelDefault]
		outScreenAudioTrack := ss.outScreenAudioTrack
		ss.mut.RUnlock()

		var outTracks []*webrtc.TrackLocalStaticRTP
		if outVoiceTrack != nil {
			outTracks = append(outTracks, outVoiceTrack)
		}
		if outScreenTrack != nil {
			outTracks = append(outTracks, outScreenTrack)
		}
		if outScreenAudioTrack != nil {
			outTracks = append(outTracks, outScreenAudioTrack)
		}

		for _, track := range outTracks {
			select {
			case us.tracksCh <- trackActionContext{action: trackActionAdd, track: track}:
			default:
				s.metrics.IncRTCErrors(us.cfg.GroupID, "track")
				s.log.Error("failed to add track on join: channel is full", mlog.String("sessionID", us.cfg.SessionID))
			}
		}
	})

	for {
		select {
		case ctx, ok := <-us.tracksCh:
			if !ok {
				return
			}

			if ctx.action == trackActionAdd {
				if err := us.addTrack(s.receiveCh, ctx.track); err != nil {
					s.metrics.IncRTCErrors(us.cfg.GroupID, "track")
					s.log.Error("failed to add track", mlog.Err(err), mlog.String("sessionID", us.cfg.SessionID), mlog.String("trackID", ctx.track.ID()))
					continue
				}
			} else if ctx.action == trackActionRemove {
				if err := us.removeTrack(s.receiveCh, ctx.track); err != nil {
					s.metrics.IncRTCErrors(us.cfg.GroupID, "track")
					s.log.Error("failed to remove track", mlog.Err(err), mlog.String("sessionID", us.cfg.SessionID), mlog.String("trackID", ctx.track.ID()))
					continue
				}
			} else {
				s.log.Error("invalid track action", mlog.Int("action", int(ctx.action)), mlog.String("sessionID", us.cfg.SessionID))
				continue
			}
		case offer, ok := <-us.sdpOfferInCh:
			if !ok {
				return
			}

			if err := us.signaling(offer, s.receiveCh); err != nil {
				s.metrics.IncRTCErrors(us.cfg.GroupID, "signaling")
				s.log.Error("failed to signal", mlog.Err(err), mlog.String("sessionID", us.cfg.SessionID))
				continue
			}
		case <-us.closeCh:
			return
		}
	}
}
