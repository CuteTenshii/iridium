package main

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net"
	"slices"
	"strconv"
	"strings"
)

var IgnoredHeaders = []string{
	"X-Forwarded-For",
}

func MakeProxyRequest(conn net.Conn, request HttpRequest, targetHost string) HttpRequest {
	proxyRequest := HttpRequest{
		Method:  request.Method,
		Body:    request.Body,
		Headers: map[string]string{},
		Path:    request.Path,
		Version: request.Version,
		Status:  200,
	}
	for k, v := range request.Headers {
		if !slices.Contains(IgnoredHeaders, k) {
			proxyRequest.Headers[k] = v
		}
	}
	if strings.Contains(targetHost, ":") {
		// If targetHost includes a port, extract just the hostname part for the Host header
		hostParts := strings.Split(targetHost, ":")
		proxyRequest.Headers["Host"] = hostParts[0]
	} else {
		proxyRequest.Headers["Host"] = targetHost
		targetHost = targetHost + ":80"
	}
	localAddr := conn.LocalAddr().String()
	proxyRequest.Headers["X-Forwarded-For"] = strings.Split(localAddr, ":")[0]
	proxyRequest.Headers["Accept-Encoding"] = ""

	req, err := tls.Dial("tcp", targetHost, &tls.Config{})
	if err != nil {
		if errors.Is(err, net.ErrWriteToConnected) {
			ServeError(conn, 502)
			conn.Close()
			return proxyRequest
		} else if strings.Contains(err.Error(), "connection refused") {
			ServeError(conn, 502)
			conn.Close()
			return proxyRequest
		} else if strings.Contains(err.Error(), "i/o timeout") {
			ServeError(conn, 504)
			conn.Close()
			return proxyRequest
		} else {
			ServeError(conn, 502)
			conn.Close()
			return proxyRequest
		}
	}
	req.Write([]byte(request.Method + " " + request.Path + " " + request.Version + CRLF))
	for k, v := range proxyRequest.Headers {
		req.Write([]byte(k + ": " + v + CRLF))
	}
	req.Write([]byte(CRLF))
	if proxyRequest.Body != "" {
		req.Write([]byte(proxyRequest.Body))
	}

	var response HttpRequest
	response, err = ReadProxyResponse(req, request.Path)
	if err != nil {
		log.Println("Error reading proxy response:", err.Error())
		ServeError(conn, 502)
		conn.Close()
		return proxyRequest
	}
	contentType, ok := response.Headers["Content-Type"]
	if !ok {
		contentType = "text/html; charset=utf-8"
	}
	ServeResponse(conn, ResponseServed{
		Status:      response.Status,
		Body:        response.Body,
		ContentType: &contentType,
		Headers:     response.Headers,
	})

	return proxyRequest
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
			key := strings.TrimSpace(hparts[0])
			value := strings.TrimSpace(hparts[1])
			response.Headers[key] = value
		} else {
			log.Println("Malformed header:", line)
			continue // Skip malformed headers
		}
	}

	if te, ok := response.Headers["Transfer-Encoding"]; ok && strings.EqualFold(te, "chunked") {
		response.Body, err = ReadChunkedBody(reader)
		if err != nil {
			return response, err
		}
	} else if cl, ok := response.Headers["Content-Length"]; ok {
		response.Body, err = ReadContentLengthBody(reader, cl)
		if err != nil {
			return response, err
		}
	}

	return response, nil
}
