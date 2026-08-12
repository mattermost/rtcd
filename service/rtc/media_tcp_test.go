// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/mattermost/rtcd/logger"
	"github.com/mattermost/rtcd/service/perf"
	"github.com/mattermost/rtcd/service/random"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"

	"github.com/stretchr/testify/require"
)

type testPeer struct {
	tb        testing.TB
	cfg       SessionConfig
	pc        *webrtc.PeerConnection
	s         *Server
	receiveCh chan Message

	connectedCh chan struct{}
	trackCh     chan *webrtc.TrackRemote
	closeOnce   sync.Once
}

// newTestPeer connects a client peer connection to the given server, driving the
// full signaling exchange (including server initiated renegotiations).
// networkTypes controls which ICE transports the client is allowed to use, which
// lets us pin a session to ICE-TCP only.
func newTestPeer(tb testing.TB, s *Server, cfg SessionConfig, networkTypes []webrtc.NetworkType) *testPeer {
	tb.Helper()

	sEngine := webrtc.SettingEngine{}
	sEngine.SetNetworkTypes(networkTypes)
	sEngine.SetIncludeLoopbackCandidate(true)
	sEngine.SetICEMulticastDNSMode(0)

	mEngine, err := initMediaEngine()
	require.NoError(tb, err)

	api := webrtc.NewAPI(webrtc.WithSettingEngine(sEngine), webrtc.WithMediaEngine(mEngine))

	pc, err := api.NewPeerConnection(webrtc.Configuration{})
	require.NoError(tb, err)

	p := &testPeer{
		tb:          tb,
		cfg:         cfg,
		pc:          pc,
		s:           s,
		receiveCh:   make(chan Message, 64),
		connectedCh: make(chan struct{}),
		trackCh:     make(chan *webrtc.TrackRemote, 8),
	}

	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		if state == webrtc.PeerConnectionStateConnected {
			p.closeOnce.Do(func() { close(p.connectedCh) })
		}
	})

	pc.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		select {
		case p.trackCh <- track:
		default:
		}
	})

	// rtcd only starts handling tracks once the signaling data channel is open.
	dc, err := pc.CreateDataChannel("calls-dc", nil)
	require.NoError(tb, err)
	require.NotNil(tb, dc)

	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			return
		}

		iceData, err := json.Marshal(candidate.ToJSON())
		if err != nil {
			return
		}

		p.send(ICEMessage, iceData)
	})

	return p
}

func (p *testPeer) send(msgType MessageType, data []byte) {
	err := p.s.Send(Message{
		GroupID:   p.cfg.GroupID,
		CallID:    p.cfg.CallID,
		UserID:    p.cfg.UserID,
		SessionID: p.cfg.SessionID,
		Type:      msgType,
		Data:      data,
	})
	require.NoError(p.tb, err)
}

// signal handles a single message coming from the server.
func (p *testPeer) signal(msg Message) {
	switch msg.Type {
	case ICEMessage:
		data := make(map[string]any)
		require.NoError(p.tb, json.Unmarshal(msg.Data, &data))
		candidate := data["candidate"].(map[string]any)["candidate"].(string)
		require.NoError(p.tb, p.pc.AddICECandidate(webrtc.ICECandidateInit{Candidate: candidate}))
	case SDPMessage:
		var sdp webrtc.SessionDescription
		require.NoError(p.tb, json.Unmarshal(msg.Data, &sdp))
		require.NoError(p.tb, p.pc.SetRemoteDescription(sdp))

		if sdp.Type != webrtc.SDPTypeOffer {
			return
		}

		answer, err := p.pc.CreateAnswer(nil)
		require.NoError(p.tb, err)
		require.NoError(p.tb, p.pc.SetLocalDescription(answer))

		answerData, err := json.Marshal(&answer)
		require.NoError(p.tb, err)
		p.send(SDPMessage, answerData)
	}
}

// connect sends the initial offer and waits for the peer connection to be established.
func (p *testPeer) connect() {
	p.tb.Helper()

	offer, err := p.pc.CreateOffer(nil)
	require.NoError(p.tb, err)
	require.NoError(p.tb, p.pc.SetLocalDescription(offer))

	offerData, err := json.Marshal(&offer)
	require.NoError(p.tb, err)
	p.send(SDPMessage, offerData)

	for {
		select {
		case msg := <-p.receiveCh:
			p.signal(msg)
		case <-p.connectedCh:
			return
		case <-time.After(15 * time.Second):
			require.FailNow(p.tb, "timed out connecting", "sessionID=%s", p.cfg.SessionID)
		}
	}
}

// pump keeps handling signaling messages until the returned function is called.
func (p *testPeer) pump() func() {
	doneCh := make(chan struct{})
	stoppedCh := make(chan struct{})

	go func() {
		defer close(stoppedCh)
		for {
			select {
			case msg := <-p.receiveCh:
				p.signal(msg)
			case <-doneCh:
				return
			}
		}
	}()

	return func() {
		close(doneCh)
		<-stoppedCh
	}
}

// TestMediaICETCP verifies audio is forwarded between two sessions that can only
// connect through ICE-TCP (e.g. when UDP is blocked by a firewall).
func TestMediaICETCP(t *testing.T) {
	tcpOnly := []webrtc.NetworkType{webrtc.NetworkTypeTCP4}

	log, err := logger.New(logger.Config{
		EnableConsole: true,
		ConsoleLevel:  "WARN",
	})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, log.Shutdown())
	}()

	metrics := perf.NewMetrics("rtcd", nil)

	s, err := NewServer(ServerConfig{
		ICEPortUDP:      30443,
		ICEPortTCP:      30443,
		UDPSocketsCount: GetDefaultUDPListeningSocketsCount(),
	}, log, metrics)
	require.NoError(t, err)

	require.NoError(t, s.Start())
	defer func() {
		require.NoError(t, s.Stop())
	}()

	groupID := random.NewID()
	callID := random.NewID()

	senderCfg := SessionConfig{
		GroupID:   groupID,
		CallID:    callID,
		UserID:    random.NewID(),
		SessionID: random.NewID(),
	}
	receiverCfg := SessionConfig{
		GroupID:   groupID,
		CallID:    callID,
		UserID:    random.NewID(),
		SessionID: random.NewID(),
	}

	sender := newTestPeer(t, s, senderCfg, tcpOnly)
	defer sender.pc.Close()
	receiver := newTestPeer(t, s, receiverCfg, tcpOnly)
	defer receiver.pc.Close()

	peers := map[string]*testPeer{
		senderCfg.SessionID:   sender,
		receiverCfg.SessionID: receiver,
	}

	go func() {
		for msg := range s.ReceiveCh() {
			if p := peers[msg.SessionID]; p != nil {
				select {
				case p.receiveCh <- msg:
				default:
				}
			}
		}
	}()

	require.NoError(t, s.InitSession(senderCfg, nil))
	require.NoError(t, s.InitSession(receiverCfg, nil))

	voiceTrack, err := webrtc.NewTrackLocalStaticRTP(rtpAudioCodec, "voice", "voice_"+random.NewID())
	require.NoError(t, err)
	_, err = sender.pc.AddTrack(voiceTrack)
	require.NoError(t, err)

	_, err = receiver.pc.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio)
	require.NoError(t, err)

	sender.connect()
	receiver.connect()

	// Make sure we are actually testing what we think we are.
	for _, p := range []*testPeer{sender, receiver} {
		pair, err := p.pc.SCTP().Transport().ICETransport().GetSelectedCandidatePair()
		require.NoError(t, err)
		require.NotNil(t, pair)
		require.Equal(t, webrtc.ICEProtocolTCP, pair.Local.Protocol)
		require.Equal(t, webrtc.ICEProtocolTCP, pair.Remote.Protocol)
	}

	stopSender := sender.pump()
	defer stopSender()
	stopReceiver := receiver.pump()
	defer stopReceiver()

	// Keep sending audio packets until the receiver gets the forwarded track.
	writeDoneCh := make(chan struct{})
	defer close(writeDoneCh)
	go func() {
		var seq uint16
		var ts uint32
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-writeDoneCh:
				return
			case <-ticker.C:
				seq++
				ts += 960
				_ = voiceTrack.WriteRTP(&rtp.Packet{
					Header: rtp.Header{
						Version:        2,
						SequenceNumber: seq,
						Timestamp:      ts,
					},
					Payload: []byte{0xDE, 0xAD, 0xBE, 0xEF},
				})
			}
		}
	}()

	var track *webrtc.TrackRemote
	select {
	case track = <-receiver.trackCh:
	case <-time.After(20 * time.Second):
		require.FailNow(t, "timed out waiting for forwarded track")
	}
	require.Equal(t, rtpAudioCodec.MimeType, track.Codec().MimeType)

	require.NoError(t, track.SetReadDeadline(time.Now().Add(20*time.Second)))
	pkt, _, err := track.ReadRTP()
	require.NoError(t, err, "no RTP packet was forwarded over ICE-TCP")
	require.Equal(t, []byte{0xDE, 0xAD, 0xBE, 0xEF}, pkt.Payload)

	require.NoError(t, s.CloseSession(senderCfg.SessionID))
	require.NoError(t, s.CloseSession(receiverCfg.SessionID))
}
