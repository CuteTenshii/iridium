package main

import (
	"fmt"
	"net"
)

func ServeFallback() string {
	html := `<html>
	  <head><title>Welcome to ReverseProxy</title></head>
	  <body>
		<h1>It works!</h1>
		<p>This is the default page served by ReverseProxy.</p>
	  </body>
	  </html>`
	return html
}

type ResponseServed struct {
	Status      int
	Body        string
	ContentType *string
	Headers     map[string]string
}

func ServeResponse(conn net.Conn, resp ResponseServed) {
	if resp.ContentType == nil {
		defaultType := "text/html; charset=utf-8"
		resp.ContentType = &defaultType
	}
	conn.Write([]byte(fmt.Sprintf("HTTP/1.1 %d\r\n", resp.Status)))
	conn.Write([]byte(fmt.Sprintf("Server: ReverseProxy/%s\r\n", VERSION)))
	conn.Write([]byte("Connection: close\r\n"))
	conn.Write([]byte(fmt.Sprintf("Content-Length: %d\r\n", len(resp.Body))))
	conn.Write([]byte(fmt.Sprintf("Content-Type: %s\r\n", *resp.ContentType)))
	if resp.Headers != nil {
		for k, v := range resp.Headers {
			if k == "Content-Length" || k == "Content-Type" || k == "Server" || k == "Connection" || k == "Date" ||
				k == "Content-Encoding" || k == "Transfer-Encoding" {
				continue
			}
			conn.Write([]byte(fmt.Sprintf("%s: %s\r\n", k, v)))
		}
	}
	conn.Write([]byte("\r\n"))
	conn.Write([]byte(resp.Body))
	conn.Close()
}

func ServeError(conn net.Conn, status int) {
	var statusText string
	switch status {
	case 400:
		statusText = "Bad Request"
	case 403:
		statusText = "Forbidden"
	case 404:
		statusText = "Not Found"
	case 500:
		statusText = "Internal Server Error"
	case 502:
		statusText = "Bad Gateway"
	case 503:
		statusText = "Service Unavailable"
	default:
		statusText = "Unknown Error"
	}
	body := fmt.Sprintf(
		"<!DOCTYPE html><html><head><title>%s</title></head><body><center><h1>%d %s</h1><hr><p>ReverseProxy</p></center></body></html>",
		statusText, status, statusText,
	)
	ServeResponse(conn, ResponseServed{
		Status: status,
		Body:   body,
	})
}
