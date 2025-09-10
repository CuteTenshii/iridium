package main

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

func StartHTTPSRedirector() {
	listener, err := net.Listen("tcp", ":80")
	if err != nil {
		fmt.Printf("Failed to start HTTP redirector: %v\n", err)
		return
	}
	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Printf("Failed to accept connection: %v\n", err)
			continue
		}
		go handleRedirectConn(conn)
	}
}

func handleRedirectConn(conn net.Conn) {
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))

	reader := bufio.NewReader(conn)
	reqLine, err := reader.ReadString('\n')
	if err != nil {
		fmt.Printf("Failed to read request line: %v\n", err)
		return
	}

	parts := strings.Fields(reqLine)
	path := "/"
	if len(parts) >= 2 {
		path = parts[1]
	}

	var host string
	for {
		line, err := reader.ReadString('\n')
		if err != nil || line == "\r\n" {
			break
		}
		if strings.HasPrefix(strings.ToLower(line), "host:") {
			host = strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
		}
	}
	if host == "" {
		conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
		return
	}

	headers := map[string]string{
		"Server":     fmt.Sprintf("Iridium/%s", VERSION),
		"Connection": "close",
		"Date":       time.Now().UTC().Format(http.TimeFormat),
		"Location":   fmt.Sprintf("https://%s%s", host, path),
	}

	fmt.Fprint(conn, "HTTP/1.1 301 Moved Permanently\r\n")
	for k, v := range headers {
		fmt.Fprintf(conn, "%s: %s\r\n", k, v)
	}
	fmt.Fprint(conn, "\r\n")
}
