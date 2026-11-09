package client

import (
	"bytes"
	"context"
	"net/http"

	"github.com/mattermost/mattermost/server/public/model"
)

// mattermost/server/public v0.6 unexported Client4.DoAPIRequest. This wrapper
// keeps the previous behavior of calling a fully-qualified plugin URL and
// turning non-2xx responses into an AppError.
func doAPIRequest(ctx context.Context, apiClient *model.Client4, method, rawURL, data, etag string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, bytes.NewReader([]byte(data)))
	if err != nil {
		return nil, err
	}

	req.Header.Set(model.HeaderEtagClient, etag)
	if apiClient.AuthToken != "" {
		req.Header.Set(model.HeaderAuth, apiClient.AuthType+" "+apiClient.AuthToken)
	}
	for k, v := range apiClient.HTTPHeader {
		req.Header.Set(k, v)
	}

	resp, err := apiClient.HTTPClient.Do(req)
	if err != nil || resp == nil {
		return resp, err
	}
	if resp.StatusCode == http.StatusNotModified {
		return resp, nil
	}
	if resp.StatusCode >= 300 {
		defer resp.Body.Close()
		return resp, model.AppErrorFromJSON(resp.Body)
	}

	return resp, nil
}
