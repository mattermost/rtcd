// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"

	"github.com/pion/webrtc/v3"
	"github.com/vmihailenco/msgpack/v5"
)

// Message structure is flat.
// The byte in front of the buffer identifies the type of message (DCMessageType).
// The remaining data (if present) constitutes the payload (e.g. DCMessageSDP).

type DCMessageType uint8

const (
	DCMessageTypePing DCMessageType = iota + 1 // no payload
	DCMessageTypePong                          // no payload
	DCMessageTypeSDP                           // DCMessageSDP
)

// Supported payloads
type DCMessageSDP []byte // payload is zlib compressed data of a JSON serialized webrtc.SessionDescription

func unpackData(data []byte) ([]byte, error) {
	rd, err := zlib.NewReader(bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create zlib reader: %w", err)
	}
	unpacked, err := io.ReadAll(rd)
	if err != nil {
		return nil, fmt.Errorf("failed to read zlib data: %w", err)
	}
	return unpacked, nil
}

func packData(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	wr := zlib.NewWriter(&buf)
	_, err := wr.Write(data)
	if err != nil {
		return nil, fmt.Errorf("failed to write zlib data: %w", err)
	}
	if err := wr.Close(); err != nil {
		return nil, fmt.Errorf("failed to close zlib writer: %w", err)
	}
	return buf.Bytes(), nil
}

func encodeDCMessage(mt DCMessageType, payload any) ([]byte, error) {
	enc := msgpack.GetEncoder()
	defer msgpack.PutEncoder(enc)
	var buf bytes.Buffer
	enc.ResetWriter(&buf)

	var err error
	// payload is optional
	if payload != nil {
		if mt == DCMessageTypeSDP {
			payload, err = packData(payload.([]byte))
			if err != nil {
				return nil, fmt.Errorf("failed to pack payload: %w", err)
			}
		}

		err = enc.EncodeMulti(mt, payload)
	} else {
		err = enc.EncodeUint8(uint8(mt))
	}

	return buf.Bytes(), err
}

func decodeDCMessage(msg []byte) (DCMessageType, any, error) {
	dec := msgpack.GetDecoder()
	defer msgpack.PutDecoder(dec)
	dec.ResetReader(bytes.NewReader(msg))

	// Decode MessageType
	t, err := dec.DecodeUint8()
	if err != nil {
		return 0, nil, fmt.Errorf("failed to decode dc message type: %w", err)
	}

	// Decode payload (if needed)
	switch DCMessageType(t) {
	case DCMessageTypePong:
		return DCMessageTypePong, nil, nil
	case DCMessageTypePing:
		return DCMessageTypePing, nil, nil
	case DCMessageTypeSDP:
		var payload DCMessageSDP
		err := dec.Decode(&payload)
		if err != nil {
			return 0, nil, fmt.Errorf("failed to decode sdp message: %w", err)
		}
		unpacked, err := unpackData(payload)
		if err != nil {
			return 0, nil, fmt.Errorf("failed to unpack sdp data: %w", err)
		}
		return DCMessageTypeSDP, unpacked, nil
	}

	return 0, nil, fmt.Errorf("unexpected dc message type: %d", t)
}

func (s *Server) handleDCMessage(data []byte, us *session, dc *webrtc.DataChannel) error {
	mt, payload, err := decodeDCMessage(data)
	if err != nil {
		return fmt.Errorf("failed to decode DC message: %w", err)
	}

	// Identify and handle message
	switch mt {
	case DCMessageTypePong:
		// nothing to do as pong is only received by clients at this point
	case DCMessageTypePing:
		data, err := encodeDCMessage(DCMessageTypePong, nil)
		if err != nil {
			return fmt.Errorf("failed to encode pong message: %w", err)
		}

		if err := dc.Send(data); err != nil {
			return fmt.Errorf("failed to send pong message: %w", err)
		}
	case DCMessageTypeSDP:
		if err := s.handleIncomingSDP(us, us.dcSDPCh, payload.([]byte)); err != nil {
			return fmt.Errorf("failed to handle incoming sdp message: %w", err)
		}
	}

	return nil
}
