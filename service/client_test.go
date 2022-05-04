// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package service

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mattermost/rtcd/service/ws"

	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	t.Run("empty config", func(t *testing.T) {
		c, err := NewClient(ClientConfig{})
		require.Error(t, err)
		require.Equal(t, "failed to parse config: invalid URL value: should not be empty", err.Error())
		require.Nil(t, c)
	})

	t.Run("invalid url", func(t *testing.T) {
		c, err := NewClient(ClientConfig{URL: "not_a_url"})
		require.Error(t, err)
		require.Equal(t, "failed to parse config: invalid url host: should not be empty", err.Error())
		require.Nil(t, c)
	})

	t.Run("invalid scheme", func(t *testing.T) {
		c, err := NewClient(ClientConfig{URL: "ftp://invalid"})
		require.Error(t, err)
		require.Equal(t, `failed to parse config: invalid url scheme: "ftp" is not valid`, err.Error())
		require.Nil(t, c)
	})

	t.Run("success http scheme", func(t *testing.T) {
		apiURL := "http://localhost"
		c, err := NewClient(ClientConfig{URL: apiURL})
		require.NoError(t, err)
		require.NotNil(t, c)
		require.NotEmpty(t, c)
		require.Equal(t, apiURL, c.cfg.httpURL)
		require.Equal(t, "ws://localhost/ws", c.cfg.wsURL)
	})

	t.Run("success https scheme", func(t *testing.T) {
		apiURL := "https://localhost"
		c, err := NewClient(ClientConfig{URL: apiURL})
		require.NoError(t, err)
		require.NotNil(t, c)
		require.NotEmpty(t, c)
		require.Equal(t, apiURL, c.cfg.httpURL)
		require.Equal(t, "wss://localhost/ws", c.cfg.wsURL)
	})
}

func TestClientRegister(t *testing.T) {
	th := SetupTestHelper(t)
	defer th.Teardown()

	c, err := NewClient(ClientConfig{
		URL:     th.apiURL,
		AuthKey: th.srvc.cfg.API.Security.AdminSecretKey,
	})
	require.NoError(t, err)
	require.NotNil(t, c)
	defer c.Close()

	t.Run("empty clientID", func(t *testing.T) {
		authToken, err := c.Register("")
		require.Error(t, err)
		require.Empty(t, authToken)
		require.Equal(t, "request failed: registration failed: error: empty key", err.Error())
	})

	t.Run("valid clientID", func(t *testing.T) {
		authToken, err := c.Register("clientA")
		require.NoError(t, err)
		require.NotEmpty(t, authToken)
	})

	t.Run("existing clientID", func(t *testing.T) {
		authToken, err := c.Register("clientA")
		require.Error(t, err)
		require.Empty(t, authToken)
		require.Equal(t, "request failed: registration failed: already registered", err.Error())
	})

	t.Run("unauthorized", func(t *testing.T) {
		c, err := NewClient(ClientConfig{
			URL:     th.apiURL,
			AuthKey: th.srvc.cfg.API.Security.AdminSecretKey + "_",
		})
		require.NoError(t, err)
		require.NotNil(t, c)
		defer c.Close()

		authToken, err := c.Register("")
		require.Error(t, err)
		require.Empty(t, authToken)
		require.Equal(t, "request failed: authentication failed: unauthorized", err.Error())
	})

	t.Run("self registering", func(t *testing.T) {
		c, err := NewClient(ClientConfig{
			URL: th.apiURL,
		})
		require.NoError(t, err)
		require.NotNil(t, c)
		defer c.Close()

		authToken, err := c.Register("clientB")
		require.Error(t, err)
		require.Empty(t, authToken)
		require.Equal(t, "request failed: authentication failed: unauthorized", err.Error())

		th.srvc.cfg.API.Security.AllowSelfRegistration = true
		authToken, err = c.Register("clientB")
		require.NoError(t, err)
		require.NotEmpty(t, authToken)
	})
}

func TestClientUnregister(t *testing.T) {
	th := SetupTestHelper(t)
	defer th.Teardown()

	c, err := NewClient(ClientConfig{
		URL:     th.apiURL,
		AuthKey: th.srvc.cfg.API.Security.AdminSecretKey,
	})
	require.NoError(t, err)
	require.NotNil(t, c)
	defer c.Close()

	t.Run("empty client ID", func(t *testing.T) {
		authKey, err := c.Register("clientA")
		require.NoError(t, err)
		require.NotEmpty(t, authKey)

		err = c.Unregister("")
		require.Error(t, err)
		require.Equal(t, "request failed: client id should not be empty", err.Error())
	})

	t.Run("not found", func(t *testing.T) {
		err := c.Unregister("clientB")
		require.Error(t, err)
		require.Equal(t, "request failed: unregister failed: error: not found", err.Error())
	})

	t.Run("success", func(t *testing.T) {
		err := c.Unregister("clientA")
		require.NoError(t, err)
	})

	t.Run("unauthorized", func(t *testing.T) {
		c, err := NewClient(ClientConfig{
			URL:     th.apiURL,
			AuthKey: th.srvc.cfg.API.Security.AdminSecretKey + "_",
		})
		require.NoError(t, err)
		require.NotNil(t, c)
		defer c.Close()

		err = c.Unregister("clientA")
		require.Error(t, err)
		require.Equal(t, "request failed: authentication failed: unauthorized", err.Error())
	})
}

func TestClientConnect(t *testing.T) {
	th := SetupTestHelper(t)
	defer th.Teardown()

	c, err := NewClient(ClientConfig{URL: th.apiURL})
	require.NoError(t, err)
	require.NotNil(t, c)

	t.Run("auth failure", func(t *testing.T) {
		err := c.Connect()
		require.Error(t, err)
	})

	t.Run("success", func(t *testing.T) {
		clientID := "clientA"
		authKey, err := th.adminClient.Register(clientID)
		require.NoError(t, err)
		require.NotEmpty(t, authKey)

		c.cfg.ClientID = clientID
		c.cfg.AuthKey = authKey

		err = c.Connect()
		require.NoError(t, err)

		err = c.Connect()
		require.Error(t, err)
		require.Equal(t, "ws client is already initialized", err.Error())

		err = c.Close()
		require.NoError(t, err)

		err = c.Connect()
		require.Error(t, err)
		require.Equal(t, "ws client is closed", err.Error())
	})
}

func TestClientSend(t *testing.T) {
	th := SetupTestHelper(t)
	defer th.Teardown()

	clientID := "clientA"
	authKey, err := th.adminClient.Register(clientID)
	require.NoError(t, err)
	require.NotEmpty(t, authKey)

	t.Run("not ininitialized", func(t *testing.T) {
		c, err := NewClient(ClientConfig{
			URL:      th.apiURL,
			ClientID: clientID,
			AuthKey:  authKey,
		})
		require.NoError(t, err)
		require.NotNil(t, c)
		defer c.Close()

		err = c.Send(ClientMessage{})
		require.Error(t, err)
		require.Equal(t, "ws client is not initialized", err.Error())
	})

	t.Run("success", func(t *testing.T) {
		c, err := NewClient(ClientConfig{
			URL:      th.apiURL,
			ClientID: clientID,
			AuthKey:  authKey,
		})
		require.NoError(t, err)
		require.NotNil(t, c)

		err = c.Connect()
		require.NoError(t, err)

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := <-c.ErrorCh()
			require.NoError(t, err)
		}()

		for i := 0; i < 10; i++ {
			cm := ClientMessage{
				Type: "msgType",
				Data: []byte(`data`),
			}
			err := c.Send(cm)
			require.NoError(t, err)
		}

		err = c.Close()
		require.NoError(t, err)
		wg.Wait()

		err = c.Send(ClientMessage{})
		require.Error(t, err)
		require.Equal(t, "ws client is closed", err.Error())
	})
}

func TestClientReceive(t *testing.T) {
	th := SetupTestHelper(t)
	defer th.Teardown()

	clientID := "clientA"
	authKey, err := th.adminClient.Register(clientID)
	require.NoError(t, err)
	require.NotEmpty(t, authKey)

	c, err := NewClient(ClientConfig{
		URL:      th.apiURL,
		ClientID: clientID,
		AuthKey:  authKey,
	})
	require.NoError(t, err)
	require.NotNil(t, c)

	err = c.Connect()
	require.NoError(t, err)

	msg, ok := <-c.ReceiveCh()
	require.True(t, ok)
	require.Equal(t, ClientMessageHello, msg.Type)
	msgData, ok := msg.Data.(map[string]string)
	require.True(t, ok)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		err := <-c.ErrorCh()
		require.NoError(t, err)
	}()

	msgs := []ClientMessage{
		{Type: "test"},
		{Type: "test2"},
		{Type: "test3"},
	}

	go func() {
		defer wg.Done()
		defer c.Close()
		i := 0
		for msg := range c.ReceiveCh() {
			require.Equal(t, msgs[i], msg)
			i++
			if i == len(msgs) {
				break
			}
		}
	}()

	for _, msg := range msgs {
		data, err := msg.Pack()
		require.NoError(t, err)
		err = th.srvc.wsServer.Send(ws.Message{Type: ws.BinaryMessage, Data: data, ConnID: msgData["connID"], ClientID: clientID})
		require.NoError(t, err)
	}

	wg.Wait()
}

func TestClientReconnect(t *testing.T) {
	th := SetupTestHelper(t)
	defer th.Teardown()

	clientID := "clientA"
	authKey, err := th.adminClient.Register(clientID)
	require.NoError(t, err)
	require.NotEmpty(t, authKey)

	t.Run("success", func(t *testing.T) {
		var cbCalled bool
		c, err := NewClient(ClientConfig{
			URL:      th.apiURL,
			ClientID: clientID,
			AuthKey:  authKey,
		}, WithClientReconnectCb(func(_ *Client, n int) error {
			require.Equal(t, 1, n)
			cbCalled = true
			return nil
		}))
		require.NoError(t, err)
		require.NotNil(t, c)

		err = c.Connect()
		require.NoError(t, err)

		msg, ok := <-c.ReceiveCh()
		require.True(t, ok)
		require.Equal(t, ClientMessageHello, msg.Type)

		msgData, ok := msg.Data.(map[string]string)
		require.True(t, ok)

		serverMsg := ClientMessage{Type: "test"}

		data, err := serverMsg.Pack()
		require.NoError(t, err)
		err = th.srvc.wsServer.Send(ws.Message{Type: ws.BinaryMessage, ClientID: clientID, ConnID: msgData["connID"], Data: data})
		require.NoError(t, err)

		msg, ok = <-c.ReceiveCh()
		require.True(t, ok)
		require.Equal(t, serverMsg, msg)

		err = th.srvc.wsServer.Send(ws.Message{Type: ws.CloseMessage, ClientID: clientID, ConnID: msgData["connID"]})
		require.NoError(t, err)

		msg, ok = <-c.ReceiveCh()
		require.True(t, ok)
		require.Equal(t, ClientMessageHello, msg.Type)

		require.True(t, cbCalled)

		err = c.Close()
		require.NoError(t, err)
	})

	t.Run("callback error", func(t *testing.T) {
		var cbCalled bool
		c, err := NewClient(ClientConfig{
			URL:      th.apiURL,
			ClientID: clientID,
			AuthKey:  authKey,
		}, WithClientReconnectCb(func(_ *Client, n int) error {
			require.Equal(t, 1, n)
			cbCalled = true
			return errors.New("cb error")
		}))
		require.NoError(t, err)
		require.NotNil(t, c)

		err = c.Connect()
		require.NoError(t, err)

		msg, ok := <-c.ReceiveCh()
		require.True(t, ok)
		require.Equal(t, ClientMessageHello, msg.Type)

		msgData, ok := msg.Data.(map[string]string)
		require.True(t, ok)

		serverMsg := ClientMessage{Type: "test"}

		data, err := serverMsg.Pack()
		require.NoError(t, err)
		err = th.srvc.wsServer.Send(ws.Message{Type: ws.BinaryMessage, ClientID: clientID, ConnID: msgData["connID"], Data: data})
		require.NoError(t, err)

		msg, ok = <-c.ReceiveCh()
		require.True(t, ok)
		require.Equal(t, serverMsg, msg)

		err = th.srvc.wsServer.Send(ws.Message{Type: ws.CloseMessage, ClientID: clientID, ConnID: msgData["connID"]})
		require.NoError(t, err)

		msg, ok = <-c.ReceiveCh()
		require.False(t, ok)
		require.True(t, cbCalled)

		err = c.Close()
		require.Error(t, err)
		require.Equal(t, "ws client is closed", err.Error())
	})
}

func TestClientCloseReconnectRace(t *testing.T) {
	th := SetupTestHelper(t)
	defer th.Teardown()

	clientID := "clientA"
	authKey, err := th.adminClient.Register(clientID)
	require.NoError(t, err)
	require.NotEmpty(t, authKey)

	c, err := NewClient(ClientConfig{
		URL:      th.apiURL,
		ClientID: clientID,
		AuthKey:  authKey,
	})
	require.NoError(t, err)
	require.NotNil(t, c)

	err = c.Connect()
	require.NoError(t, err)

	msg, ok := <-c.ReceiveCh()
	require.True(t, ok)
	require.Equal(t, ClientMessageHello, msg.Type)
	msgData, ok := msg.Data.(map[string]string)
	require.True(t, ok)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		err := th.srvc.wsServer.Send(ws.Message{Type: ws.CloseMessage, ClientID: clientID, ConnID: msgData["connID"]})
		require.NoError(t, err)
	}()

	go func() {
		defer wg.Done()
		time.Sleep(time.Millisecond)
		_ = c.Close()
	}()

	wg.Wait()
}

func TestClientConcurrency(t *testing.T) {
	th := SetupTestHelper(t)
	defer th.Teardown()

	clientID := "clientA"
	authKey, err := th.adminClient.Register(clientID)
	require.NoError(t, err)
	require.NotEmpty(t, authKey)

	c, err := NewClient(ClientConfig{
		URL:      th.apiURL,
		ClientID: clientID,
		AuthKey:  authKey,
	})
	require.NoError(t, err)
	require.NotNil(t, c)

	n := 10
	var nErrors int32
	var wg sync.WaitGroup

	t.Run("Connect", func(t *testing.T) {
		wg.Add(n)
		for i := 0; i < n; i++ {
			go func() {
				defer wg.Done()
				err := c.Connect()
				if err != nil {
					atomic.AddInt32(&nErrors, 1)
				}
			}()
		}
		wg.Wait()

		require.Equal(t, n-1, int(nErrors))
	})

	t.Run("Close", func(t *testing.T) {
		nErrors = 0
		wg.Add(n)
		for i := 0; i < n; i++ {
			go func() {
				defer wg.Done()
				err := c.Close()
				if err != nil {
					atomic.AddInt32(&nErrors, 1)
				}
			}()
		}
		wg.Wait()

		require.Equal(t, n-1, int(nErrors))
	})
}
