// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package service

import (
	"net"
	"os"
	"testing"

	"github.com/mattermost/rtcd/logger"
	"github.com/mattermost/rtcd/service/api"
	"github.com/mattermost/rtcd/service/auth"
	"github.com/mattermost/rtcd/service/rtc"

	"github.com/stretchr/testify/require"
)

type TestHelper struct {
	srvc        *Service
	adminClient *Client
	cfg         Config
	tb          testing.TB
	apiURL      string
	dbDir       string
}

// SetupTestHelper takes a *cfg, pass nil to use the default config.
func SetupTestHelper(tb testing.TB, cfg *Config) *TestHelper {
	tb.Helper()
	var err error

	if cfg == nil {
		cfg = MakeDefaultCfg(tb)
	}

	th := &TestHelper{
		cfg:   *cfg,
		tb:    tb,
		dbDir: cfg.Store.DataSource,
	}

	th.srvc, err = New(th.cfg)
	require.NoError(th.tb, err)
	require.NotNil(th.tb, th.srvc)

	err = th.srvc.Start()
	require.NoError(th.tb, err)

	_, port, err := net.SplitHostPort(th.srvc.apiServer.Addr())
	require.NoError(th.tb, err)
	th.apiURL = "http://localhost:" + port

	th.adminClient, err = NewClient(ClientConfig{
		URL:     th.apiURL,
		AuthKey: th.srvc.cfg.API.Security.AdminSecretKey,
	})
	require.NoError(th.tb, err)
	require.NotNil(th.tb, th.adminClient)

	return th
}

func MakeDefaultCfg(tb testing.TB) *Config {
	tb.Helper()

	dbDir, err := os.MkdirTemp("", "db")
	require.NoError(tb, err)

	return &Config{
		API: APIConfig{
			HTTP: api.Config{
				ListenAddress: ":0",
			},
			Security: SecurityConfig{
				EnableAdmin:    true,
				AdminSecretKey: "admin_secret_key",
				SessionCache: auth.SessionCacheConfig{
					ExpirationMinutes: 1440,
				},
			},
		},
		RTC: rtc.ServerConfig{
			ICEPortUDP:      30444,
			ICEPortTCP:      30444,
			UDPSocketsCount: rtc.GetDefaultUDPListeningSocketsCount(),
		},
		Store: StoreConfig{
			DataSource: dbDir,
		},
		Logger: logger.Config{
			EnableConsole: true,
			ConsoleLevel:  "ERROR",
		},
	}
}

func (th *TestHelper) Teardown() {
	err := th.srvc.Stop()
	require.NoError(th.tb, err)

	err = os.RemoveAll(th.dbDir)
	require.NoError(th.tb, err)
}
