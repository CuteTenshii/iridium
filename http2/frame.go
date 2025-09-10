package http2

import (
	"bufio"
	"encoding/binary"
	"io"
	"net"
)

// WriteFrame writes an HTTP/2 frame to the given connection.
// ft is the frame type, flags are the frame flags, streamID is the stream identifier,
// and payload is the frame payload.
func WriteFrame(conn net.Conn, ft byte, flags byte, streamID uint32, payload []byte) error {
	hdr := make([]byte, 9)
	// length is 24-bit
	length := uint32(len(payload))
	hdr[0] = byte(length >> 16)
	hdr[1] = byte(length >> 8)
	hdr[2] = byte(length)
	hdr[3] = ft
	hdr[4] = flags
	// top bit of stream id reserved
	binary.BigEndian.PutUint32(hdr[5:], streamID&0x7FFFFFFF)
	if _, err := conn.Write(hdr); err != nil {
		return err
	}
	if len(payload) > 0 {
		_, err := conn.Write(payload)
		return err
	}
	return nil
}

// ReadFrame reads an HTTP/2 frame from the given bufio.Reader.
// It returns the frame type, flags, stream ID, payload, and any error encountered.
func ReadFrame(reader *bufio.Reader) (byte, byte, uint32, []byte, error) {
	hdr := make([]byte, 9)
	if _, err := io.ReadFull(reader, hdr); err != nil {
		return 0, 0, 0, nil, err
	}

	// parse header
	length := int(hdr[0])<<16 | int(hdr[1])<<8 | int(hdr[2])
	ft := hdr[3]
	flags := hdr[4]
	streamID := binary.BigEndian.Uint32(hdr[5:]) & 0x7fffffff

	// read full payload
	payload := make([]byte, length)
	if length > 0 {
		if _, err := io.ReadFull(reader, payload); err != nil {
			return 0, 0, 0, nil, err
		}
	}

	return ft, flags, streamID, payload, nil
}

func ParseSettingsFrame(payload []byte) map[uint16]uint32 {
	settings := make(map[uint16]uint32)
	for i := 0; i+6 <= len(payload); i += 6 {
		id := binary.BigEndian.Uint16(payload[i : i+2])
		value := binary.BigEndian.Uint32(payload[i+2 : i+6])
		settings[id] = value
	}
	return settings
}
