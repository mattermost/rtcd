// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package service

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetStats(t *testing.T) {
	th := SetupTestHelper(t)
	defer th.Teardown()

	t.Run("invalid method", func(t *testing.T) {
		resp, err := http.Post(th.apiURL+"/stats", "", nil)
		require.NoError(t, err)
		require.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("valid response", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, th.apiURL+"/stats", nil)
		require.NoError(t, err)
		req.SetBasicAuth(th.adminClient.cfg.ClientID, th.adminClient.cfg.AuthKey)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		defer resp.Body.Close()
		var data map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&data)
		require.NoError(t, err)
		require.Equal(t, float64(0), data["calls"])
		require.Equal(t, float64(0), data["sessions"])
	})
}
