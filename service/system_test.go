// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package service

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetSystem(t *testing.T) {
	th := SetupTestHelper(t, nil)
	defer th.Teardown()

	t.Run("invalid method", func(t *testing.T) {
		resp, err := http.Post(th.apiURL+"/system", "", nil)
		require.NoError(t, err)
		require.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("valid response", func(t *testing.T) {
		resp, err := http.Get(th.apiURL + "/system")
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		defer resp.Body.Close()
		var info SystemInfo
		err = json.NewDecoder(resp.Body).Decode(&info)
		require.NoError(t, err)
		require.NotZero(t, info.CPULoad)
	})
}
