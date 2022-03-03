// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package service

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetVersion(t *testing.T) {
	th := SetupTestHelper(t)
	defer th.Teardown()

	t.Run("invalid method", func(t *testing.T) {
		resp, err := http.Post(th.apiURL+"/version", "", nil)
		require.NoError(t, err)
		require.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("valid response", func(t *testing.T) {
		version = "0.1.0-test"
		buildHash = "test"
		resp, err := http.Get(th.apiURL + "/version")
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		defer resp.Body.Close()
		var data map[string]string
		err = json.NewDecoder(resp.Body).Decode(&data)
		require.NoError(t, err)
		require.Equal(t, version, data["version"])
		require.Equal(t, buildHash, data["build"])
	})
}
