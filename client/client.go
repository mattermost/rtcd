// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package client

import (
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mattermost/rtcd/service/ws"

	"github.com/mattermost/mattermost/server/public/model"

	"github.com/pion/webrtc/v3"
)

type EventHandler func(ctx any) error

type EventType string

const (
	RTCConnectEvent    EventType = "RTCConnect"
	RTCDisconnectEvent EventType = "RTCDisconnect"
	RTCTrackEvent      EventType = "RTCTrack"

	CloseEvent EventType = "Close"
	ErrorEvent EventType = "Error"

	WSConnectEvent            EventType = "WSConnect"
	WSDisconnectEvent         EventType = "WSDisconnect"
	WSCallJoinEvent           EventType = "WSCallJoin"
	WSCallRecordingStateEvent EventType = "WSCallRecordingState" // DEPRECATED
	WSCallJobStateEvent       EventType = "WSCallJobState"
	WSJobStopEvent            EventType = "WSStopJobEvent"
	WSCallHostChangedEvent    EventType = "WSCallHostChanged"
	WSCallMutedEvent          EventType = "WSCallMuted"
	WSCallUnmutedEvent        EventType = "WSCallUnmuted"
	WSCallRaisedHandEvent     EventType = "WSCallRaisedHand"
	WSCallLoweredHandEvent    EventType = "WSCallLoweredHand"
	WSCallScreenOnEvent       EventType = "WSCallScreenOn"
	WSCallScreenOffEvent      EventType = "WSCallScreenOff"
)

func (e EventType) IsValid() bool {
	switch e {
	case RTCConnectEvent, RTCDisconnectEvent, RTCTrackEvent,
		CloseEvent,
		ErrorEvent,
		WSConnectEvent, WSDisconnectEvent,
		WSCallJoinEvent,
		WSCallRecordingStateEvent,
		WSCallHostChangedEvent,
		WSCallUnmutedEvent, WSCallMutedEvent,
		WSCallRaisedHandEvent, WSCallLoweredHandEvent,
		WSCallScreenOnEvent, WSCallScreenOffEvent,
		WSCallJobStateEvent,
		WSJobStopEvent:
		return true
	default:
		return false
	}
}

const (
	clientStateNew int32 = iota
	clientStateInit
	clientStateClosing
	clientStateClosed
)

var (
	ErrAlreadySubscribed = errors.New("already subscribed")
)

// Client is a Golang implementation of a client for Mattermost Calls.
type Client struct {
	cfg Config
	log *slog.Logger

	handlers map[EventType]EventHandler

	// HTTP API
	apiClient *model.Client4

	// WebSocket
	ws                  *ws.Client
	wsDoneCh            chan struct{}
	wsCloseCh           chan struct{}
	wsReconnectInterval time.Duration
	wsLastDisconnect    time.Time
	wsClientSeqNo       int64
	originalConnID      string
	currentConnID       string

	// WebRTC
	pc                *webrtc.PeerConnection
	dc                *webrtc.DataChannel
	iceCh             chan webrtc.ICECandidateInit
	receivers         map[string][]*webrtc.RTPReceiver
	voiceSender       *webrtc.RTPSender
	screenTransceiver *webrtc.RTPTransceiver

	state int32

	mut sync.RWMutex
}

type Option func(c *Client) error

func WithLogger(log *slog.Logger) Option {
	return func(c *Client) error {
		c.log = log
		return nil
	}
}

// New initializes and returns a new Calls client.
func New(cfg Config, opts ...Option) (*Client, error) {
	if err := cfg.Parse(); err != nil {
		return nil, fmt.Errorf("failed to validate config: %w", err)
	}

	apiClient := model.NewAPIv4Client(cfg.SiteURL)
	apiClient.SetToken(cfg.AuthToken)

	c := &Client{
		cfg:           cfg,
		handlers:      make(map[EventType]EventHandler),
		wsDoneCh:      make(chan struct{}),
		wsCloseCh:     make(chan struct{}),
		wsClientSeqNo: 1,
		iceCh:         make(chan webrtc.ICECandidateInit, iceChSize),
		receivers:     make(map[string][]*webrtc.RTPReceiver),
		apiClient:     apiClient,
	}

	for _, opt := range opts {
		if err := opt(c); err != nil {
			return nil, fmt.Errorf("failed to apply option: %w", err)
		}
	}

	if c.log == nil {
		c.log = slog.Default()
	}

	return c, nil
}

// Connect connects a call in the configured channel.
func (c *Client) Connect() error {
	c.mut.Lock()
	defer c.mut.Unlock()

	if !atomic.CompareAndSwapInt32(&c.state, clientStateNew, clientStateInit) {
		return fmt.Errorf("ws client is already initialized")
	}

	if err := c.wsOpen(); err != nil {
		return err
	}

	go c.wsReader()

	return nil
}

// Close permanently disconnects the client.
func (c *Client) Close() error {
	c.mut.RLock()
	if !atomic.CompareAndSwapInt32(&c.state, clientStateInit, clientStateClosing) {
		c.mut.RUnlock()
		return fmt.Errorf("client is not initialized")
	}
	c.mut.RUnlock()

	close(c.wsCloseCh)
	<-c.wsDoneCh

	if err := c.ws.Close(); err != nil {
		return fmt.Errorf("failed to close ws: %w", err)
	}

	c.close()

	return nil
}

// On is used to subscribe to any events fired by the client.
// Note: there can only be one subscriber per event type.
func (c *Client) On(eventType EventType, h EventHandler) error {
	if !eventType.IsValid() {
		return fmt.Errorf("invalid event type %q", eventType)
	}

	c.mut.Lock()
	defer c.mut.Unlock()

	if _, ok := c.handlers[eventType]; ok {
		return ErrAlreadySubscribed
	}

	c.handlers[eventType] = h

	return nil
}

func (c *Client) emit(eventType EventType, ctx any) {
	c.mut.RLock()
	handler := c.handlers[eventType]
	c.mut.RUnlock()
	if handler != nil {
		if err := handler(ctx); err != nil {
			c.log.Error("failed to handle event",
				slog.Any("type", eventType), slog.String("err", err.Error()))
		}
	}
}

func (c *Client) close() {
	atomic.StoreInt32(&c.state, clientStateClosed)

	if c.pc != nil {
		if err := c.pc.Close(); err != nil {
			c.log.Error("failed to close peer connection", slog.String("err", err.Error()))
		} else {
			c.log.Debug("pc closed successfully")
		}
	}

	c.emit(CloseEvent, nil)
}
