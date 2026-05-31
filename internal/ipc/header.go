package ipc

import (
	"encoding/binary"
	"fmt"
	"io"
)

const (
	Magic      = 0x414E5453 // "ANTS"
	HeaderSize = 32
	Version    = 0x01
)

type Direction uint8

const (
	DirGoToJS Direction = 0x00
	DirJSToGo Direction = 0x01
)

type MessageType uint8

const (
	MsgDispatch MessageType = 0x00
	MsgResult   MessageType = 0x01
	MsgPing     MessageType = 0x02
	MsgError    MessageType = 0x03
)

type Header struct {
	Magic      uint32
	Version    uint8
	Direction  Direction
	MsgType    MessageType
	Flags      uint8
	RequestID  uint32
	HandlerID  uint32
	PayloadLen uint32
	StatusCode uint32
	Reserved   [8]byte
}

func NewHeader(dir Direction, msgType MessageType, requestID, handlerID, statusCode uint32, payloadLen uint32) Header {
	return Header{
		Magic:      Magic,
		Version:    Version,
		Direction:  dir,
		MsgType:    msgType,
		RequestID:  requestID,
		HandlerID:  handlerID,
		PayloadLen: payloadLen,
		StatusCode: statusCode,
	}
}

func (h *Header) Encode() [HeaderSize]byte {
	var buf [HeaderSize]byte
	binary.LittleEndian.PutUint32(buf[0:4], h.Magic)
	buf[4] = h.Version
	buf[5] = byte(h.Direction)
	buf[6] = byte(h.MsgType)
	buf[7] = h.Flags
	binary.LittleEndian.PutUint32(buf[8:12], h.RequestID)
	binary.LittleEndian.PutUint32(buf[12:16], h.HandlerID)
	binary.LittleEndian.PutUint32(buf[16:20], h.PayloadLen)
	binary.LittleEndian.PutUint32(buf[20:24], h.StatusCode)
	copy(buf[24:32], h.Reserved[:])
	return buf
}

func DecodeHeader(buf [HeaderSize]byte) (Header, error) {
	magic := binary.LittleEndian.Uint32(buf[0:4])
	if magic != Magic {
		return Header{}, fmt.Errorf("ipc: invalid magic: 0x%08X", magic)
	}

	return Header{
		Magic:      magic,
		Version:    buf[4],
		Direction:  Direction(buf[5]),
		MsgType:    MessageType(buf[6]),
		Flags:      buf[7],
		RequestID:  binary.LittleEndian.Uint32(buf[8:12]),
		HandlerID:  binary.LittleEndian.Uint32(buf[12:16]),
		PayloadLen: binary.LittleEndian.Uint32(buf[16:20]),
		StatusCode: binary.LittleEndian.Uint32(buf[20:24]),
	}, nil
}

// ReadHeader reads a header from a reader (e.g., net.Conn).
func ReadHeader(r io.Reader) (Header, error) {
	var buf [HeaderSize]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return Header{}, fmt.Errorf("ipc: read header: %w", err)
	}
	return DecodeHeader(buf)
}

// WriteHeader writes a header to a writer.
func WriteHeader(w io.Writer, h Header) error {
	buf := h.Encode()
	_, err := w.Write(buf[:])
	return err
}
