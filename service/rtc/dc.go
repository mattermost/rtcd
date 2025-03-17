// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"fmt"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/shared/mlog"
	"github.com/mattermost/rtcd/service/rtc/dc"
	"github.com/pion/webrtc/v4"
)

func (s *Server) handleDC(us *session, dataCh *webrtc.DataChannel) {
	s.log.Debug("data channel open", mlog.String("sessionID", us.cfg.SessionID))

	select {
	case us.dcOpenCh <- struct{}{}:
	default:
		s.log.Error("failed to send open dc message", mlog.String("sessionID", us.cfg.SessionID))
	}

	go func() {
		for {
			select {
			case msg := <-us.dcOutCh:
				dcMsg, err := dc.EncodeMessage(msg.msgType, msg.payload)
				if err != nil {
					s.log.Error("failed to encode sdp message", mlog.Err(err), mlog.String("sessionID", us.cfg.SessionID))
					continue
				}

				if err := dataCh.Send(dcMsg); err != nil {
					s.log.Error("failed to send sdp message", mlog.Err(err), mlog.String("sessionID", us.cfg.SessionID))
					continue
				}
			case msg := <-us.dcSDPCh:
				dcMsg, err := dc.EncodeMessage(dc.MessageTypeSDP, msg.Data)
				if err != nil {
					s.log.Error("failed to encode dc message", mlog.Err(err), mlog.String("sessionID", us.cfg.SessionID))
					continue
				}

				if err := dataCh.Send(dcMsg); err != nil {
					s.log.Error("failed to send dc message", mlog.Err(err), mlog.String("sessionID", us.cfg.SessionID))
					continue
				}
			case <-us.closeCh:
				return
			}
		}
	}()

	dataCh.OnMessage(func(msg webrtc.DataChannelMessage) {
		// DEPRECATED
		// keeping this for compatibility with older clients (i.e. mobile)
		if string(msg.Data) == "ping" {
			if err := dataCh.SendText("pong"); err != nil {
				s.log.Error("failed to send message", mlog.Err(err), mlog.String("sessionID", us.cfg.SessionID))
			}
			return
		}

		if err := s.handleDCMessage(msg.Data, us, dataCh); err != nil {
			s.log.Error("failed to handle dc message", mlog.Err(err), mlog.String("sessionID", us.cfg.SessionID))
		}
	})
}

func (s *Server) handleDCMessage(data []byte, us *session, dataCh *webrtc.DataChannel) error {
	mt, payload, err := dc.DecodeMessage(data)
	if err != nil {
		return fmt.Errorf("failed to decode DC message: %w", err)
	}

	// Identify and handle message
	switch mt {
	case dc.MessageTypePong:
		// nothing to do as pong is only received by clients at this point
	case dc.MessageTypePing:
		data, err := dc.EncodeMessage(dc.MessageTypePong, nil)
		if err != nil {
			return fmt.Errorf("failed to encode pong message: %w", err)
		}

		if err := dataCh.Send(data); err != nil {
			return fmt.Errorf("failed to send pong message: %w", err)
		}
	case dc.MessageTypeSDP:
		if err := s.handleIncomingSDP(us, us.dcSDPCh, payload.([]byte)); err != nil {
			return fmt.Errorf("failed to handle incoming sdp message: %w", err)
		}
	case dc.MessageTypeLossRate:
		s.metrics.ObserveRTCClientLossRate(us.cfg.GroupID, payload.(float64))
	case dc.MessageTypeRoundTripTime:
		s.metrics.ObserveRTCClientRTT(us.cfg.GroupID, payload.(float64))
	case dc.MessageTypeJitter:
		s.metrics.ObserveRTCClientJitter(us.cfg.GroupID, payload.(float64))
	case dc.MessageTypeLock:
		locked := us.signalingLock.TryLock()
		if locked {
			us.startLockTime.Store(model.NewPointer(time.Now()))
		}

		s.log.Debug("received lock message", mlog.String("sessionID", us.cfg.SessionID), mlog.Bool("locked", locked))

		select {
		case us.dcOutCh <- dcMessage{
			msgType: dc.MessageTypeLock,
			payload: locked,
		}:
		default:
			return fmt.Errorf("failed to send lock message: channel is full")
		}
	case dc.MessageTypeUnlock:
		if startLockTime := us.startLockTime.Load(); startLockTime != nil && !startLockTime.IsZero() {
			s.metrics.ObserveRTCSignalingLockLockedTime(us.cfg.GroupID, time.Since(*startLockTime).Seconds())
			us.startLockTime.Store(model.NewPointer(time.Time{}))
		}

		s.log.Debug("received unlock message", mlog.String("sessionID", us.cfg.SessionID))
		if err := us.signalingLock.Unlock(); err != nil {
			return fmt.Errorf("failed to unlock signaling lock: %w", err)
		}
	}

	return nil
}
