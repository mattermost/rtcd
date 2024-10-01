// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package dc

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"

	"github.com/vmihailenco/msgpack/v5"
)

// Message structure is flat.
// The byte in front of the buffer identifies the type of message (MessageType).
// The remaining data (if present) constitutes the payload (e.g. MessageSDP).

type MessageType uint8

const (
	MessageTypePing MessageType = iota + 1 // no payload
	MessageTypePong                        // no payload
	MessageTypeSDP                         // MessageSDP
)

// Supported payloads
type MessageSDP []byte // payload is zlib compressed data of a JSON serialized webrtc.SessionDescription

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

func EncodeMessage(mt MessageType, payload any) ([]byte, error) {
	enc := msgpack.GetEncoder()
	defer msgpack.PutEncoder(enc)
	var buf bytes.Buffer
	enc.ResetWriter(&buf)

	var err error
	// payload is optional
	if payload != nil {
		if mt == MessageTypeSDP {
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

func DecodeMessage(msg []byte) (MessageType, any, error) {
	dec := msgpack.GetDecoder()
	defer msgpack.PutDecoder(dec)
	dec.ResetReader(bytes.NewReader(msg))

	// Decode MessageType
	t, err := dec.DecodeUint8()
	if err != nil {
		return 0, nil, fmt.Errorf("failed to decode dc message type: %w", err)
	}

	// Decode payload (if needed)
	switch MessageType(t) {
	case MessageTypePong:
		return MessageTypePong, nil, nil
	case MessageTypePing:
		return MessageTypePing, nil, nil
	case MessageTypeSDP:
		var payload MessageSDP
		err := dec.Decode(&payload)
		if err != nil {
			return 0, nil, fmt.Errorf("failed to decode sdp message: %w", err)
		}
		unpacked, err := unpackData(payload)
		if err != nil {
			return 0, nil, fmt.Errorf("failed to unpack sdp data: %w", err)
		}
		return MessageTypeSDP, unpacked, nil
	}

	return 0, nil, fmt.Errorf("unexpected dc message type: %d", t)
}
