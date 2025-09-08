package main

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/http/httputil"
	"slices"
	"strconv"
	"strings"
	"time"
)

var ClientIgnoredHeaders = []string{
	"x-forwarded-for", "host",
}
var ServerIgnoredHeaders = []string{
	"content-encoding", "content-length", "transfer-encoding", "connection", "keep-alive", "alt-svc", "server",
	"content-type", "date", "vary",
}

// GetLocalIpWithoutPort extracts the IP address from a given address string, removing the port if present.
func GetLocalIpWithoutPort(addr string) string {
	split := strings.Split(addr, ":")
	if len(split) < 2 {
		return addr
	}
	return strings.TrimSuffix(addr, ":"+split[len(split)-1])
}

// DialTarget tries to connect to the target host using TLS first, and falls back to plain TCP if TLS fails due to the target not supporting it.
func DialTarget(targetHost string) (net.Conn, error) {
	// Timeout after 90 seconds
	gatewayTimeout := 90 * time.Second
	dialer := &net.Dialer{Timeout: gatewayTimeout}
	tlsConn, err := tls.DialWithDialer(dialer, "tcp", targetHost, &tls.Config{})
	if err == nil {
		_ = tlsConn.SetDeadline(time.Now().Add(gatewayTimeout))
		return tlsConn, nil
	}

	// Fallback to plain TCP if TLS handshake fails
	if strings.Contains(err.Error(), "first record does not look like a TLS handshake") {
		conn, err := dialer.Dial("tcp", targetHost)
		if err != nil {
			return nil, err
		}
		_ = conn.SetDeadline(time.Now().Add(gatewayTimeout))
		return conn, nil
	}

	return nil, err
}

// MakeProxyRequest constructs and sends a proxied HTTP request to the target host, then reads and serves the response back to the client.
func MakeProxyRequest(conn net.Conn, request HttpRequest, targetHost string) (*HttpRequest, error) {
	proxyRequest := HttpRequest{
		Method:  request.Method,
		Body:    request.Body,
		Headers: map[string]string{},
		Path:    request.Path,
		Version: request.Version,
		Status:  200,
	}
	for k, v := range request.Headers {
		k = strings.TrimSpace(strings.ToLower(k))
		if !slices.Contains(ClientIgnoredHeaders, k) {
			proxyRequest.Headers[k] = v
		}
	}

	// Ensure "Host" header is set correctly
	if strings.Contains(targetHost, ":") {
		// If targetHost includes a port, extract just the hostname part for the Host header
		proxyRequest.Headers["host"] = GetLocalIpWithoutPort(targetHost)
	} else {
		proxyRequest.Headers["host"] = targetHost
		// Default to port 80 if no port is specified
		targetHost = targetHost + ":80"
	}
	localAddr := conn.LocalAddr().String()
	proxyRequest.Headers["x-forwarded-for"] = GetLocalIpWithoutPort(localAddr)
	proxyRequest.Headers["accept-encoding"] = "gzip, deflate, zstd"
	proxyRequest.Headers["connection"] = "keep-alive"

	req, err := DialTarget(targetHost)
	if err != nil {
		if strings.Contains(err.Error(), "i/o timeout") {
			ErrorLog(err)
			ServeError(conn, request, 504)
			conn.Close()
			return nil, err
		} else {
			ErrorLog(err)
			ServeError(conn, request, 502)
			conn.Close()
			return nil, err
		}
	}

	req.Write([]byte(request.Method + " " + request.Path + " " + request.Version + CRLF))
	for k, v := range proxyRequest.Headers {
		req.Write([]byte(fmt.Sprintf("%s: %s\r\n", k, v)))
	}
	req.Write([]byte(CRLF))
	if proxyRequest.Body != "" {
		req.Write([]byte(proxyRequest.Body))
	}

	var response HttpRequest
	response, err = ReadProxyResponse(req, request.Path)
	if err != nil {
		if strings.Contains(err.Error(), "i/o timeout") {
			ErrorLog(err)
			ServeError(conn, request, 504)
			conn.Close()
			return nil, err
		}
		log.Println("Error reading proxy response:", err.Error())
		ServeError(conn, request, 500)
		return nil, err
	}

	return &response, nil
}

func ReadProxyResponse(conn net.Conn, path string) (HttpRequest, error) {
	reader := bufio.NewReader(conn)

	var response HttpRequest
	response.Headers = make(map[string]string)

	line, err := reader.ReadString('\n')
	if err != nil {
		return response, fmt.Errorf("failed to read response line: %v", err)
	}

	// Parse the response line. Example: "HTTP/1.1 200"
	fmtParts := make([]string, 2)
	n, _ := fmt.Sscanf(line, "%s %s", &fmtParts[0], &fmtParts[1])
	if n != 2 {
		return response, fmt.Errorf("malformed response")
	}
	if !slices.Contains(HttpVersions, fmtParts[0]) {
		return response, fmt.Errorf("unsupported method or version")
	}

	response.Method = "GET"
	response.Path = path
	response.Version = fmtParts[0]
	statusCode, err := strconv.Atoi(fmtParts[1])
	if err != nil {
		return response, fmt.Errorf("invalid status code")
	}
	response.Status = statusCode

	// Read headers
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return response, err
		}
		if line == CRLF {
			break // End of headers
		}
		hparts := strings.SplitN(strings.TrimRight(line, "\r\n"), ":", 2)
		if len(hparts) == 2 {
			k := strings.TrimSpace(strings.ToLower(hparts[0]))
			v := strings.TrimSpace(hparts[1])
			response.Headers[k] = v
		} else {
			log.Println("Malformed header:", line)
			continue // Skip malformed headers
		}
	}

	response.Body = ""
	if response.Status < 100 || response.Status > 599 {
		return response, fmt.Errorf("invalid status code: %d", response.Status)
		// Status codes: 204 (No Content), 304 (Not Modified), and 1xx (Informational) do not have a body
	} else if response.Status == 204 || response.Status == 304 || (response.Status >= 100 && response.Status < 200) {
		return response, nil
	}

	if ce, ok := response.Headers["content-encoding"]; ok && (strings.EqualFold(ce, "gzip") || strings.EqualFold(ce, "zstd") ||
		strings.EqualFold(ce, "deflate")) {
		// Handle chunked transfer encoding if present
		if strings.EqualFold(response.Headers["transfer-encoding"], "chunked") {
			reader = bufio.NewReader(httputil.NewChunkedReader(reader))
		}
		decompressed, err := DecompressBody(reader, strings.ToLower(ce))
		if err != nil {
			return response, fmt.Errorf("failed to decompress response body: %v", err)
		}
		bodyBytes, err := io.ReadAll(decompressed)
		if err != nil {
			return response, fmt.Errorf("failed to read decompressed response body: %v", err)
		}
		response.Body = string(bodyBytes)
		return response, nil
	}

	if te, ok := response.Headers["transfer-encoding"]; ok && strings.EqualFold(te, "chunked") {
		response.Body, err = ReadChunkedBody(reader)
		if err != nil {
			return response, err
		}
	} else if cl, ok := response.Headers["content-length"]; ok {
		response.Body, err = ReadContentLengthBody(reader, cl)
		if err != nil {
			return response, err
		}
	}

	return response, nil
}
