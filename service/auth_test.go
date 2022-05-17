// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package service

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"testing"

	"github.com/mattermost/rtcd/service/ws"

	"github.com/stretchr/testify/require"
)

func TestRegisterClient(t *testing.T) {
	th := SetupTestHelper(t)
	defer th.Teardown()

	t.Run("invalid method", func(t *testing.T) {
		resp, err := http.Get(th.apiURL + "/register")
		require.NoError(t, err)
		require.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("bad request", func(t *testing.T) {
		req, err := http.NewRequest("POST", th.apiURL+"/register", bytes.NewBuffer(nil))
		require.NoError(t, err)
		req.SetBasicAuth("", th.srvc.cfg.API.Security.AdminSecretKey)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		defer resp.Body.Close()
	})

	t.Run("valid response", func(t *testing.T) {
		buf := bytes.NewBuffer([]byte(`{"clientID": "clientA", "authKey": "Ey4-H_BJA00_TVByPi8DozE12ekN3S7L"}`))
		req, err := http.NewRequest("POST", th.apiURL+"/register", buf)
		require.NoError(t, err)
		req.SetBasicAuth("", th.srvc.cfg.API.Security.AdminSecretKey)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		defer resp.Body.Close()
		var response map[string]string
		err = json.NewDecoder(resp.Body).Decode(&response)
		require.NoError(t, err)
		require.NotEmpty(t, response["clientID"])
	})
}

func TestWSAuthHandler(t *testing.T) {
	th := SetupTestHelper(t)
	defer th.Teardown()

	_, port, err := net.SplitHostPort(th.srvc.apiServer.Addr())
	require.NoError(t, err)
	wsURL := url.URL{Scheme: "ws", Host: "localhost:" + port, Path: "/ws"}

	t.Run("missing auth", func(t *testing.T) {
		wsClient, err := ws.NewClient(ws.ClientConfig{
			URL: wsURL.String(),
		})
		require.Error(t, err)
		require.Nil(t, wsClient)
	})

	t.Run("bad auth", func(t *testing.T) {
		wsClient, err := ws.NewClient(ws.ClientConfig{
			URL:       wsURL.String(),
			AuthToken: "invalid",
		})
		require.Error(t, err)
		require.Nil(t, wsClient)
	})

	t.Run("valid auth", func(t *testing.T) {
		clientID := "clientA"
		authKey := "Ey4-H_BJA00_TVByPi8DozE12ekN3S7L"
		buf := bytes.NewBuffer([]byte(fmt.Sprintf(`{"clientID": "%s", "authKey": "%s"}`, clientID, authKey)))
		req, err := http.NewRequest("POST", th.apiURL+"/register", buf)
		require.NoError(t, err)
		req.SetBasicAuth("", th.srvc.cfg.API.Security.AdminSecretKey)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		defer resp.Body.Close()
		var response map[string]string
		err = json.NewDecoder(resp.Body).Decode(&response)
		require.NoError(t, err)

		token := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", clientID, authKey)))
		require.NotEmpty(t, token)
		wsClient, err := ws.NewClient(ws.ClientConfig{
			URL:       wsURL.String(),
			AuthToken: token,
		})
		require.NoError(t, err)
		require.NotNil(t, wsClient)
	})
}
