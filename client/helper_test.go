// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/mattermost/rtcd/service/random"

	"github.com/mattermost/mattermost/server/public/model"

	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
	"github.com/pion/webrtc/v4/pkg/media/ivfreader"
	"github.com/pion/webrtc/v4/pkg/media/oggreader"

	"github.com/stretchr/testify/require"
)

type TestHelper struct {
	tb             testing.TB
	apiURL         string
	adminAPIClient *model.Client4
	userAPIClient  *model.Client4
	adminClient    *Client
	userClient     *Client
	users          []*model.User
	team           *model.Team
	channels       map[string]*model.Channel
}

const (
	adminName   = "sysadmin"
	adminPass   = "Sys@dmin-sample1"
	userName    = "calls-user0"
	userPass    = "U$er-sample1"
	teamName    = "calls"
	nChannels   = 2
	waitTimeout = 10 * time.Second
)

func (th *TestHelper) newVideoTrack(mimeType string) *webrtc.TrackLocalStaticRTP {
	th.tb.Helper()

	track, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{
		MimeType:    mimeType,
		ClockRate:   90000,
		SDPFmtpLine: "",
		RTCPFeedback: []webrtc.RTCPFeedback{
			{Type: "goog-remb", Parameter: ""},
			{Type: "ccm", Parameter: "fir"},
			{Type: "nack", Parameter: ""},
			{Type: "nack", Parameter: "pli"},
		},
	}, "video", "video_"+random.NewID())
	require.NoError(th.tb, err)

	return track
}

func (th *TestHelper) newScreenTrack(mimeType string) *webrtc.TrackLocalStaticRTP {
	th.tb.Helper()

	track, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{
		MimeType:    mimeType,
		ClockRate:   90000,
		SDPFmtpLine: "",
		RTCPFeedback: []webrtc.RTCPFeedback{
			{Type: "goog-remb", Parameter: ""},
			{Type: "ccm", Parameter: "fir"},
			{Type: "nack", Parameter: ""},
			{Type: "nack", Parameter: "pli"},
		},
	}, "screen", "screen_"+random.NewID())
	require.NoError(th.tb, err)

	return track
}

func (th *TestHelper) videoTrackWriter(track *webrtc.TrackLocalStaticRTP, closeCh <-chan struct{}) {
	var payloader rtp.Payloader
	payloader = &codecs.VP8Payloader{
		EnablePictureID: true,
	}
	filename := "../testfiles/video.ivf"
	if track.Codec().MimeType == webrtc.MimeTypeAV1 {
		payloader = &codecs.AV1Payloader{}
		filename = "../testfiles/video_av1.ivf"
	}

	packetizer := rtp.NewPacketizer(
		1200,
		0,
		0,
		payloader,
		rtp.NewRandomSequencer(),
		90000,
	)

	// Open a IVF file and start reading using our IVFReader
	file, ivfErr := os.Open(filename)
	if ivfErr != nil {
		log.Fatal(ivfErr.Error())
	}
	defer file.Close()

	ivf, header, ivfErr := ivfreader.NewWith(file)
	if ivfErr != nil {
		log.Fatal(ivfErr.Error())
	}

	// Send our video file frame at a time. Pace our sending so we send it at the same speed it should be played back as.
	// This isn't required since the video is timestamped, but we will have much higher loss if we send all at once.
	//
	// It is important to use a time.Ticker instead of time.Sleep because
	// * avoids accumulating skew, just calling time.Sleep didn't compensate for the time spent parsing the data
	// * works around latency issues with Sleep (see https://github.com/golang/go/issues/44343)
	frameDuration := time.Millisecond * time.Duration((float32(header.TimebaseNumerator)/float32(header.TimebaseDenominator))*1000)

	ticker := time.NewTicker(frameDuration)
	for {
		select {
		case <-closeCh:
			log.Printf("existing ivf reader")
			return
		case <-ticker.C:
		}

		var frame []byte
		var ivfErr error
		frame, _, ivfErr = ivf.ParseNextFrame()
		if ivfErr == io.EOF || (ivfErr != nil && ivfErr.Error() == "incomplete frame data") {
			ivf.ResetReader(func(_ int64) io.Reader {
				_, _ = file.Seek(0, 0)
				ivf, header, ivfErr = ivfreader.NewWith(file)
				if ivfErr != nil {
					log.Fatal(ivfErr.Error())
				}
				return file
			})
			frame, _, ivfErr = ivf.ParseNextFrame()
		}
		if ivfErr != nil {
			log.Fatal(ivfErr.Error())
		}

		packets := packetizer.Packetize(frame, 90000/header.TimebaseDenominator)
		for _, p := range packets {
			if err := track.WriteRTP(p); err != nil {
				log.Printf("failed to write video sample: %s", err.Error())
				return
			}
		}
	}
}

func (th *TestHelper) transmitScreenTrack(c *Client) {
	th.tb.Helper()

	track := th.newScreenTrack(webrtc.MimeTypeVP8)

	data, err := json.Marshal(map[string]string{
		"screenStreamID": track.StreamID(),
	})
	require.NoError(th.tb, err)

	err = c.sendWS(wsEventScreenOn, map[string]any{
		"data": string(data),
	}, false)
	require.NoError(th.tb, err)

	sender, err := c.pc.AddTrack(track)
	require.NoError(th.tb, err)

	closeCh := make(chan struct{})
	go func() {
		rtcpBuf := make([]byte, receiveMTU)
		for {
			if _, _, rtcpErr := sender.Read(rtcpBuf); rtcpErr != nil {
				log.Printf("failed to read rtcp: %s", rtcpErr.Error())
				close(closeCh)
				return
			}
		}
	}()

	go th.videoTrackWriter(track, closeCh)
}

func (th *TestHelper) newVoiceTrack() *webrtc.TrackLocalStaticSample {
	th.tb.Helper()

	track, err := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{
		MimeType:     "audio/opus",
		ClockRate:    48000,
		Channels:     2,
		SDPFmtpLine:  "minptime=10;useinbandfec=1",
		RTCPFeedback: nil,
	}, "audio", "voice_"+random.NewID())
	require.NoError(th.tb, err)

	return track
}

func (th *TestHelper) voiceTrackWriter(track *webrtc.TrackLocalStaticSample, closeCh <-chan struct{}) {
	th.tb.Helper()

	// Open a OGG file and start reading using our OGGReader
	file, oggErr := os.Open("../testfiles/audio.ogg")
	require.NoError(th.tb, oggErr)
	defer file.Close()

	// Open on oggfile in non-checksum mode.
	ogg, _, oggErr := oggreader.NewWith(file)
	require.NoError(th.tb, oggErr)

	// Keep track of last granule, the difference is the amount of samples in the buffer
	var lastGranule uint64

	// It is important to use a time.Ticker instead of time.Sleep because
	// * avoids accumulating skew, just calling time.Sleep didn't compensate for the time spent parsing the data
	// * works around latency issues with Sleep (see https://github.com/golang/go/issues/44343)
	oggPageDuration := time.Millisecond * 20
	ticker := time.NewTicker(oggPageDuration)
	for {
		select {
		case <-closeCh:
			log.Printf("existing ogg reader")
			return
		case <-ticker.C:
		}

		var oggErr error
		var pageData []byte
		var pageHeader *oggreader.OggPageHeader
		pageData, pageHeader, oggErr = ogg.ParseNextPage()
		if oggErr == io.EOF {
			ogg.ResetReader(func(_ int64) io.Reader {
				_, _ = file.Seek(0, 0)
				return file
			})
			pageData, pageHeader, oggErr = ogg.ParseNextPage()
		}
		require.NoError(th.tb, oggErr)

		// The amount of samples is the difference between the last and current timestamp
		sampleCount := float64(pageHeader.GranulePosition - lastGranule)
		lastGranule = pageHeader.GranulePosition
		sampleDuration := time.Duration((sampleCount/48000)*1000) * time.Millisecond

		if err := track.WriteSample(media.Sample{Data: pageData, Duration: sampleDuration}); err != nil {
			log.Printf("failed to write audio sample: %s", err)
			return
		}
	}
}

func (th *TestHelper) transmitAudioTrack(c *Client) {
	th.tb.Helper()

	track := th.newVoiceTrack()

	sender, err := c.pc.AddTrack(track)
	require.NoError(th.tb, err)

	closeCh := make(chan struct{})
	go func() {
		rtcpBuf := make([]byte, receiveMTU)
		for {
			if _, _, rtcpErr := sender.Read(rtcpBuf); rtcpErr != nil {
				log.Printf("failed to read rtcp: %s", rtcpErr.Error())
				close(closeCh)
				return
			}
		}
	}()

	go th.voiceTrackWriter(track, closeCh)
}

func (th *TestHelper) ensureUser(client *model.Client4, username, password string) *model.User {
	th.tb.Helper()

	apiRequestTimeout := 10 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), apiRequestTimeout)
	defer cancel()
	user, _, err := client.Login(ctx, username, password)
	if err != nil {
		user, _, err = client.CreateUser(ctx, &model.User{
			Username: username,
			Password: password,
			Email:    fmt.Sprintf("%s@example.com", username),
		})
		require.Nil(th.tb, err)
		_, _, err = client.Login(ctx, username, password)
		require.Nil(th.tb, err)
	}

	return user
}

func (th *TestHelper) ensureTeamAndChannels() {
	apiRequestTimeout := 10 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), apiRequestTimeout)
	defer cancel()

	var err error
	th.team, _, err = th.adminAPIClient.GetTeamByName(ctx, teamName, "")
	if err != nil {
		th.team, _, err = th.adminAPIClient.CreateTeam(ctx, &model.Team{
			Name:        teamName,
			DisplayName: teamName,
			Type:        model.TeamOpen,
		})
		require.Nil(th.tb, err)
	}

	for _, user := range th.users {
		_, _, err := th.adminAPIClient.AddTeamMember(ctx, th.team.Id, user.Id)
		require.Nil(th.tb, err)
	}

	for i := 0; i < nChannels; i++ {
		channelName := fmt.Sprintf("%s%d", teamName, i)
		ch, _, err := th.adminAPIClient.GetChannelByName(ctx, channelName, th.team.Id, "")
		if err != nil {
			ch, _, err = th.adminAPIClient.CreateChannel(ctx, &model.Channel{
				Name:        channelName,
				DisplayName: channelName,
				TeamId:      th.team.Id,
				Type:        model.ChannelTypeOpen,
			})
			require.Nil(th.tb, err)
		}

		th.channels[channelName] = ch

		for _, user := range th.users {
			_, _, err := th.adminAPIClient.AddChannelMember(ctx, ch.Id, user.Id)
			require.Nil(th.tb, err)
		}
	}
}

func setupTestHelper(tb testing.TB, channelName string) *TestHelper {
	tb.Helper()
	var err error

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelDebug,
	}))

	th := &TestHelper{
		tb:       tb,
		channels: make(map[string]*model.Channel),
	}

	th.apiURL = "http://localhost:8065"

	th.adminAPIClient = model.NewAPIv4Client(th.apiURL)
	th.userAPIClient = model.NewAPIv4Client(th.apiURL)

	th.users = []*model.User{
		th.ensureUser(th.adminAPIClient, adminName, adminPass),
		th.ensureUser(th.userAPIClient, userName, userPass),
	}

	th.ensureTeamAndChannels()

	var channelID string
	if channelName != "" {
		channelID = th.channels[channelName].Id
	} else {
		channelID = random.NewID()
	}

	th.adminClient, err = New(Config{
		SiteURL:          th.apiURL,
		AuthToken:        th.adminAPIClient.AuthToken,
		ChannelID:        channelID,
		EnableRTCMonitor: true,
	}, WithLogger(logger))
	require.NoError(tb, err)
	require.NotNil(tb, th.adminClient)

	th.userClient, err = New(Config{
		SiteURL:          th.apiURL,
		AuthToken:        th.userAPIClient.AuthToken,
		ChannelID:        channelID,
		EnableRTCMonitor: true,
	}, WithLogger(logger))
	require.NoError(tb, err)
	require.NotNil(tb, th.userClient)

	return th
}
