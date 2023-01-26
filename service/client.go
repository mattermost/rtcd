// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package service

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/mattermost/rtcd/service/ws"
)

const (
	maxReconnectInterval     = 30 * time.Second
	defaultReconnectInterval = 2 * time.Second
)

type Client struct {
	cfg    *ClientConfig
	connID string

	httpClient  *http.Client
	wsClient    *ws.Client
	receiveCh   chan ClientMessage
	errorCh     chan error
	reconnectCb ClientReconnectCb
	dialFn      DialContextFn
	closed      bool

	mut sync.RWMutex
	wg  sync.WaitGroup
}

func NewClient(cfg ClientConfig, opts ...ClientOption) (*Client, error) {
	var c Client

	if err := cfg.Parse(); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}
	c.cfg = &cfg
	c.receiveCh = make(chan ClientMessage, ws.ReceiveChSize)
	c.errorCh = make(chan error, 32)

	for _, opt := range opts {
		if err := opt(&c); err != nil {
			return nil, fmt.Errorf("failed to apply option: %w", err)
		}
	}

	dialFn := (&net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
		DualStack: true,
	}).DialContext

	if c.dialFn != nil {
		dialFn = c.dialFn
	}

	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialFn,
		MaxConnsPerHost:       100,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   100,
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   1 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	c.httpClient = &http.Client{Transport: transport}

	return &c, nil
}

func (c *Client) Register(clientID string, authKey string) error {
	if c.httpClient == nil {
		return fmt.Errorf("http client is not initialized")
	}

	reqData := map[string]string{
		"clientID": clientID,
		"authKey":  authKey,
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(reqData); err != nil {
		return fmt.Errorf("failed to encode body: %w", err)
	}

	req, err := http.NewRequest("POST", c.cfg.httpURL+"/register", &buf)
	if err != nil {
		return fmt.Errorf("failed to build request: %w", err)
	}
	req.SetBasicAuth(c.cfg.ClientID, c.cfg.AuthKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	respData := map[string]string{}
	if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
		return fmt.Errorf("decoding http response failed: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		if errMsg := respData["error"]; errMsg != "" {
			return fmt.Errorf("request failed: %s", errMsg)
		}
		return fmt.Errorf("request failed with status %s", resp.Status)
	}

	return nil
}

func (c *Client) Unregister(clientID string) error {
	if c.httpClient == nil {
		return fmt.Errorf("http client is not initialized")
	}

	reqData := map[string]string{
		"clientID": clientID,
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(reqData); err != nil {
		return fmt.Errorf("failed to encode body: %w", err)
	}

	req, err := http.NewRequest("POST", c.cfg.httpURL+"/unregister", &buf)
	if err != nil {
		return fmt.Errorf("failed to build request: %w", err)
	}
	req.SetBasicAuth(c.cfg.ClientID, c.cfg.AuthKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respData := map[string]string{}
		if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
			return fmt.Errorf("decoding http response failed: %w", err)
		}

		if errMsg := respData["error"]; errMsg != "" {
			return fmt.Errorf("request failed: %s", errMsg)
		}
		return fmt.Errorf("request failed with status %s", resp.Status)
	}

	return nil
}

func (c *Client) Connect() error {
	c.mut.Lock()
	defer c.mut.Unlock()

	if c.closed {
		return fmt.Errorf("ws client is closed")
	}

	if c.wsClient != nil {
		return fmt.Errorf("ws client is already initialized")
	}

	wsClient, err := ws.NewClient(ws.ClientConfig{
		URL:       c.cfg.wsURL,
		AuthToken: base64.StdEncoding.EncodeToString([]byte(c.cfg.ClientID + ":" + c.cfg.AuthKey)),
	}, ws.WithDialFunc(ws.DialContextFn(c.dialFn)))
	if err != nil {
		return fmt.Errorf("failed to create ws client: %w", err)
	}

	c.wsClient = wsClient

	c.wg.Add(2)

	go func() {
		defer c.wg.Done()
		c.msgReader(wsClient)
		c.mut.Lock()
		if c.wsClient != nil {
			c.wsClient = nil
			c.mut.Unlock()
			c.reconnectHandler()
			return
		}
		c.mut.Unlock()
	}()

	go func() {
		defer c.wg.Done()
		for err := range wsClient.ErrorCh() {
			c.sendError(err)
		}
	}()

	return nil
}

func (c *Client) Send(msg ClientMessage) error {
	c.mut.RLock()
	defer c.mut.RUnlock()

	if c.closed {
		return fmt.Errorf("ws client is closed")
	}

	if c.wsClient == nil {
		return fmt.Errorf("ws client is not initialized")
	}

	data, err := msg.Pack()
	if err != nil {
		return fmt.Errorf("failed to pack message: %w", err)
	}

	return c.wsClient.Send(ws.BinaryMessage, data)
}

func (c *Client) ReceiveCh() <-chan ClientMessage {
	return c.receiveCh
}

func (c *Client) ErrorCh() <-chan error {
	return c.errorCh
}

func (c *Client) Close() error {
	if c.httpClient != nil {
		c.httpClient.CloseIdleConnections()
	}

	c.mut.Lock()
	if c.closed {
		c.mut.Unlock()
		return fmt.Errorf("ws client is closed")
	}

	if c.wsClient == nil {
		c.mut.Unlock()
		return fmt.Errorf("ws client is not initialized")
	}

	wsClient := c.wsClient
	c.wsClient = nil
	c.closed = true
	c.mut.Unlock()
	err := wsClient.Close()
	c.wg.Wait()
	close(c.receiveCh)
	close(c.errorCh)
	return err
}

func (c *Client) sendError(err error) {
	c.mut.RLock()
	defer c.mut.RUnlock()
	if c.closed {
		return
	}

	select {
	case c.errorCh <- err:
	default:
		log.Printf("failed to send error: channel is full: %s", err.Error())
	}
}

func (c *Client) msgReader(wsClient *ws.Client) {
	for msg := range wsClient.ReceiveCh() {
		if msg.Type != ws.BinaryMessage {
			c.sendError(fmt.Errorf("unexpected msg type: %d", msg.Type))
			continue
		}

		var cm ClientMessage
		if err := cm.Unpack(msg.Data); err != nil {
			c.sendError(fmt.Errorf("failed to unpack message: %w", err))
			continue
		}

		if cm.Type == ClientMessageHello {
			data, ok := cm.Data.(map[string]string)
			if ok && data["connID"] != "" {
				c.mut.Lock()
				c.connID = data["connID"]
				c.mut.Unlock()
			}
		}

		select {
		case c.receiveCh <- cm:
		default:
			c.sendError(fmt.Errorf("failed to send client message: channel is full"))
		}
	}
}

func (c *Client) reconnectHandler() {
	var attempt int
	var waitTime time.Duration
	for {
		attempt++
		if waitTime < maxReconnectInterval {
			waitTime += c.cfg.ReconnectInterval
		}
		time.Sleep(waitTime)

		if c.reconnectCb != nil {
			if err := c.reconnectCb(c, attempt); err != nil {
				c.sendError(fmt.Errorf("reconnect callback failed: %w", err))
				c.mut.Lock()
				c.closed = true
				c.mut.Unlock()
				close(c.receiveCh)
				close(c.errorCh)
				return
			}
		}

		err := c.Connect()
		if err == nil {
			break
		}

		c.sendError(fmt.Errorf("failed to re-connect: %w", err))
	}
}

func (c *Client) GetVersionInfo() (VersionInfo, error) {
	if c.httpClient == nil {
		return VersionInfo{}, fmt.Errorf("http client is not initialized")
	}

	req, err := http.NewRequest("GET", c.cfg.httpURL+"/version", nil)
	if err != nil {
		return VersionInfo{}, fmt.Errorf("failed to build request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return VersionInfo{}, fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	var info VersionInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return VersionInfo{}, fmt.Errorf("decoding http response failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return VersionInfo{}, fmt.Errorf("request failed with status %s", resp.Status)
	}

	return info, nil
}
