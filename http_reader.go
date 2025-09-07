package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"slices"
	"strconv"
	"strings"
)

var (
	// HttpMethods List of supported HTTP methods
	HttpMethods = []string{"GET", "POST", "PUT", "DELETE", "HEAD", "OPTIONS", "PATCH", "TRACE", "CONNECT"}
	// HttpVersions List of supported HTTP versions
	HttpVersions = []string{"HTTP/1.0", "HTTP/1.1", "HTTP/2.0"}
)

const CRLF = "\r\n"

type HttpRequest struct {
	Version string
	Method  string
	Path    string
	Headers map[string]string
	Body    string
	Status  int
}

// ReadRequest reads and parses an HTTP request from the given connection.
func ReadRequest(conn net.Conn) (HttpRequest, error) {
	reader := bufio.NewReader(conn)

	var request HttpRequest
	request.Headers = make(map[string]string)

	line, err := reader.ReadString('\n')
	if err != nil {
		return request, fmt.Errorf("failed to read request line: %v", err)
	}

	// Parse the request line. Example: "GET /path HTTP/1.1"
	fmtParts := make([]string, 3)
	n, _ := fmt.Sscanf(line, "%s %s %s", &fmtParts[0], &fmtParts[1], &fmtParts[2])
	if n != 3 {
		return request, fmt.Errorf("malformed request")
	}
	if !slices.Contains(HttpMethods, fmtParts[0]) || !slices.Contains(HttpVersions, fmtParts[2]) {
		return request, fmt.Errorf("%s is not a supported method or version", fmtParts[0]+" "+fmtParts[2])
	}

	request.Method = fmtParts[0]
	request.Path = fmtParts[1]
	request.Version = fmtParts[2]

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
			key := strings.TrimSpace(hparts[0])
			value := strings.TrimSpace(hparts[1])
			request.Headers[key] = value
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
