// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package service

import (
	"encoding/json"
	"net/http"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetVersion(t *testing.T) {
	th := SetupTestHelper(t, nil)
	defer th.Teardown()

	t.Run("invalid method", func(t *testing.T) {
		resp, err := http.Post(th.apiURL+"/version", "", nil)
		require.NoError(t, err)
		require.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("valid response, all fields set", func(t *testing.T) {
		buildHash = "test"
		buildDate = "2022-05-12 09:05"
		buildVersion = "dev-432dad0"
		defer func() {
			buildHash = ""
			buildDate = ""
			buildVersion = ""
		}()
		goVersion := runtime.Version()
		resp, err := http.Get(th.apiURL + "/version")
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		defer resp.Body.Close()
		var info VersionInfo
		err = json.NewDecoder(resp.Body).Decode(&info)
		require.NoError(t, err)
		require.Equal(t, VersionInfo{
			BuildHash:    buildHash,
			BuildDate:    buildDate,
			BuildVersion: buildVersion,
			GoVersion:    goVersion,
		}, info)
	})

	t.Run("valid response, empty fields", func(t *testing.T) {
		goVersion := runtime.Version()
		resp, err := http.Get(th.apiURL + "/version")
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		defer resp.Body.Close()
		var info VersionInfo
		err = json.NewDecoder(resp.Body).Decode(&info)
		require.NoError(t, err)
		require.Equal(t, VersionInfo{
			GoVersion: goVersion,
		}, info)
	})
}
