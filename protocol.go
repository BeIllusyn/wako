package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

// Frames are how wako talks to a run's supervisor over its control socket.
// Each frame is a 1-byte type followed by a 4-byte big-endian length and the
// payload:
//
//	H <token>  client hello, proves it can read the run's state file
//	O <bytes>  service output (supervisor -> client)
//	I <bytes>  terminal input (client -> supervisor -> service stdin)
//	C          close the service's stdin
//	S          stop the service (client -> supervisor)
//	E <text>   the run failed to start (supervisor -> client)
//	X <json>   the service exited, with its exit code (supervisor -> client)
const (
	frameHello      = 'H'
	frameOutput     = 'O'
	frameInput      = 'I'
	frameStdinClose = 'C'
	frameStop       = 'S'
	frameError      = 'E'
	frameExit       = 'X'
)

const maxFrameSize = 1 << 20

type exitInfo struct {
	Code int `json:"code"`
}

func encodeFrame(typ byte, payload []byte) []byte {
	frame := make([]byte, 5+len(payload))
	frame[0] = typ
	binary.BigEndian.PutUint32(frame[1:], uint32(len(payload)))
	copy(frame[5:], payload)
	return frame
}

func exitPayload(code int) []byte {
	data, err := json.Marshal(exitInfo{Code: code})
	if err != nil {
		return []byte(`{"code":1}`)
	}
	return data
}

func writeFrame(w io.Writer, typ byte, payload []byte) error {
	_, err := w.Write(encodeFrame(typ, payload))
	return err
}

func readFrame(r io.Reader) (byte, []byte, error) {
	var header [5]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return 0, nil, err
	}

	size := binary.BigEndian.Uint32(header[1:])
	if size > maxFrameSize {
		return 0, nil, fmt.Errorf("frame too large (%d bytes)", size)
	}

	payload := make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}
	return header[0], payload, nil
}
