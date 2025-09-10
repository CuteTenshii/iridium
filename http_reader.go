package main

import (
	"bufio"
	"fmt"
	"io"
	"iridium/http2"
	"log"
	"net"
	"slices"
	"strconv"
	"strings"
)

var (
	// HttpMethods List of supported HTTP methods
	HttpMethods = []string{"GET", "POST", "PATCH", "PUT", "DELETE", "HEAD", "OPTIONS", "TRACE", "CONNECT"}
	// HttpVersions List of supported HTTP versions
	HttpVersions = []string{"HTTP/1.0", "HTTP/1.1", "HTTP/2.0"}
)

const CRLF = "\r\n"

type HttpRequest struct {
	Version  string
	Method   string
	Path     string
	Headers  map[string]string
	Body     string
	Status   int
	StreamID *uint32
}

// ReadRequest reads and parses an HTTP request from the given connection.
// It supports both HTTP/1.x and HTTP/2 based on the ALPN protocol.
func ReadRequest(conn net.Conn, alpnProto string) (HttpRequest, error) {
	reader := bufio.NewReader(conn)
	var request HttpRequest

	if alpnProto == "h2" {
		// HTTP/2 over TLS - skip text parsing and handle preface directly
		preface, err := http2.HandlePreface(conn, true)
		if err != nil {
			return request, err
		}
		request.Method = preface.Method
		request.Path = preface.Path
		request.Headers = preface.Headers
		request.Body = string(preface.Body)
		request.Version = "HTTP/2.0"
		request.StreamID = &preface.StreamID
		return request, nil
	}

	// Fallback to HTTP/1.x or h2c (HTTP/2 cleartext) parsing
	line, err := reader.ReadString('\n')
	if err != nil {
		return request, fmt.Errorf("failed to read request line: %v", err)
	}

	var method, path, version string
	// Parses the request line. Example: "GET /path HTTP/1.1"
	n, err := fmt.Sscanf(line, "%s %s %s", &method, &path, &version)
	if err != nil || n != 3 {
		return request, fmt.Errorf("malformed request")
	}

	// Handle HTTP/2 preface
	if method == "PRI" && path == "*" && version == "HTTP/2.0" {
		preface, err := http2.HandlePreface(conn, false)
		if err != nil {
			return request, fmt.Errorf("failed to handle HTTP/2 preface: %v", err)
		}
		request.Method = preface.Method
		request.Path = preface.Path
		request.Headers = preface.Headers
		request.Body = string(preface.Body)
		request.Version = "HTTP/2.0"
		request.StreamID = &preface.StreamID
		return request, nil
	} else if !slices.Contains(HttpMethods, method) {
		return request, fmt.Errorf("unsupported HTTP method: %s", method)
	} else if !slices.Contains(HttpVersions, version) {
		return request, fmt.Errorf("unsupported HTTP version: %s", version)
	}

	request.Headers = make(map[string]string)
	request.Method = method
	request.Path = path
	request.Version = version

	// Read headers
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return request, err
		}
		if line == CRLF {
			break // End of headers
		}
		hparts := strings.SplitN(strings.TrimRight(line, "\r\n"), ":", 2)
		if len(hparts) == 2 {
			k := strings.TrimSpace(strings.ToLower(hparts[0]))
			v := strings.TrimSpace(hparts[1])
			request.Headers[k] = v
		} else {
			log.Println("Malformed header:", line)
			continue // Skip malformed headers
		}
	}

	// Read body if Content-Length is specified
	if te, ok := request.Headers["transfer-encoding"]; ok && strings.EqualFold(te, "chunked") {
		request.Body, err = ReadChunkedBody(reader)
		if err != nil {
			return request, err
		}
	} else if cl, ok := request.Headers["content-length"]; ok {
		request.Body, err = ReadContentLengthBody(reader, cl)
		if err != nil {
			return request, err
		}
	}

	return request, nil
}

func ReadContentLengthBody(reader *bufio.Reader, contentLength string) (string, error) {
	length, err := strconv.Atoi(contentLength)
	if err != nil {
		return "", err
	}
	body := make([]byte, length)
	_, err = io.ReadFull(reader, body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func ReadChunkedBody(reader *bufio.Reader) (string, error) {
	var body strings.Builder
	for {
		sizeLine, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		sizeLine = strings.TrimSpace(sizeLine)
		size, err := strconv.ParseInt(sizeLine, 16, 64)
		if err != nil {
			return "", err
		}
		if size == 0 {
			// Read and discard trailing CRLF after last chunk
			_, _ = reader.ReadString('\n')
			break
		}
		chunk := make([]byte, size)
		_, err = io.ReadFull(reader, chunk)
		if err != nil {
			return "", err
		}
		body.Write(chunk)
		// Read and discard trailing CRLF after each chunk
		_, _ = reader.ReadString('\n')
	}
	return body.String(), nil
}
