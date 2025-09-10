package http2

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"

	"golang.org/x/net/http2/hpack"
)

type H2Request struct {
	Method   string
	Path     string
	Headers  map[string]string
	Body     []byte
	StreamID uint32
}

// HandlePreface handles the HTTP/2 connection preface.
// If tls is true, it assumes the connection is over TLS and skips the preface check (since it's handled by ALPN).
func HandlePreface(conn net.Conn, tls bool) (*H2Request, error) {
	if tls {
		// h2c (cleartext): must receive PRI * HTTP/2.0 preface
		preface := make([]byte, len(ClientPreface))
		if _, err := io.ReadFull(conn, preface); err != nil {
			return nil, err
		}
		if string(preface) != ClientPreface {
			return nil, fmt.Errorf("invalid h2c preface: %q", preface)
		}
	}

	// In both TLS and h2c (HTTP/2 cleartext) cases, the client next sends a SETTINGS frame
	// Browsers typically use TLS, so they skip the preface and go straight to SETTINGS
	// Clients like cURL in h2c mode will send the preface followed by SETTINGS
	reader := bufio.NewReader(conn)

	// Read the client's SETTINGS frame
	ft, _, streamID, payload, err := ReadFrame(reader)
	if err != nil {
		return nil, err
	}
	if ft != SettingsFrameType || streamID != 0 {
		return nil, fmt.Errorf("expected SETTINGS frame on stream 0, got type=%d stream=%d", ft, streamID)
	}

	ParseSettingsFrame(payload)

	// ACK the client's SETTINGS
	if err := WriteFrame(conn, SettingsFrameType, AckFlag, 0, []byte{}); err != nil {
		return nil, fmt.Errorf("failed to send SETTINGS ACK: %w", err)
	}

	// Send our own SETTINGS (empty is fine to start)
	if err := WriteFrame(conn, SettingsFrameType, 0, 0, []byte{}); err != nil {
		return nil, fmt.Errorf("failed to send server SETTINGS: %w", err)
	}

	// Now enter the main frame processing loop
	for {
		ft, flags, streamID, payload, err := ReadFrame(reader)
		if err != nil {
			return nil, err
		}

		switch ft {
		case SettingsFrameType:
			if flags&AckFlag == 0 {
				// Must ACK these settings
				if err := WriteFrame(conn, SettingsFrameType, AckFlag, 0, []byte{}); err != nil {
					return nil, err
				}
			}
		case HeadersFrameType:
			index := 0
			headerBlock := payload
			if flags&PaddedFlag != 0 {
				padLength := int(payload[0])
				index = 1
				headerBlock = payload[index : len(payload)-padLength]
			}
			if flags&PriorityFlag != 0 {
				// 5 bytes: 4 for stream dependency + 1 for weight
				index += 5
				headerBlock = payload[index:len(payload)]
			}

			var request H2Request
			request.StreamID = streamID
			request.Headers = make(map[string]string)

			var headers []hpack.HeaderField
			decoder := hpack.NewDecoder(4096, func(f hpack.HeaderField) {
				headers = append(headers, f)
			})
			decoder.Write(headerBlock)
			for _, hf := range headers {
				pseudoHeader := hf.Name[0] == ':'
				if pseudoHeader {
					switch hf.Name {
					case ":method":
						request.Method = hf.Value
					case ":path":
						request.Path = hf.Value
					case ":scheme":
						if hf.Value != "https" && hf.Value != "http" {
							return nil, fmt.Errorf("unsupported scheme: %s", hf.Value)
						}
					case ":authority":
						request.Headers["host"] = hf.Value
					}
				} else {
					request.Headers[hf.Name] = hf.Value
				}
			}

			return &request, nil
		case PingFrameType:
			if err := WriteFrame(conn, PingFrameType, AckFlag, 0, payload); err != nil {
				return nil, err
			}
		case WindowUpdateFrameType:
		default:
			log.Printf("unhandled frame type=%d", ft)
		}
	}
}
