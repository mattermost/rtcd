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
	th := SetupTestHelper(t, nil)
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

func TestUnregisterClient(t *testing.T) {
	t.Run("invalid method", func(t *testing.T) {
		th := SetupTestHelper(t, nil)
		defer th.Teardown()
		resp, err := http.Get(th.apiURL + "/unregister")
		require.NoError(t, err)
		require.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("invalid: admin and self-registering not enabled", func(t *testing.T) {
		cfg := MakeDefaultCfg(t)
		cfg.API.Security.EnableAdmin = false
		th := SetupTestHelper(t, cfg)
		defer th.Teardown()

		req, err := http.NewRequest("POST", th.apiURL+"/unregister", bytes.NewBuffer(nil))
		require.NoError(t, err)
		req.SetBasicAuth("", th.srvc.cfg.API.Security.AdminSecretKey)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		require.Equal(t, http.StatusForbidden, resp.StatusCode)
		defer resp.Body.Close()
	})

	t.Run("invalid: admin enabled, self-registering is enabled (but client is not registered)", func(t *testing.T) {
		cfg := MakeDefaultCfg(t)
		cfg.API.Security.AllowSelfRegistration = true
		th := SetupTestHelper(t, cfg)
		defer th.Teardown()

		req, err := http.NewRequest("POST", th.apiURL+"/unregister", bytes.NewBuffer(nil))
		require.NoError(t, err)
		req.SetBasicAuth("clientA", "Ey4-H_BJA00_TVByPi8DozE12ekN3S7L")
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		defer resp.Body.Close()
	})

	t.Run("invalid: admin disabled, self-registering is enabled (but client is not registered)", func(t *testing.T) {
		cfg := MakeDefaultCfg(t)
		cfg.API.Security.EnableAdmin = false
		cfg.API.Security.AllowSelfRegistration = true
		th := SetupTestHelper(t, cfg)
		defer th.Teardown()

		req, err := http.NewRequest("POST", th.apiURL+"/unregister", bytes.NewBuffer(nil))
		require.NoError(t, err)
		req.SetBasicAuth("clientA", "Ey4-H_BJA00_TVByPi8DozE12ekN3S7L")
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		defer resp.Body.Close()
	})

	t.Run("invalid: admin disabled, self-registering is enabled, wrong authkey", func(t *testing.T) {
		cfg := MakeDefaultCfg(t)
		cfg.API.Security.EnableAdmin = false
		cfg.API.Security.AllowSelfRegistration = true
		th := SetupTestHelper(t, cfg)
		defer th.Teardown()

		registerClient(t, th, "clientA", "Ey4-H_BJA00_TVByPi8DozE12ekN3S7H")

		req, err := http.NewRequest("POST", th.apiURL+"/unregister", bytes.NewBuffer([]byte(`{"clientID":"clientA"}`)))
		require.NoError(t, err)
		req.SetBasicAuth("clientA", "Ey4-H_BJA00_TVByPi8DozE12ekN3AAA")
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		defer resp.Body.Close()
	})

	t.Run("invalid: self-registering enabled, different clientID", func(t *testing.T) {
		cfg := MakeDefaultCfg(t)
		cfg.API.Security.EnableAdmin = false
		cfg.API.Security.AllowSelfRegistration = true
		th := SetupTestHelper(t, cfg)
		defer th.Teardown()

		registerClient(t, th, "clientA", "Ey4-H_BJA00_TVByPi8DozE12ekN3S7H")
		registerClient(t, th, "clientB", "Ey4-H_BJA00_TVByPi8DozJIF8IewuPf")

		req, err := http.NewRequest("POST", th.apiURL+"/unregister", bytes.NewBuffer([]byte(`{"clientID":"clientB"}`)))
		require.NoError(t, err)
		req.SetBasicAuth("clientA", "Ey4-H_BJA00_TVByPi8DozE12ekN3S7H")
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		require.Equal(t, http.StatusForbidden, resp.StatusCode)
		defer resp.Body.Close()
	})

	t.Run("invalid: admin enabled, but non-existent client", func(t *testing.T) {
		th := SetupTestHelper(t, nil)
		defer th.Teardown()

		registerClient(t, th, "clientA", "Ey4-H_BJA00_TVByPi8DozE12ekN3S7H")

		req, err := http.NewRequest("POST", th.apiURL+"/unregister", bytes.NewBuffer([]byte(`{"clientID":"clientB"}`)))
		require.NoError(t, err)
		req.SetBasicAuth("", th.srvc.cfg.API.Security.AdminSecretKey)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		defer resp.Body.Close()
	})

	t.Run("valid: admin disabled, self-registering is enabled", func(t *testing.T) {
		cfg := MakeDefaultCfg(t)
		cfg.API.Security.EnableAdmin = false
		cfg.API.Security.AllowSelfRegistration = true
		th := SetupTestHelper(t, cfg)
		defer th.Teardown()

		registerClient(t, th, "clientA", "Ey4-H_BJA00_TVByPi8DozE12ekN3S7H")

		req, err := http.NewRequest("POST", th.apiURL+"/unregister", bytes.NewBuffer([]byte(`{"clientID":"clientA"}`)))
		require.NoError(t, err)
		req.SetBasicAuth("clientA", "Ey4-H_BJA00_TVByPi8DozE12ekN3S7H")
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		defer resp.Body.Close()
	})

	t.Run("valid: admin enabled", func(t *testing.T) {
		th := SetupTestHelper(t, nil)
		defer th.Teardown()

		registerClient(t, th, "clientA", "Ey4-H_BJA00_TVByPi8DozE12ekN3S7H")

		req, err := http.NewRequest("POST", th.apiURL+"/unregister", bytes.NewBuffer([]byte(`{"clientID":"clientA"}`)))
		require.NoError(t, err)
		req.SetBasicAuth("", th.srvc.cfg.API.Security.AdminSecretKey)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		defer resp.Body.Close()
	})
}

func TestLoginClient(t *testing.T) {
	th := SetupTestHelper(t, nil)
	defer th.Teardown()

	t.Run("invalid method", func(t *testing.T) {
		resp, err := http.Get(th.apiURL + "/login")
		require.NoError(t, err)
		require.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("bad request", func(t *testing.T) {
		req, err := http.NewRequest("POST", th.apiURL+"/login", bytes.NewBuffer(nil))
		require.NoError(t, err)
		req.SetBasicAuth("", th.srvc.cfg.API.Security.AdminSecretKey)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		defer resp.Body.Close()
	})

	t.Run("valid response", func(t *testing.T) {
		clientID := "clientA"
		authKey := "Ey4-H_BJA00_TVByPi8DozE12ekN3S7L"
		err := th.srvc.auth.Register(clientID, authKey)
		buf := bytes.NewBuffer([]byte(fmt.Sprintf(`{"clientID": "%s", "authKey": "%s"}`, clientID, authKey)))
		require.NoError(t, err)
		req, err := http.NewRequest("POST", th.apiURL+"/login", buf)
		require.NoError(t, err)
		req.SetBasicAuth("", th.srvc.cfg.API.Security.AdminSecretKey)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		defer resp.Body.Close()
		var response map[string]string
		err = json.NewDecoder(resp.Body).Decode(&response)
		require.NoError(t, err)

		token := response["bearerToken"]
		require.NotEmpty(t, token)
		buf = bytes.NewBuffer([]byte(fmt.Sprintf(`{"clientID": "%s"}`, clientID)))
		req, err = http.NewRequest("POST", th.apiURL+"/unregister", buf)
		require.NoError(t, err)
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
		resp, err = http.DefaultClient.Do(req)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})
}

func TestWSAuthHandler(t *testing.T) {
	th := SetupTestHelper(t, nil)
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

func registerClient(t *testing.T, th *TestHelper, clientID string, authKey string) {
	bufStr := fmt.Sprintf(`{"clientID": "%s", "authKey": "%s"}`, clientID, authKey)
	buf := bytes.NewBuffer([]byte(bufStr))
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
}