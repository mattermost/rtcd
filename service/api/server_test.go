// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package api

import (
	"crypto/tls"
	"net"
	"net/http"
	"testing"

	"github.com/mattermost/mattermost-server/server/public/shared/mlog"
	"github.com/stretchr/testify/require"
)

func TestStartServer(t *testing.T) {
	log, err := mlog.NewLogger()
	require.NoError(t, err)
	defer func() {
		err := log.Shutdown()
		require.NoError(t, err)
	}()

	t.Run("port unavailable", func(t *testing.T) {
		listener, err := net.Listen("tcp", ":0")
		require.NoError(t, err)
		defer func() {
			err := listener.Close()
			require.NoError(t, err)
		}()

		cfg := Config{
			ListenAddress: listener.Addr().String(),
		}
		s, err := NewServer(cfg, log)
		require.NoError(t, err)
		require.NotNil(t, s)

		err = s.Start()
		require.Error(t, err)
		err = s.Stop()
		require.NoError(t, err)
	})

	t.Run("plaintext", func(t *testing.T) {
		cfg := Config{
			ListenAddress: ":0",
		}
		s, err := NewServer(cfg, log)
		require.NoError(t, err)
		require.NotNil(t, s)

		err = s.Start()
		require.NoError(t, err)

		client := &http.Client{}

		_, port, err := net.SplitHostPort(s.listener.Addr().String())
		require.NoError(t, err)
		_, err = client.Get("http://localhost:" + port)
		require.NoError(t, err)

		err = s.Stop()
		require.NoError(t, err)

		_, err = client.Get("http://localhost:" + port)
		require.Error(t, err)
	})

	t.Run("tls", func(t *testing.T) {
		cfg := Config{
			ListenAddress: ":0",
			TLS: TLSConfig{
				Enable:   true,
				CertFile: "../../testfiles/tls_test_cert.pem",
				CertKey:  "../../testfiles/tls_test_key.pem",
			},
		}
		s, err := NewServer(cfg, log)
		require.NoError(t, err)
		require.NotNil(t, s)

		err = s.Start()
		require.NoError(t, err)

		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		client := &http.Client{Transport: tr}

		_, port, err := net.SplitHostPort(s.listener.Addr().String())
		require.NoError(t, err)
		_, err = client.Get("https://localhost:" + port)
		require.NoError(t, err)

		err = s.Stop()
		require.NoError(t, err)

		_, err = client.Get("https://localhost:" + port)
		require.Error(t, err)
	})
}
