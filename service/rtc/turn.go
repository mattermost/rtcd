// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"time"
)

const MaxTURNCredentialsExpiration = 7 * 24 * 60 // 1 week in minutes

type TURNConfig struct {
	// The secret key used to generate TURN short-lived authentication
	// credentials.
	StaticAuthSecret string `toml:"static_auth_secret"`
	// The number of minutes that the generated TURN credentials will be valid for.
	CredentialsExpirationMinutes int `toml:"credentials_expiration_minutes"`
}

func (c TURNConfig) IsValid() error {
	if c.StaticAuthSecret != "" {
		if c.CredentialsExpirationMinutes <= 0 {
			return fmt.Errorf("invalid CredentialsExpirationMinutes value: should be a positive number")
		}
		if c.CredentialsExpirationMinutes >= MaxTURNCredentialsExpiration {
			return fmt.Errorf("invalid CredentialsExpirationMinutes value: should be less than 1 week")
		}
	}

	return nil
}

func genTURNCredentials(username, secret string, expirationTS int64) (string, string, error) {
	if username == "" {
		return "", "", fmt.Errorf("username should not be empty")
	}

	if secret == "" {
		return "", "", fmt.Errorf("secret should not be empty")
	}

	if expirationTS <= 0 {
		return "", "", fmt.Errorf("expirationTS should be a positive number")
	}

	if expirationTS > time.Now().Add(MaxTURNCredentialsExpiration*time.Minute).Unix() {
		return "", "", fmt.Errorf("expirationTS cannot be more than a week into the future")
	}

	h := hmac.New(sha1.New, []byte(secret))
	username = fmt.Sprintf("%d:%s", expirationTS, username)
	_, err := h.Write([]byte(username))
	if err != nil {
		return "", "", fmt.Errorf("failed to write hmac: %w", err)
	}
	password := base64.StdEncoding.EncodeToString(h.Sum(nil))
	return username, password, nil
}

func GenTURNConfigs(turnServers ICEServers, username, secret string, expiryMinutes int) (ICEServers, error) {
	var configs ICEServers
	ts := time.Now().Add(time.Duration(expiryMinutes) * time.Minute).Unix()

	for _, cfg := range turnServers {
		if cfg.Username != "" || cfg.Credential != "" {
			continue
		}
		username, password, err := genTURNCredentials(username, secret, ts)
		if err != nil {
			return nil, err
		}
		configs = append(configs, ICEServerConfig{
			URLs:       cfg.URLs,
			Username:   username,
			Credential: password,
		})
	}

	return configs, nil
}
