// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package client

import (
	"bytes"
	"compress/zlib"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/mattermost/rtcd/service/rtc/dc"

	"github.com/pion/interceptor"
	"github.com/pion/interceptor/pkg/stats"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"
)

const (
	signalMsgCandidate = "candidate"
	signalMsgOffer     = "offer"
	signalMsgAnswer    = "answer"

	iceChSize          = 20
	receiveMTU         = 1460
	rtcMonitorInterval = 4 * time.Second
	pingInterval       = time.Second
)

var rtpVideoExtensions = []string{
	"urn:ietf:params:rtp-hdrext:sdes:mid",
	"urn:ietf:params:rtp-hdrext:sdes:rtp-stream-id",
	"urn:ietf:params:rtp-hdrext:sdes:repaired-rtp-stream-id",
}

func (c *Client) handleWSEventSignal(evData map[string]any) error {
	data, ok := evData["data"].(string)
	if !ok {
		return fmt.Errorf("unexpected data type found for signaling data")
	}

	var msg map[string]interface{}
	if err := json.Unmarshal([]byte(data), &msg); err != nil {
		return fmt.Errorf("failed to unmarshal signal data: %w", err)
	}

	msgType, ok := msg["type"].(string)
	if !ok {
		return fmt.Errorf("unexpected msg type found in signaling data")
	}

	switch msgType {
	case signalMsgCandidate:
		wrapper, ok := msg["candidate"].(map[string]interface{})
		if !ok {
			return fmt.Errorf("invalid candidate format found")
		}

		candidate, ok := wrapper["candidate"].(string)
		if !ok {
			return fmt.Errorf("invalid candidate format found")
		}

		c.log.Debug("received remote candidate", slog.Any("candidate", candidate))

		if c.pc.RemoteDescription() != nil {
			c.log.Debug("adding remote candidate")
			if err := c.pc.AddICECandidate(webrtc.ICECandidateInit{Candidate: candidate}); err != nil {
				return fmt.Errorf("failed to add remote candidate: %w", err)
			}
		} else {
			// Candidates cannot be added until the remote description is set, so we
			// queue them until that happens.
			c.log.Debug("queuing remote candidate")
			select {
			case c.iceCh <- webrtc.ICECandidateInit{Candidate: candidate}:
			default:
				return fmt.Errorf("failed to queue candidate")
			}
		}
	case signalMsgOffer:
		sdp, ok := msg["sdp"].(string)
		if !ok {
			return fmt.Errorf("invalid SDP data received")
		}

		return c.handleOffer(sdp)
	case signalMsgAnswer:
		sdp, ok := msg["sdp"].(string)
		if !ok {
			return fmt.Errorf("invalid SDP data received")
		}

		return c.handleAnswer(sdp)
	default:
		return fmt.Errorf("invalid signaling msg type %s", msgType)
	}

	return nil
}

func (c *Client) handleAnswer(sdp string) error {
	c.log.Debug("received sdp answer", slog.Any("sdp", sdp))

	if err := c.pc.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  sdp,
	}); err != nil {
		return fmt.Errorf("failed to set remote description: %w", err)
	}

	for i := 0; i < len(c.iceCh); i++ {
		c.log.Debug("adding queued remote candidate")
		if err := c.pc.AddICECandidate(<-c.iceCh); err != nil {
			return fmt.Errorf("failed to add remote candidate: %w", err)
		}
	}

	if c.dcNegotiated.Load() {
		c.log.Debug("unlocking signaling lock")
		if err := c.unlockSignalingLock(); err != nil {
			c.log.Error("failed to unlock signaling lock", slog.String("err", err.Error()))
		}
	} else {
		c.dcNegotiated.Store(true)
	}

	return nil
}

func (c *Client) handleOffer(sdp string) error {
	c.log.Debug("received sdp offer", slog.Any("sdp", sdp))

	if err := c.pc.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  sdp,
	}); err != nil {
		return fmt.Errorf("failed to set remote description: %w", err)
	}

	answer, err := c.pc.CreateAnswer(nil)
	if err != nil {
		return fmt.Errorf("failed to create answer: %w", err)
	}

	if err := c.pc.SetLocalDescription(answer); err != nil {
		return fmt.Errorf("failed to set local description: %w", err)
	}

	if dataCh := c.dc.Load(); c.cfg.EnableDCSignaling && dataCh != nil && dataCh.ReadyState() == webrtc.DataChannelStateOpen {
		c.log.Debug("sending answer through dc")
		data, err := json.Marshal(answer)
		if err != nil {
			return fmt.Errorf("failed to marshal answer: %w", err)
		}

		msg, err := dc.EncodeMessage(dc.MessageTypeSDP, data)
		if err != nil {
			return fmt.Errorf("failed to encode dc message: %w", err)
		}

		return dataCh.Send(msg)
	}

	if c.cfg.EnableDCSignaling {
		c.log.Debug("dc not connected, sending answer through ws")
	}

	var sdpData bytes.Buffer
	w := zlib.NewWriter(&sdpData)
	if err := json.NewEncoder(w).Encode(answer); err != nil {
		w.Close()
		return fmt.Errorf("failed to encode answer: %w", err)
	}
	w.Close()

	return c.SendWS(wsEventSDP, map[string]any{
		"data": sdpData.Bytes(),
	}, true)
}

func (c *Client) initRTCSession() error {
	cfg := webrtc.Configuration{
		ICEServers:   []webrtc.ICEServer{}, // TODO: consider loading ICE servers from config
		SDPSemantics: webrtc.SDPSemanticsUnifiedPlan,
	}

	var m webrtc.MediaEngine
	if err := m.RegisterDefaultCodecs(); err != nil {
		return fmt.Errorf("failed to register default codecs: %w", err)
	}

	i := interceptor.Registry{}

	var statsGetter stats.Getter
	if c.cfg.EnableRTCMonitor {
		statsInterceptorFactory, err := stats.NewInterceptor()
		if err != nil {
			return fmt.Errorf("failed to create stats interceptor: %w", err)
		}
		statsInterceptorFactory.OnNewPeerConnection(func(_ string, g stats.Getter) {
			statsGetter = g
		})
		i.Add(statsInterceptorFactory)
	}

	if err := webrtc.RegisterDefaultInterceptors(&m, &i); err != nil {
		return fmt.Errorf("failed to register default interceptors: %w", err)
	}

	for _, ext := range rtpVideoExtensions {
		if err := m.RegisterHeaderExtension(webrtc.RTPHeaderExtensionCapability{URI: ext}, webrtc.RTPCodecTypeVideo); err != nil {
			return fmt.Errorf("failed to register header extension: %w", err)
		}
	}

	s := webrtc.SettingEngine{}
	s.EnableSCTPZeroChecksum(true)

	api := webrtc.NewAPI(webrtc.WithMediaEngine(&m), webrtc.WithInterceptorRegistry(&i), webrtc.WithSettingEngine(s))

	pc, err := api.NewPeerConnection(cfg)
	if err != nil {
		return fmt.Errorf("failed to create new peer connection: %s", err)
	}
	c.mut.Lock()
	c.pc = pc
	c.mut.Unlock()

	rtcMon := newRTCMonitor(c.log, pc, statsGetter, rtcMonitorInterval)
	if c.cfg.EnableRTCMonitor {
		c.mut.Lock()
		c.rtcMon = rtcMon
		c.mut.Unlock()
		rtcMon.Start()
	}

	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			c.log.Debug("local ICE gathering completed")
			return
		}

		c.log.Debug("local candidate", slog.Any("candidate", candidate))

		data, err := json.Marshal(candidate.ToJSON())
		if err != nil {
			c.log.Error("failed to marshal local candidate", slog.String("err", err.Error()))
			return
		}

		if err := c.SendWS(wsEventICE, map[string]any{
			"data": string(data),
		}, true); err != nil {
			c.log.Error("failed to send ws msg", slog.String("err", err.Error()))
		}
	})

	pc.OnICEConnectionStateChange(func(st webrtc.ICEConnectionState) {
		if st == webrtc.ICEConnectionStateConnected {
			c.log.Debug("ice connect")
			c.emit(RTCConnectEvent, nil)
		}

		if st == webrtc.ICEConnectionStateDisconnected {
			c.log.Debug("ice disconnect")
		}

		if st == webrtc.ICEConnectionStateClosed || st == webrtc.ICEConnectionStateFailed {
			c.log.Debug("ice closed or failed")
			c.emit(RTCDisconnectEvent, nil)

			if atomic.LoadInt32(&c.state) != clientStateInit {
				return
			}

			c.log.Debug("rtc disconnected, closing")
			if err := c.Close(); err != nil {
				c.log.Error("failed to close", slog.String("err", err.Error()))
			}
		}
	})

	pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		c.log.Debug("received remote track",
			slog.Any("payload", track.PayloadType()),
			slog.Any("codec", track.Codec()),
			slog.Any("id", track.ID()))

		trackType, sessionID, err := ParseTrackID(track.ID())
		if err != nil {
			c.log.Error("failed to parse track ID", slog.String("err", err.Error()))
			if err := receiver.Stop(); err != nil {
				c.log.Error("failed to stop receiver", slog.String("err", err.Error()))
			}
			return
		}

		if trackType != TrackTypeVoice && trackType != TrackTypeScreen && trackType != TrackTypeVideo {
			c.log.Debug("ignoring unsupported track type", slog.Any("trackType", trackType))
			if err := receiver.Stop(); err != nil {
				c.log.Error("failed to stop receiver", slog.String("err", err.Error()))
			}
			return
		}

		if trackType == TrackTypeScreen {
			c.log.Debug("sending PLI request for received screen track", slog.String("trackID", track.ID()), slog.Any("SSRC", track.SSRC()))
			if err := pc.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: uint32(track.SSRC())}}); err != nil {
				c.log.Error("failed to write RTCP packet", slog.String("err", err.Error()))
			}
		}

		c.mut.Lock()
		c.receivers[sessionID] = append(c.receivers[sessionID], receiver)
		c.mut.Unlock()

		// RTCP handler
		go func(rid string) {
			var err error
			rtcpBuf := make([]byte, receiveMTU)
			for {
				if rid != "" {
					_, _, err = receiver.ReadSimulcast(rtcpBuf, rid)
				} else {
					_, _, err = receiver.Read(rtcpBuf)
				}
				if err != nil {
					if !errors.Is(err, io.EOF) {
						c.log.Error("failed to read RTCP packet", slog.String("err", err.Error()))
					}
					return
				}
			}
		}(track.RID())

		c.emit(RTCTrackEvent, map[string]any{
			"track":    track,
			"receiver": receiver,
		})
	})

	onNegotiationNeeded := func() {
		offer, err := pc.CreateOffer(nil)
		if err != nil {
			c.log.Error("failed to create offer", slog.String("err", err.Error()))
			return
		}

		if err := pc.SetLocalDescription(offer); err != nil {
			c.log.Error("failed to set local description", slog.String("err", err.Error()))
			c.emit(ErrorEvent, err)
			return
		}

		if dataCh := c.dc.Load(); c.cfg.EnableDCSignaling && dataCh != nil && dataCh.ReadyState() == webrtc.DataChannelStateOpen {
			c.log.Debug("sending offer through dc")
			data, err := json.Marshal(offer)
			if err != nil {
				c.log.Error("failed to marshal offer", slog.String("err", err.Error()))
				return
			}

			msg, err := dc.EncodeMessage(dc.MessageTypeSDP, data)
			if err != nil {
				c.log.Error("failed to encode dc message", slog.String("err", err.Error()))
				return
			}

			if err := dataCh.Send(msg); err != nil {
				c.log.Error("failed to send on dc", slog.String("err", err.Error()))
			}
		} else {
			if c.cfg.EnableDCSignaling {
				c.log.Debug("dc not connected, sending offer through ws")
			}

			var sdpData bytes.Buffer
			w := zlib.NewWriter(&sdpData)
			if err := json.NewEncoder(w).Encode(offer); err != nil {
				w.Close()
				c.log.Error("failed to encode offer", slog.String("err", err.Error()))
				return
			}
			w.Close()
			err = c.SendWS(wsEventSDP, map[string]any{
				"data": sdpData.Bytes(),
			}, true)
			if err != nil {
				c.log.Error("failed to send ws msg", slog.String("err", err.Error()))
				return
			}
		}
	}

	pc.OnNegotiationNeeded(func() {
		c.log.Debug("negotiation needed")

		if !c.dcNegotiationStarted.Load() {
			onNegotiationNeeded()
			c.dcNegotiationStarted.Store(true)
			return
		}

		go func() {
			if err := c.grabSignalingLock(); err != nil {
				c.log.Error("failed to grab signaling lock", slog.String("err", err.Error()))
				return
			}
			onNegotiationNeeded()
		}()
	})

	// DC creation must happen after OnNegotiationNeeded has been registered
	// to avoid races between dc initialization and initial offer.
	dataCh, err := pc.CreateDataChannel("calls-dc", nil)
	if err != nil {
		return fmt.Errorf("failed to create data channel: %w", err)
	}
	c.dc.Store(dataCh)

	lastPingTS := new(int64)
	lastRTT := new(int64)
	go func() {
		pingTicker := time.NewTicker(pingInterval)
		for {
			select {
			case <-pingTicker.C:
				msg, err := dc.EncodeMessage(dc.MessageTypePing, nil)
				if err != nil {
					c.log.Error("failed to encode ping msg", slog.String("err", err.Error()))
					continue
				}

				if err := dataCh.Send(msg); err != nil {
					c.log.Error("failed to send ping msg", slog.String("err", err.Error()))
					continue
				}

				atomic.StoreInt64(lastPingTS, time.Now().UnixMilli())
			case stats := <-rtcMon.StatsCh():
				c.log.Debug("rtc stats",
					slog.Float64("lossRate", stats.lossRate),
					slog.Int64("rtt", atomic.LoadInt64(lastRTT)),
					slog.Float64("jitter", stats.jitter))

				if stats.lossRate >= 0 {
					msg, err := dc.EncodeMessage(dc.MessageTypeLossRate, stats.lossRate)
					if err != nil {
						c.log.Error("failed to encode loss rate msg", slog.String("err", err.Error()))
					} else {
						if err := dataCh.Send(msg); err != nil {
							c.log.Error("failed to send loss rate msg", slog.String("err", err.Error()))
						}
					}
				}

				if rtt := atomic.LoadInt64(lastRTT); rtt > 0 {
					msg, err := dc.EncodeMessage(dc.MessageTypeRoundTripTime, float64(rtt/1000))
					if err != nil {
						c.log.Error("failed to encode rtt msg", slog.String("err", err.Error()))
					} else {
						if err := dataCh.Send(msg); err != nil {
							c.log.Error("failed to send rtt msg", slog.String("err", err.Error()))
						}
					}
				}

				if stats.jitter > 0 {
					msg, err := dc.EncodeMessage(dc.MessageTypeJitter, stats.jitter)
					if err != nil {
						c.log.Error("failed to encode jitter msg", slog.String("err", err.Error()))
					} else {
						if err := dataCh.Send(msg); err != nil {
							c.log.Error("failed to send jitter msg", slog.String("err", err.Error()))
						}
					}
				}
			case <-c.wsCloseCh:
				return
			}
		}
	}()

	dataCh.OnMessage(func(msg webrtc.DataChannelMessage) {
		mt, payload, err := dc.DecodeMessage(msg.Data)
		if err != nil {
			c.log.Error("failed to decode dc message", slog.String("err", err.Error()))
			return
		}

		switch mt {
		case dc.MessageTypePong:
			if ts := atomic.LoadInt64(lastPingTS); ts > 0 {
				atomic.StoreInt64(lastRTT, time.Now().UnixMilli()-ts)
			}
		case dc.MessageTypeSDP:
			var sdp webrtc.SessionDescription
			if err := json.Unmarshal(payload.([]byte), &sdp); err != nil {
				c.log.Error("failed to unmarshal sdp", slog.String("err", err.Error()))
				return
			}
			c.log.Debug("received sdp through DC", slog.String("sdp", sdp.SDP))
			if sdp.Type == webrtc.SDPTypeOffer {
				if err := c.handleOffer(sdp.SDP); err != nil {
					c.log.Error("failed to offer", slog.String("err", err.Error()))
				}
			} else if sdp.Type == webrtc.SDPTypeAnswer {
				if err := c.handleAnswer(sdp.SDP); err != nil {
					c.log.Error("failed to answer", slog.String("err", err.Error()))
				}
			}
		case dc.MessageTypeLock:
			locked := payload.(bool)
			c.log.Debug("received lock message", slog.Bool("locked", locked))
			select {
			case c.dcLockedCh <- locked:
			default:
				c.log.Error("dcLockedCh is full")
			}
		case dc.MessageTypeMediaMap:
			c.log.Debug("received media map message", slog.Any("payload", payload))
		case dc.MessageTypeCodecSupportMap:
			c.log.Debug("received codec support map message", slog.Any("payload", payload))
			if csm, ok := payload.(dc.CodecSupportMap); ok {
				c.codecSupportMap.Store(&csm)
			}
		default:
			c.log.Error("unexpected dc message type", slog.Any("mt", mt))
		}
	})

	return nil
}

func (c *Client) CodecSupportMap() *dc.CodecSupportMap {
	return c.codecSupportMap.Load()
}

func (c *Client) unlockSignalingLock() error {
	dataCh := c.dc.Load()
	if dataCh == nil || dataCh.ReadyState() != webrtc.DataChannelStateOpen {
		return fmt.Errorf("dc not connected")
	}
	msg, err := dc.EncodeMessage(dc.MessageTypeUnlock, nil)
	if err != nil {
		return fmt.Errorf("failed to encode unlock msg: %w", err)
	}
	if err := dataCh.Send(msg); err != nil {
		return fmt.Errorf("failed to send unlock msg: %w", err)
	}

	return nil
}

func (c *Client) grabSignalingLock() error {
	timeoutCh := time.After(5 * time.Second)

	for {
		c.log.Debug("attempting to grab signaling lock")
		dataCh := c.dc.Load()
		if dataCh == nil || dataCh.ReadyState() != webrtc.DataChannelStateOpen {
			return fmt.Errorf("dc not connected")
		}
		msg, err := dc.EncodeMessage(dc.MessageTypeLock, nil)
		if err != nil {
			return fmt.Errorf("failed to encode lock msg: %w", err)
		}
		if err := dataCh.Send(msg); err != nil {
			return fmt.Errorf("failed to send lock msg: %w", err)
		}

		select {
		case locked := <-c.dcLockedCh:
			if locked {
				c.log.Debug("dc lock acquired")
				return nil
			}
			c.log.Debug("dc lock not acquired, retrying")
			time.Sleep(100 * time.Millisecond)
			continue
		case <-timeoutCh:
			return fmt.Errorf("failed to lock dc: timed out")
		case <-c.wsCloseCh:
			return fmt.Errorf("closing")
		}
	}
}
