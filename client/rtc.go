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
	"log"
	"sync/atomic"

	"github.com/pion/webrtc/v3"
)

const (
	signalMsgCandidate = "candidate"
	signalMsgOffer     = "offer"
	signalMsgAnswer    = "answer"

	iceChSize  = 20
	receiveMTU = 1460
)

func (c *Client) handleWSEventSignal(evData map[string]any) error {
	data, ok := evData["data"].(string)
	if !ok {
		return fmt.Errorf("unexpected data type found for signaling data")
	}

	var msg map[string]interface{}
	if err := json.Unmarshal([]byte(data), &msg); err != nil {
		log.Fatalf(err.Error())
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

		log.Printf("received remote candidate %v", candidate)

		if c.pc.RemoteDescription() != nil {
			log.Printf("adding remote candidate")
			if err := c.pc.AddICECandidate(webrtc.ICECandidateInit{Candidate: candidate}); err != nil {
				return fmt.Errorf("failed to add remote candidate: %w", err)
			}
		} else {
			// Candidates cannot be added until the remote description is set, so we
			// queue them until that happens.
			log.Printf("queuing remote candidate")
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

		log.Printf("received sdp offer: %v", sdp)

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

		log.Printf("received sdp answer: %v", sdp)

		if err := c.pc.SetRemoteDescription(webrtc.SessionDescription{
			Type: webrtc.SDPTypeAnswer,
			SDP:  sdp,
		}); err != nil {
			return fmt.Errorf("failed to set remote description: %w", err)
		}

		for i := 0; i < len(c.iceCh); i++ {
			log.Printf("adding queued remote candidate")
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

	pc, err := webrtc.NewPeerConnection(cfg)
	if err != nil {
		return fmt.Errorf("failed to create new peer connection: %s", err)
	}
	c.pc = pc

	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			log.Printf("local ICE gathering completed")
			return
		}

		log.Printf("local candidate: %v", candidate)

		data, err := json.Marshal(candidate.ToJSON())
		if err != nil {
			log.Printf("failed to marshal local candidate: %s", err)
			return
		}

		if err := c.SendWS(wsEventICE, map[string]any{
			"data": string(data),
		}, true); err != nil {
			log.Print(err.Error())
		}
	})

	pc.OnICEConnectionStateChange(func(st webrtc.ICEConnectionState) {
		if st == webrtc.ICEConnectionStateConnected {
			log.Printf("ice connect")
			c.emit(RTCConnectEvent, nil)
		}

		if st == webrtc.ICEConnectionStateDisconnected {
			log.Printf("ice disconnect")
		}

		if st == webrtc.ICEConnectionStateClosed || st == webrtc.ICEConnectionStateFailed {
			log.Printf("ice closed or failed")
			c.emit(RTCDisconnectEvent, nil)

			if atomic.LoadInt32(&c.state) != clientStateInit {
				return
			}

			log.Printf("rtc disconnected, closing")
			if err := c.Close(); err != nil {
				log.Printf("failed to close: %s", err)
			}
		}
	})

	pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		log.Printf("received remote track: %v %v %v", track.PayloadType(), track.Codec(), track.ID())

		trackType, sessionID, err := ParseTrackID(track.ID())
		if err != nil {
			log.Printf("failed to parse track ID: %s", err)
			if err := receiver.Stop(); err != nil {
				log.Printf("failed to stop receiver: %s", err)
			}
			return
		}

		if trackType != TrackTypeVoice {
			// We ignore any non voice track for now.
			log.Printf("ignoring non voice track")
			if err := receiver.Stop(); err != nil {
				log.Printf("failed to stop receiver: %s", err)
			}
			return
		}

		c.mut.Lock()
		c.receivers[sessionID] = receiver
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
						log.Printf("failed to read RTCP packet: %s", err)
					}
					return
				}
			}
		}(track.RID())

		c.emit(RTCTrackEvent, track)
	})

	pc.OnNegotiationNeeded(func() {
		log.Printf("negotiation needed")

		offer, err := pc.CreateOffer(nil)
		if err != nil {
			log.Printf("failed to create offer: %s", err)
			return
		}

		if err := pc.SetLocalDescription(offer); err != nil {
			log.Printf("failed to set local description: %s", err)
			return
		}

		var sdpData bytes.Buffer
		w := zlib.NewWriter(&sdpData)
		if err := json.NewEncoder(w).Encode(offer); err != nil {
			w.Close()
			fmt.Printf("failed to encode offer: %s", err)
			return
		}
		w.Close()
		err = c.SendWS(wsEventSDP, map[string]any{
			"data": sdpData.Bytes(),
		}, true)
		if err != nil {
			log.Print(err.Error())
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
