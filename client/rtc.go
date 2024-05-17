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

	"github.com/pion/interceptor"
	"github.com/pion/webrtc/v3"
)

const (
	signalMsgCandidate = "candidate"
	signalMsgOffer     = "offer"
	signalMsgAnswer    = "answer"

	iceChSize  = 20
	receiveMTU = 1460
)

var (
	rtpVideoExtensions = []string{
		"urn:ietf:params:rtp-hdrext:sdes:mid",
		"urn:ietf:params:rtp-hdrext:sdes:rtp-stream-id",
		"urn:ietf:params:rtp-hdrext:sdes:repaired-rtp-stream-id",
	}
)

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
	case signalMsgAnswer:
		sdp, ok := msg["sdp"].(string)
		if !ok {
			return fmt.Errorf("invalid SDP data received")
		}

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
	default:
		return fmt.Errorf("invalid signaling msg type %s", msgType)
	}

	return nil
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
	if err := webrtc.RegisterDefaultInterceptors(&m, &i); err != nil {
		return fmt.Errorf("failed to register default interceptors: %w", err)
	}

	for _, ext := range rtpVideoExtensions {
		if err := m.RegisterHeaderExtension(webrtc.RTPHeaderExtensionCapability{URI: ext}, webrtc.RTPCodecTypeVideo); err != nil {
			return fmt.Errorf("failed to register header extension: %w", err)
		}
	}

	api := webrtc.NewAPI(webrtc.WithMediaEngine(&m), webrtc.WithInterceptorRegistry(&i))

	pc, err := api.NewPeerConnection(cfg)
	if err != nil {
		return fmt.Errorf("failed to create new peer connection: %s", err)
	}
	c.mut.Lock()
	c.pc = pc
	c.mut.Unlock()

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

		if trackType != TrackTypeVoice && trackType != TrackTypeScreen {
			c.log.Debug("ignoring unsupported track type", slog.Any("trackType", trackType))
			if err := receiver.Stop(); err != nil {
				c.log.Error("failed to stop receiver", slog.String("err", err.Error()))
			}
			return
		}

		c.mut.Lock()
		c.receivers[sessionID] = append(c.receivers[sessionID], receiver)
		c.mut.Unlock()

		// RTCP handler
		go func(rid string) {
			for {
				rtcpBuf := make([]byte, receiveMTU)
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

	pc.OnNegotiationNeeded(func() {
		c.log.Debug("negotiation needed")

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
	})

	dc, err := pc.CreateDataChannel("calls-dc", nil)
	if err != nil {
		return fmt.Errorf("failed to create data channel: %w", err)
	}
	c.dc = dc

	return nil
}
