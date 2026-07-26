package peer

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const (
	PTYFrameVersion   = 1
	PTYDataInput      = 1
	PTYDataOutput     = 2
	ptyDataHeaderSize = 20
	maxPTYFrameData   = 64 << 10
	maxPTYReason      = 256
)

var ptyFrameMagic = [4]byte{'T', 'A', 'P', '1'}

type PTYDataFrame struct {
	Direction uint8
	Sequence  uint64
	Payload   []byte
}

func EncodePTYDataFrame(direction uint8, sequence uint64, payload []byte) ([]byte, error) {
	if direction != PTYDataInput && direction != PTYDataOutput {
		return nil, errors.New("invalid PTY data direction")
	}
	if sequence == 0 {
		return nil, errors.New("PTY data sequence must be positive")
	}
	if len(payload) > maxPTYFrameData {
		return nil, errors.New("PTY data payload exceeds limit")
	}
	frame := make([]byte, ptyDataHeaderSize+len(payload))
	copy(frame[:4], ptyFrameMagic[:])
	frame[4] = PTYFrameVersion
	frame[5] = direction
	binary.BigEndian.PutUint64(frame[8:16], sequence)
	binary.BigEndian.PutUint32(frame[16:20], uint32(len(payload)))
	copy(frame[20:], payload)
	return frame, nil
}

func DecodePTYDataFrame(data []byte) (PTYDataFrame, error) {
	if len(data) < ptyDataHeaderSize {
		return PTYDataFrame{}, errors.New("PTY data frame is truncated")
	}
	if !bytes.Equal(data[:4], ptyFrameMagic[:]) {
		return PTYDataFrame{}, errors.New("invalid PTY data magic")
	}
	if data[4] != PTYFrameVersion {
		return PTYDataFrame{}, fmt.Errorf("unsupported PTY frame version %d", data[4])
	}
	if data[6] != 0 || data[7] != 0 {
		return PTYDataFrame{}, errors.New("PTY data reserved bytes are non-zero")
	}
	direction := data[5]
	if direction != PTYDataInput && direction != PTYDataOutput {
		return PTYDataFrame{}, errors.New("invalid PTY data direction")
	}
	sequence := binary.BigEndian.Uint64(data[8:16])
	if sequence == 0 {
		return PTYDataFrame{}, errors.New("PTY data sequence must be positive")
	}
	payloadLength := binary.BigEndian.Uint32(data[16:20])
	if payloadLength > maxPTYFrameData || int(payloadLength) != len(data)-ptyDataHeaderSize {
		return PTYDataFrame{}, errors.New("invalid PTY data payload length")
	}
	payload := append([]byte(nil), data[ptyDataHeaderSize:]...)
	return PTYDataFrame{Direction: direction, Sequence: sequence, Payload: payload}, nil
}

type PTYControlFrame struct {
	Version  uint8  `json:"version"`
	Type     string `json:"type"`
	Sequence uint64 `json:"sequence"`
	Cols     uint16 `json:"cols,omitempty"`
	Rows     uint16 `json:"rows,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

func EncodePTYControlFrame(frame PTYControlFrame) ([]byte, error) {
	if err := frame.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(frame)
}

func DecodePTYControlFrame(data []byte) (PTYControlFrame, error) {
	var frame PTYControlFrame
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&frame); err != nil {
		return PTYControlFrame{}, fmt.Errorf("decode PTY control frame: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return PTYControlFrame{}, err
	}
	if err := frame.Validate(); err != nil {
		return PTYControlFrame{}, err
	}
	return frame, nil
}

func (frame PTYControlFrame) Validate() error {
	if frame.Version != PTYFrameVersion {
		return errors.New("invalid PTY control version")
	}
	if frame.Sequence == 0 {
		return errors.New("PTY control sequence must be positive")
	}
	switch frame.Type {
	case "resize":
		if frame.Cols == 0 || frame.Rows == 0 {
			return errors.New("PTY resize dimensions must be positive")
		}
		if frame.Reason != "" {
			return errors.New("PTY resize cannot contain reason")
		}
	case "close", "error":
		if frame.Cols != 0 || frame.Rows != 0 || len(frame.Reason) > maxPTYReason {
			return errors.New("invalid PTY close/error control frame")
		}
	default:
		return errors.New("invalid PTY control type")
	}
	return nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values are not allowed")
		}
		return err
	}
	return nil
}

type PTYSequence struct {
	next uint64
}

func (sequence *PTYSequence) Accept(value uint64) (duplicate bool, err error) {
	if sequence.next == 0 {
		sequence.next = 1
	}
	if value < sequence.next {
		return true, nil
	}
	if value > sequence.next {
		return false, fmt.Errorf("PTY sequence gap: got %d want %d", value, sequence.next)
	}
	sequence.next++
	return false, nil
}
