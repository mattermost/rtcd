// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"time"

	"golang.org/x/time/rate"

	"github.com/mattermost/rtcd/service/random"

	"github.com/mattermost/mattermost/server/public/shared/mlog"

	"github.com/pion/ice/v4"
	"github.com/pion/interceptor"
	"github.com/pion/interceptor/pkg/cc"
	"github.com/pion/interceptor/pkg/gcc"
	"github.com/pion/interceptor/pkg/nack"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
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
		webrtc.MimeTypeVP8: {
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:     webrtc.MimeTypeVP8,
				ClockRate:    90000,
				SDPFmtpLine:  "",
				RTCPFeedback: videoRTCPFeedback,
			},
			PayloadType: 96,
		},
		webrtc.MimeTypeAV1: {
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:     webrtc.MimeTypeAV1,
				ClockRate:    90000,
				SDPFmtpLine:  "",
				RTCPFeedback: videoRTCPFeedback,
			},
			PayloadType: 45,
		},
	}
	rtpVideoExtensions = []string{
		"urn:ietf:params:rtp-hdrext:sdes:mid",
		"urn:ietf:params:rtp-hdrext:sdes:rtp-stream-id",
		"urn:ietf:params:rtp-hdrext:sdes:repaired-rtp-stream-id",
	}
)

const (
	nackResponderBufferSize    = 256
	audioLevelExtensionURI     = "urn:ietf:params:rtp-hdrext:ssrc-audio-level"
	writerQueueSize            = 200 // Enough to hold up to one second of video packets.
	ScreenTrackMimeTypeDefault = webrtc.MimeTypeVP8
)

func (s *Server) initSettingEngine() (webrtc.SettingEngine, error) {
	sEngine := webrtc.SettingEngine{
		LoggerFactory: s,
	}
	sEngine.EnableSCTPZeroChecksum(true)
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
	if os.Getenv("RTCD_RTC_DTLS_INSECURE_SKIP_HELLOVERIFY") == "true" {
		s.log.Warn("RTCD_RTC_DTLS_INSECURE_SKIP_HELLOVERIFY is set, will skip hello verify phase")
		sEngine.SetDTLSInsecureSkipHelloVerify(true)
	}

	pairs, err := generateAddrsPairs(s.localIPs, s.publicAddrsMap, s.cfg.ICEHostOverride, s.cfg.EnableIPv6)
	if err != nil {
		return webrtc.SettingEngine{}, fmt.Errorf("failed to generate addresses pairs: %w", err)
	} else if len(pairs) > 0 {
		s.log.Debug("generated remote/local addrs pairs", mlog.Any("pairs", pairs))
		sEngine.SetNAT1To1IPs(pairs, webrtc.ICECandidateTypeHost)
	}

	return sEngine, nil
}

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
	congestionController.OnNewPeerConnection(func(_ string, estimator cc.BandwidthEstimator) {
		bwEstimatorCh <- estimator
	})
	i.Add(congestionController)
	if err = webrtc.ConfigureTWCCHeaderExtensionSender(m, &i); err != nil {
		return nil, nil, fmt.Errorf("failed to add TWCC extensions: %w", err)
	}

	return &i, bwEstimatorCh, nil
}

func (s *Server) InitSession(cfg SessionConfig, closeCb func() error) error {
	start := time.Now()

	if err := cfg.IsValid(); err != nil {
		return fmt.Errorf("invalid session config: %w", err)
	}

	s.metrics.IncRTCSessions(cfg.GroupID)

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

	mEngine, err := initMediaEngine()
	if err != nil {
		return fmt.Errorf("failed to init media engine: %w", err)
	}

	iRegistry, bwEstimatorCh, err := initInterceptors(mEngine)
	if err != nil {
		return fmt.Errorf("failed to init interceptors: %w", err)
	}

	sEngine, err := s.initSettingEngine()
	if err != nil {
		return fmt.Errorf("failed to init setting engine: %w", err)
	}

	api := webrtc.NewAPI(
		webrtc.WithMediaEngine(mEngine),
		webrtc.WithSettingEngine(sEngine),
		webrtc.WithInterceptorRegistry(iRegistry),
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
		us.mut.RLock()
		defer us.mut.RUnlock()
		if candidate == nil {
			return
		}

		if port := s.cfg.ICEHostPortOverride.SinglePort(); port != 0 && candidate.Typ == webrtc.ICECandidateTypeHost {
			if m := getExternalAddrMapFromHostOverride(s.cfg.ICEHostOverride, s.publicAddrsMap); m[candidate.Address] {
				s.log.Debug("overriding host candidate port",
					mlog.String("sessionID", cfg.SessionID),
					mlog.Uint("port", candidate.Port),
					mlog.Int("override", port),
					mlog.String("addr", candidate.Address),
					mlog.Int("protocol", candidate.Protocol))
				candidate.Port = uint16(port)
			}
		}

		msg, err := newICEMessage(us, candidate)
		if err != nil {
			s.log.Error("failed to create ICE message", mlog.Err(err), mlog.String("sessionID", cfg.SessionID))
			return
		}

		select {
		case <-us.closeCh:
			s.log.Debug("closeCh closed during ICE gathering", mlog.Any("sessionCfg", us.cfg))
			return
		default:
		}

		select {
		case s.receiveCh <- msg:
		default:
			s.log.Error("failed to send ICE message: channel is full", mlog.String("sessionID", cfg.SessionID))
		}
	})

	peerConn.OnICEGatheringStateChange(func(state webrtc.ICEGatheringState) {
		if state == webrtc.ICEGatheringStateComplete {
			s.log.Debug("ice gathering complete", mlog.String("sessionID", cfg.SessionID))
		}
	})

	peerConn.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		if state == webrtc.PeerConnectionStateConnected {
			s.log.Debug("rtc connected!", mlog.String("sessionID", cfg.SessionID))
			s.metrics.IncRTCConnState("connected")
			s.metrics.ObserveRTCConnectionTime(cfg.GroupID, time.Since(start).Seconds())
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

	peerConn.OnDataChannel(func(dataCh *webrtc.DataChannel) {
		s.handleDC(us, dataCh)
	})

	peerConn.OnTrack(func(remoteTrack *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		streamID := remoteTrack.StreamID()
		trackMimeType := remoteTrack.Codec().MimeType

		s.log.Debug("new track received",
			mlog.Any("codec", remoteTrack.Codec().RTPCodecCapability),
			mlog.Int("payload", int(remoteTrack.PayloadType())),
			mlog.Int("kind", int(remoteTrack.Kind())),
			mlog.String("type", trackMimeType),
			mlog.String("streamID", streamID),
			mlog.String("remoteTrackID", remoteTrack.ID()),
			mlog.Int("ssrc", int(remoteTrack.SSRC())),
			mlog.String("rid", remoteTrack.RID()),
			mlog.String("sessionID", us.cfg.SessionID),
		)

		s.metrics.IncRTPTracks(us.cfg.GroupID, "in", trackMimeType)
		defer func() {
			s.log.Debug("exiting track handler",
				mlog.String("streamID", streamID),
				mlog.String("remoteTrackID", remoteTrack.ID()),
				mlog.Int("ssrc", int(remoteTrack.SSRC())),
				mlog.String("rid", remoteTrack.RID()),
				mlog.String("sessionID", us.cfg.SessionID))

			if err := receiver.Stop(); err != nil {
				s.log.Error("failed to stop receiver",
					mlog.Err(err),
					mlog.String("streamID", streamID),
					mlog.String("remoteTrackID", remoteTrack.ID()),
					mlog.Int("ssrc", int(remoteTrack.SSRC())),
					mlog.String("rid", remoteTrack.RID()),
					mlog.String("sessionID", us.cfg.SessionID))
			}

			s.metrics.DecRTPTracks(us.cfg.GroupID, "in", trackMimeType)
		}()

		var screenStreamID string
		if screenSession := call.getScreenSession(); screenSession != nil {
			screenStreamID = screenSession.getScreenStreamID()
		}

		videoStreamID := us.getVideoStreamID()

		go us.handleReceiverRTCP(receiver, remoteTrack.RID())

		if trackMimeType == rtpAudioCodec.MimeType {
			trackType := trackTypeVoice
			if streamID != "" && streamID == screenStreamID {
				s.log.Debug("received screen sharing audio track", mlog.String("sessionID", us.cfg.SessionID))
				trackType = trackTypeScreenAudio
			}

			outAudioTrack, err := webrtc.NewTrackLocalStaticRTP(rtpAudioCodec, genTrackID(trackType, us.cfg.SessionID), random.NewID())
			if err != nil {
				s.log.Error("failed to create local track", mlog.Err(err), mlog.String("sessionID", us.cfg.SessionID))
				return
			}

			call.mut.Lock()
			us.mut.Lock()
			if trackType == trackTypeVoice {
				us.outVoiceTrack = outAudioTrack
				us.outVoiceTrackEnabled = true
				us.remoteVoiceTrack = remoteTrack
			} else {
				us.outScreenAudioTrack = outAudioTrack
				us.remoteScreenAudioTrack = remoteTrack
			}
			us.mut.Unlock()

			call.iterSessions(func(ss *session) {
				// We skip sending the track to the current session (the one sending it) and to any session
				// that hasn't finished initializing its tracks (they are still connecting).
				// This is to avoid queuing duplicate tracks. The call of call.mut.Lock() is needed to guarantee
				// we won't be missing any tracks.
				if ss.cfg.SessionID == us.cfg.SessionID || !ss.tracksInitDone.Load() {
					return
				}

				select {
				case ss.tracksCh <- trackActionContext{
					action:        trackActionAdd,
					localTrack:    outAudioTrack,
					remoteTrack:   remoteTrack,
					senderSession: us,
				}:
				default:
					s.log.Error("failed to send voice track: channel is full",
						mlog.String("userID", ss.cfg.UserID),
						mlog.String("sessionID", ss.cfg.SessionID),
						mlog.String("trackUserID", us.cfg.UserID),
						mlog.String("trackSessionID", us.cfg.SessionID),
					)
				}
			})
			call.mut.Unlock()

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

				// Mitigating against https://github.com/pion/webrtc/issues/2403
				// The padding will be stripped by pion but the header bit will be forwarded as is (set).
				// This causes clients (e.g. calls-transcriber) to fail to decode the packet.
				// Since the payload is empty, we simply reset the padding to 0.
				if packet.Padding && len(packet.Payload) == 0 {
					packet.Padding = false
					packet.PaddingSize = 0
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

				if trackType == trackTypeVoice {
					us.mut.RLock()
					isEnabled := us.outVoiceTrackEnabled
					us.mut.RUnlock()
					if !isEnabled {
						continue
					}
				}

				writeStartTime := time.Now()
				if err := outAudioTrack.WriteRTP(packet); err != nil && !errors.Is(err, io.ErrClosedPipe) {
					s.log.Error("failed to write RTP packet",
						mlog.Err(err), mlog.String("sessionID", us.cfg.SessionID))
					s.metrics.IncRTCErrors(us.cfg.GroupID, "rtp")
					return
				}
				s.metrics.ObserveRTPTracksWrite(us.cfg.GroupID, string(trackType), time.Since(writeStartTime).Seconds())
			}
		} else if params, ok := rtpVideoCodecs[trackMimeType]; ok {
			// We want to ensure the track we are receiving belongs to the expected stream.
			// The expected stream ID (screenStreamID or videoStreamID) is sent in advance by the sender through either the ScreenOn or VideoOn websocket message.
			var outTrackType trackType
			isScreen := screenStreamID != "" && screenStreamID == streamID
			if isScreen {
				s.log.Debug("received screen sharing stream", mlog.String("streamID", streamID), mlog.String("sessionID", us.cfg.SessionID))
				outTrackType = trackTypeScreen
			} else if videoStreamID != "" && videoStreamID == streamID {
				s.log.Debug("received video stream", mlog.String("streamID", streamID), mlog.String("sessionID", us.cfg.SessionID))
				outTrackType = trackTypeVideo
			} else {
				s.log.Error("received unexpected video track",
					mlog.String("streamID", streamID), mlog.String("sessionID", us.cfg.SessionID))
				return
			}

			// To improve concurrency and support larger calls we create NumCPU output tracks and randomly distribute the receivers among them.
			createOutTracks := func(num int) ([]*webrtc.TrackLocalStaticRTP, error) {
				outTracks := make([]*webrtc.TrackLocalStaticRTP, num)
				for i := 0; i < num; i++ {
					outTrack, err := webrtc.NewTrackLocalStaticRTP(params.RTPCodecCapability,
						genTrackID(outTrackType, us.cfg.SessionID), random.NewID(), webrtc.WithRTPStreamID(remoteTrack.RID()))
					if err != nil {
						return nil, fmt.Errorf("failed to create screen track")
					}
					outTracks[i] = outTrack
				}
				return outTracks, nil
			}

			outTracks, err := createOutTracks(runtime.NumCPU())
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

			trackIdx := getTrackIndex(trackMimeType, rid)

			call.mut.Lock()

			us.mut.Lock()
			if isScreen {
				us.outScreenTracks[trackIdx] = outTracks
				us.remoteScreenTracks[trackIdx] = remoteTrack
				us.screenRateMonitors[trackIdx] = rm
			} else {
				us.outVideoTracks[trackIdx] = outTracks
				us.remoteVideoTracks[trackIdx] = remoteTrack
				us.videoRateMonitors[trackIdx] = rm
			}
			us.mut.Unlock()

			call.iterSessions(func(ss *session) {
				// We skip sending the track to the current session (the one sending it) and to any session
				// that hasn't finished initializing its tracks (they are still connecting).
				// This is to avoid queuing duplicate tracks. The call of call.mut.Lock() is needed to guarantee
				// we won't be missing any tracks.
				if ss.cfg.SessionID == us.cfg.SessionID || !ss.tracksInitDone.Load() {
					return
				}

				expectedLevel := SimulcastLevelDefault
				if remoteTrack.RID() != "" {
					expectedLevel = ss.getExpectedSimulcastLevel()
				}

				if rid != expectedLevel {
					return
				}

				if trackMimeType == ScreenTrackMimeTypeDefault && us.supportsAV1() && ss.supportsAV1() {
					s.log.Debug("skipping VP8 track for AV1 supported receiver",
						mlog.String("sessionID", ss.cfg.SessionID),
					)
					return
				} else if trackMimeType == webrtc.MimeTypeAV1 && !ss.supportsAV1() {
					s.log.Debug("skipping AV1 track for unsupported receiver",
						mlog.String("sessionID", ss.cfg.SessionID),
					)
					return
				}

				s.log.Debug("received track matches expected level, sending",
					mlog.String("lvl", expectedLevel),
					mlog.String("sessionID", ss.cfg.SessionID),
					mlog.String("trackMimeType", trackMimeType),
				)

				select {
				case ss.tracksCh <- trackActionContext{
					action:        trackActionAdd,
					localTrack:    pickRandom(outTracks),
					remoteTrack:   remoteTrack,
					senderSession: us,
				}:
				default:
					s.log.Error("failed to send track: channel is full",
						mlog.String("userID", ss.cfg.UserID),
						mlog.String("sessionID", ss.cfg.SessionID),
						mlog.String("trackUserID", us.cfg.UserID),
						mlog.String("trackSessionID", us.cfg.SessionID),
						mlog.String("trackType", outTrackType),
					)
				}
			})
			call.mut.Unlock()

			writeTrack := func(writerCh <-chan *rtp.Packet, outTrack *webrtc.TrackLocalStaticRTP) {
				for pkt := range writerCh {
					writeStartTime := time.Now()
					if err := outTrack.WriteRTP(pkt); err != nil && !errors.Is(err, io.ErrClosedPipe) {
						s.log.Error("failed to write RTP packet",
							mlog.Err(err), mlog.String("sessionID", us.cfg.SessionID))
						s.metrics.IncRTCErrors(us.cfg.GroupID, "rtp")
						continue
					}
					s.metrics.ObserveRTPTracksWrite(us.cfg.GroupID, string(trackTypeScreen), time.Since(writeStartTime).Seconds())
				}
			}

			writerChs := make([]chan *rtp.Packet, len(outTracks))
			for i := 0; i < len(outTracks); i++ {
				writerChs[i] = make(chan *rtp.Packet, writerQueueSize)
				defer close(writerChs[i])
				go writeTrack(writerChs[i], outTracks[i])
			}

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

				// Mitigating against https://github.com/pion/webrtc/issues/2403
				// The padding will be stripped by pion but the header bit will be forwarded as is (set).
				// This causes clients (e.g. calls-transcriber) to fail to decode the packet.
				// Since the payload is empty, we simply reset the padding to 0.
				if packet.Padding && len(packet.Payload) == 0 {
					packet.Padding = false
					packet.PaddingSize = 0
				}

				rm.PushSample(packet.MarshalSize())
				if limiter.Allow() {
					rate, dur := rm.GetRate()
					s.log.Debug("rate monitor",
						mlog.String("sessionID", us.cfg.SessionID),
						mlog.String("RID", rid),
						mlog.Int("rate", rate),
						mlog.Float("duration", dur.Seconds()),
						mlog.Float("totalDuration", rm.GetSamplesDuration().Seconds()),
					)
				}

				for i, writerCh := range writerChs {
					// We need to copy the packet header to keep it race free in case
					// of simulcast as we are dealing with concurrent writers.
					pkt := *packet
					pkt.Header = packet.Header.Clone()
					pkt.Extension = false
					pkt.Extensions = nil

					select {
					case writerCh <- &pkt:
					default:
						s.log.Error("failed to write RTP packet to writer channel", mlog.String("trackID", outTracks[i].ID()))
						s.metrics.IncRTCErrors(us.cfg.GroupID, "rtp")
					}
				}
			}
		}
	})

	go s.handleDCNegotiation(us, call)

	s.log.Debug("session has joined call",
		mlog.String("userID", cfg.UserID),
		mlog.String("sessionID", cfg.SessionID),
		mlog.String("channelID", cfg.Props.ChannelID()),
		mlog.String("callID", cfg.CallID),
	)

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

	s.metrics.DecRTCSessions(cfg.GroupID)

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

	us.mut.Lock()
	close(us.closeCh)
	us.mut.Unlock()
	us.rtcConn.Close()

	// Wait for the signaling goroutines to be done.
	<-us.doneCh

	if us.closeCb != nil {
		return us.closeCb()
	}

	return nil
}

// handleTracks manages (adds and removes) a/v tracks for the peer associated with the session.
func (s *Server) handleTracks(call *call, us *session) {
	call.mut.Lock()
	call.iterSessions(func(ss *session) {
		if ss.cfg.SessionID == us.cfg.SessionID {
			return
		}

		ss.mut.RLock()
		outVoiceTrack := ss.outVoiceTrack

		// Screen track selection. Both sender and receiver support it
		// in order to send out the AV1 track.
		screenTrackMimeType := ScreenTrackMimeTypeDefault
		if ss.supportsAV1() && us.supportsAV1() {
			s.log.Debug("both sender and receiver support AV1", mlog.String("sessionID", us.cfg.SessionID))
			screenTrackMimeType = webrtc.MimeTypeAV1
		}
		outScreenTracks := ss.outScreenTracks[getTrackIndex(screenTrackMimeType, SimulcastLevelDefault)]
		outScreenAudioTrack := ss.outScreenAudioTrack
		outVideoTracks := ss.outVideoTracks[getTrackIndex(screenTrackMimeType, SimulcastLevelDefault)]

		remoteVoiceTrack := ss.remoteVoiceTrack
		remoteScreenTrack := ss.remoteScreenTracks[getTrackIndex(screenTrackMimeType, SimulcastLevelDefault)]
		remoteScreenAudioTrack := ss.remoteScreenAudioTrack
		remoteVideoTrack := ss.remoteVideoTracks[getTrackIndex(screenTrackMimeType, SimulcastLevelDefault)]

		ss.mut.RUnlock()

		var outTracks []*webrtc.TrackLocalStaticRTP
		var remoteTracks []*webrtc.TrackRemote
		if outVoiceTrack != nil {
			outTracks = append(outTracks, outVoiceTrack)
			remoteTracks = append(remoteTracks, remoteVoiceTrack)
		}
		if len(outScreenTracks) > 0 {
			outTracks = append(outTracks, pickRandom(outScreenTracks))
			remoteTracks = append(remoteTracks, remoteScreenTrack)
		}
		if outScreenAudioTrack != nil {
			outTracks = append(outTracks, outScreenAudioTrack)
			remoteTracks = append(remoteTracks, remoteScreenAudioTrack)
		}
		if len(outVideoTracks) > 0 {
			outTracks = append(outTracks, pickRandom(outVideoTracks))
			remoteTracks = append(remoteTracks, remoteVideoTrack)
		}

		for i, track := range outTracks {
			select {
			case us.tracksCh <- trackActionContext{
				action:        trackActionAdd,
				localTrack:    track,
				remoteTrack:   remoteTracks[i],
				senderSession: ss,
			}:
			default:
				s.metrics.IncRTCErrors(us.cfg.GroupID, "track")
				s.log.Error("failed to add track on join: channel is full", mlog.String("sessionID", us.cfg.SessionID))
			}
		}
	})
	us.tracksInitDone.Store(true)
	call.mut.Unlock()

	// Incoming offers handler. This requires a dedicated goroutine since the other handler below could be blocked
	// waiting for the signaling lock which is released by the client upon receiving an answer.
	go func() {
		for {
			select {
			case offerMsg, ok := <-us.sdpOfferInCh:
				if !ok {
					return
				}

				if err := us.signaling(offerMsg.sdp, offerMsg.answerCh); err != nil {
					s.metrics.IncRTCErrors(us.cfg.GroupID, "signaling")
					s.log.Error("failed to signal", mlog.Err(err), mlog.String("sessionID", us.cfg.SessionID))
					continue
				}
			case <-us.closeCh:
				return
			}
		}
	}()

	// Outgoing offers handler
	for {
		select {
		case ctx, ok := <-us.tracksCh:
			if !ok {
				return
			}

			sdpCh := s.receiveCh
			if us.dcSignaling() {
				sdpCh = us.dcSDPCh
			}

			start := time.Now()
			err := us.signalingLock.Lock(signalingLockTimeout)
			s.metrics.ObserveRTCSignalingLockGrabTime(us.cfg.GroupID, time.Since(start).Seconds())
			if err != nil {
				s.log.Error("failed to grab signaling lock", mlog.Err(err), mlog.String("sessionID", us.cfg.SessionID))
				s.metrics.IncRTCErrors(us.cfg.GroupID, "signaling_lock_timeout")
				continue
			}

			start = time.Now()
			s.log.Debug("signaling lock acquired", mlog.String("sessionID", us.cfg.SessionID))

			if ctx.action == trackActionAdd {
				if err := us.addTrack(sdpCh, ctx.localTrack, ctx.remoteTrack, ctx.senderSession); err != nil {
					s.metrics.IncRTCErrors(us.cfg.GroupID, "track")
					s.log.Error("failed to add track", mlog.Err(err), mlog.String("sessionID", us.cfg.SessionID), mlog.String("trackID", ctx.localTrack.ID()))
				}
			} else if ctx.action == trackActionRemove {
				if err := us.removeTrack(sdpCh, ctx.localTrack); err != nil {
					s.metrics.IncRTCErrors(us.cfg.GroupID, "track")
					var trackID string
					if ctx.localTrack != nil {
						trackID = ctx.localTrack.ID()
					}
					s.log.Error("failed to remove track", mlog.Err(err), mlog.String("sessionID", us.cfg.SessionID), mlog.String("trackID", trackID))
				}
			} else {
				s.log.Error("invalid track action", mlog.Int("action", int(ctx.action)), mlog.String("sessionID", us.cfg.SessionID))
			}

			s.log.Debug("releasing signaling lock", mlog.String("sessionID", us.cfg.SessionID))
			if err := us.signalingLock.Unlock(); err != nil {
				s.log.Error("failed to unlock signaling lock", mlog.Err(err), mlog.String("sessionID", us.cfg.SessionID))
			}
			s.metrics.ObserveRTCSignalingLockLockedTime(us.cfg.GroupID, time.Since(start).Seconds())
		case <-us.closeCh:
			return
		}
	}
}
