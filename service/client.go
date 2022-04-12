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
	msgChSize = 64
)

type Client struct {
	cfg *ClientConfig

	httpClient *http.Client
	wsClient   *ws.Client
	receiveCh  chan ClientMessage
	errorCh    chan error
}

func NewClient(cfg ClientConfig) (*Client, error) {
	var c Client

	if err := cfg.Parse(); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}
	c.cfg = &cfg

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}).DialContext,
		MaxConnsPerHost:       100,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   100,
		ResponseHeaderTimeout: 10 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   1 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	c.httpClient = &http.Client{Transport: transport}

	return &c, nil
}

func (c *Client) Register(clientID string) (string, error) {
	if c.httpClient == nil {
		return "", fmt.Errorf("http client is not initialized")
	}

	reqData := map[string]string{
		"clientID": clientID,
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(reqData); err != nil {
		return "", fmt.Errorf("failed to encode body: %w", err)
	}

	req, err := http.NewRequest("POST", c.cfg.httpURL+"/register", &buf)
	if err != nil {
		return "", fmt.Errorf("failed to build request: %w", err)
	}
	req.SetBasicAuth(c.cfg.ClientID, c.cfg.AuthKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	respData := map[string]string{}
	if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
		return "", fmt.Errorf("decoding http response failed: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		if errMsg := respData["error"]; errMsg != "" {
			return "", fmt.Errorf("request failed: %s", errMsg)
		}
		return "", fmt.Errorf("request failed with status %s", resp.Status)
	}

	authKey := respData["authKey"]
	if authKey == "" {
		return "", fmt.Errorf("unexpected empty auth key")
	}

	return authKey, nil
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
	if c.wsClient != nil {
		return fmt.Errorf("ws client is already initialized")
	}

	wsClient, err := ws.NewClient(ws.ClientConfig{
		URL:       c.cfg.wsURL,
		AuthToken: base64.StdEncoding.EncodeToString([]byte(c.cfg.ClientID + ":" + c.cfg.AuthKey)),
	})
	if err != nil {
		return fmt.Errorf("failed to create ws client: %w", err)
	}

	c.wsClient = wsClient
	c.receiveCh = make(chan ClientMessage, msgChSize)
	c.errorCh = make(chan error)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		c.msgReader()
	}()

	go func() {
		for err := range c.wsClient.ErrorCh() {
			c.sendError(err)
		}
		wg.Wait()
		close(c.errorCh)
	}()

	return nil
}

func (c *Client) Send(msg ClientMessage) error {
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
	if c.wsClient != nil {
		err := c.wsClient.Close()
		close(c.receiveCh)
		return err
	}
	return nil
}

func (c *Client) sendError(err error) {
	select {
	case c.errorCh <- err:
	default:
		log.Printf("failed to send error: channel is full")
	}
}

func (c *Client) msgReader() {
	for msg := range c.wsClient.ReceiveCh() {
		if msg.Type != ws.BinaryMessage {
			c.sendError(fmt.Errorf("unexpected msg type: %d", msg.Type))
			continue
		}

		var cm ClientMessage
		if err := cm.Unpack(msg.Data); err != nil {
			c.sendError(fmt.Errorf("failed to unpack message: %w", err))
			continue
		}

		select {
		case c.receiveCh <- cm:
		default:
			c.sendError(fmt.Errorf("failed to send client message: channel is full"))
		}
	}
}
