// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/pion/webrtc/v3"
)

const (
	httpRequestTimeout           = 10 * time.Second
	httpResponseBodyMaxSizeBytes = 1024 * 1024 // 1MB
)

func (c *Client) Unmute(track webrtc.TrackLocal) error {
	if track == nil {
		return fmt.Errorf("invalid nil track")
	}

	c.mut.RLock()
	sender := c.voiceSender
	c.mut.RUnlock()

	if sender == nil {
		snd, err := c.pc.AddTrack(track)
		if err != nil {
			return fmt.Errorf("failed to add track: %w", err)
		}
		c.mut.Lock()
		c.voiceSender = snd
		sender = snd
		c.mut.Unlock()
	} else {
		if err := sender.ReplaceTrack(track); err != nil {
			return fmt.Errorf("failed to replace track: %w", err)
		}
	}

	go func() {
		defer log.Printf("exiting RTCP handler")
		rtcpBuf := make([]byte, receiveMTU)
		for {
			if _, _, rtcpErr := sender.Read(rtcpBuf); rtcpErr != nil {
				log.Printf("failed to read rtcp: %s", rtcpErr.Error())
				return
			}
		}
	}()

	return c.SendWS(wsEventUnmute, nil, false)
}

func (c *Client) Mute() error {
	c.mut.Lock()
	defer c.mut.Unlock()

	if c.voiceSender != nil {
		if err := c.voiceSender.ReplaceTrack(nil); err != nil {
			return fmt.Errorf("failed to replace track: %w", err)
		}
	}

	return c.sendWS(wsEventMute, nil, false)
}

func (c *Client) StartScreenShare(tracks []webrtc.TrackLocal) (*webrtc.RTPTransceiver, error) {
	if len(tracks) == 0 {
		return nil, fmt.Errorf("invalid empty tracks")
	}

	if len(tracks) > 2 {
		return nil, fmt.Errorf("too many tracks")
	}

	data, err := json.Marshal(map[string]string{
		"screenStreamID": tracks[0].StreamID(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal data: %w", err)
	}

	if err := c.SendWS(wsEventScreenOn, map[string]any{
		"data": string(data),
	}, false); err != nil {
		return nil, fmt.Errorf("failed to send screen on event: %w", err)
	}

	trx, err := c.pc.AddTransceiverFromTrack(tracks[0], webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionSendonly})
	if err != nil {
		return nil, fmt.Errorf("failed to add transceiver for track: %w", err)
	}

	// Simulcast
	if len(tracks) > 1 {
		if err := trx.Sender().AddEncoding(tracks[1]); err != nil {
			return nil, fmt.Errorf("failed to add encoding: %w", err)
		}
	}

	c.mut.Lock()
	c.screenTransceiver = trx
	c.mut.Unlock()

	sender := trx.Sender()

	go func() {
		defer log.Printf("exiting RTCP handler")
		rtcpBuf := make([]byte, receiveMTU)
		for {
			if _, _, rtcpErr := sender.Read(rtcpBuf); rtcpErr != nil {
				log.Printf("failed to read rtcp: %s", rtcpErr.Error())
				return
			}
		}
	}()

	return trx, nil
}

func (c *Client) StopScreenShare() error {
	c.mut.Lock()
	defer c.mut.Unlock()

	if c.screenTransceiver != nil {
		if err := c.pc.RemoveTrack(c.screenTransceiver.Sender()); err != nil {
			return fmt.Errorf("failed to remove track: %w", err)
		}
		c.screenTransceiver = nil
	}

	return c.sendWS(wsEventScreenOff, nil, false)
}

func (c *Client) RaiseHand() error {
	return c.SendWS(wsEventRaiseHand, nil, false)
}

func (c *Client) LowerHand() error {
	return c.SendWS(wsEventLowerHand, nil, false)
}

func (c *Client) StartRecording() error {
	ctx, cancel := context.WithTimeout(context.Background(), httpRequestTimeout)
	defer cancel()
	res, err := c.apiClient.DoAPIRequest(ctx, http.MethodPost,
		fmt.Sprintf("%s/plugins/%s/calls/%s/recording/start", c.cfg.SiteURL, pluginID, c.cfg.ChannelID), "", "")
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return fmt.Errorf("unexpected response status code %d", res.StatusCode)
	}

	return nil
}

func (c *Client) StopRecording() error {
	ctx, cancel := context.WithTimeout(context.Background(), httpRequestTimeout)
	defer cancel()
	res, err := c.apiClient.DoAPIRequest(ctx, http.MethodPost,
		fmt.Sprintf("%s/plugins/%s/calls/%s/recording/stop", c.cfg.SiteURL, pluginID, c.cfg.ChannelID), "", "")
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return fmt.Errorf("unexpected response status code %d", res.StatusCode)
	}

	return nil
}

// TODO: return a proper Config object, ideally exposed in github.com/mattermost/mattermost-plugin-calls/server/public.
func (c *Client) GetCallsConfig() (map[string]any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), httpRequestTimeout)
	defer cancel()
	res, err := c.apiClient.DoAPIRequest(ctx, http.MethodGet,
		fmt.Sprintf("%s/plugins/%s/config", c.cfg.SiteURL, pluginID), "", "")
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected response status code %d", res.StatusCode)
	}

	dec := json.NewDecoder(&io.LimitedReader{
		R: res.Body,
		N: httpResponseBodyMaxSizeBytes,
	})

	var config map[string]any
	if err := dec.Decode(&config); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return config, nil
}