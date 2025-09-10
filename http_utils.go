package main

import (
	"bytes"
	"fmt"
	"io"
	"iridium/http2"
	"net"
	"net/http"
	"slices"
	"strings"
	"time"

	"golang.org/x/net/http2/hpack"
)

func FallbackHtml() string {
	html := `<!DOCTYPE html>
<html>
  <head>
    <title>Welcome to Iridium!</title>
    <meta charset="utf-8">
  </head>
  <body>
	<center>
	  <h1>Welcome to Iridium!</h1>
	  <p>This is the default page served by Iridium.</p>
	  <p>If you see this page, it means that no host configuration matched your request.</p>
	  <hr>
	  <p>Iridium v` + VERSION + `</p>
	</center>
  </body>
</html>`
	return strings.TrimSpace(html)
}

// GetContentBody returns the body as a string and the content length
func GetContentBody(body []byte, encoding string) ([]byte, int) {
	if encoding == "zstd" || encoding == "gzip" || encoding == "deflate" {
		enc, err := CompressData(strings.NewReader(string(body)), encoding)
		if err != nil {
			fmt.Printf("Error compressing response: %v\n", err)
			return body, len(body)
		}
		ioData, err := io.ReadAll(enc)
		if err != nil {
			fmt.Printf("Error reading compressed response: %v\n", err)
			return body, len(body)
		}
		return ioData, len(ioData)
	}

	return body, len(body)
}

type ResponseServed struct {
	Status      int
	Body        string
	ContentType *string
	Headers     map[string]string
}

func ServeResponse(conn net.Conn, request HttpRequest, resp ResponseServed) {
	var encoding string
	clientEncodings := request.Headers["accept-encoding"]
	if clientEncodings != "" {
		// If client accepts any encoding, prefer "zstd, gzip, deflate" in that order
		if clientEncodings == "*" {
			clientEncodings = "zstd, gzip, deflate"
		}
		clientEncodings := strings.Split(clientEncodings, ",")
		for _, enc := range clientEncodings {
			enc = strings.TrimSpace(enc)
			if enc == "zstd" || enc == "gzip" || enc == "deflate" {
				encoding = enc
				break
			}
		}
	}

	if encoding == "" {
		// Fallback to no encoding if client does not support any
		encoding = "none"
	}
	isValidEncoding := encoding == "zstd" || encoding == "gzip" || encoding == "deflate"
	contentBody, contentLength := GetContentBody([]byte(resp.Body), encoding)

	if resp.ContentType == nil {
		defaultType := "text/html; charset=utf-8"
		resp.ContentType = &defaultType
	}

	// Build HTTP response
	if request.Version == "HTTP/1.1" || request.Version == "HTTP/1.0" {
		response := fmt.Sprintf("HTTP/1.1 %d\r\n", resp.Status)
		response += fmt.Sprintf("server: Iridium/%s\r\n", VERSION)
		response += "connection: keep-alive\r\n"
		response += fmt.Sprintf("content-length: %d\r\n", contentLength)
		response += fmt.Sprintf("content-type: %s\r\n", *resp.ContentType)
		response += "vary: Accept-Encoding\r\n"
		response += fmt.Sprintf("date: %s\r\n", time.Now().UTC().Format(http.TimeFormat))
		if isValidEncoding {
			response += fmt.Sprintf("content-encoding: %s\r\n", encoding)
		}
		if resp.Headers != nil {
			for k, v := range resp.Headers {
				k = strings.TrimSpace(strings.ToLower(k))
				if slices.Contains(ServerIgnoredHeaders, k) {
					continue
				}
				response += fmt.Sprintf("%s: %s\r\n", k, v)
			}
		}
		response += "\r\n"
		response += string(contentBody)

		// Write response to connection
		_, err := conn.Write([]byte(response))
		if err != nil {
			fmt.Printf("Error writing response: %v\n", err)
		}
		conn.Close()
	} else if request.Version == "HTTP/2.0" {
		responseHeaders := []hpack.HeaderField{
			{Name: ":status", Value: fmt.Sprintf("%d", resp.Status)},
			{Name: "server", Value: fmt.Sprintf("Iridium/%s", VERSION)},
			{Name: "content-length", Value: fmt.Sprintf("%d", contentLength)},
			{Name: "content-type", Value: *resp.ContentType},
			{Name: "vary", Value: "Accept-Encoding"},
			{Name: "date", Value: time.Now().UTC().Format(http.TimeFormat)},
		}
		if isValidEncoding {
			responseHeaders = append(responseHeaders, hpack.HeaderField{Name: "content-encoding", Value: encoding})
		}
		if resp.Headers != nil {
			for k, v := range resp.Headers {
				k = strings.TrimSpace(strings.ToLower(k))
				if slices.Contains(ServerIgnoredHeaders, k) {
					continue
				}
				responseHeaders = append(responseHeaders, hpack.HeaderField{Name: k, Value: v})
			}
		}
		var buf bytes.Buffer
		encoder := hpack.NewEncoder(&buf)
		for _, hf := range responseHeaders {
			encoder.WriteField(hf) // encodes each header into buf
		}

		if request.StreamID == nil {
			return
		}
		if err := http2.WriteFrame(conn, http2.HeadersFrameType, http2.EndHeadersFlag, *request.StreamID, buf.Bytes()); err != nil {
			return
		}
		if err := http2.WriteFrame(conn, http2.DataFrameType, http2.EndStreamFlag, *request.StreamID, contentBody); err != nil {
			return
		}
	}
}

func ServeError(conn net.Conn, request HttpRequest, status int) {
	body := ErrorHTML(status)
	ServeResponse(conn, request, ResponseServed{
		Status: status,
		Body:   body,
	})
}

func ErrorHTML(status int) string {
	var statusText string
	switch status {
	case 400:
		statusText = "Bad Request"
	case 403:
		statusText = "Forbidden"
	case 404:
		statusText = "Not Found"
	case 416:
		statusText = "Range Not Satisfiable"
	case 500:
		statusText = "Internal Server Error"
	case 502:
		statusText = "Bad Gateway"
	case 503:
		statusText = "Service Unavailable"
	case 504:
		statusText = "Gateway Timeout"
	default:
		statusText = "Unknown Error"
	}
	return fmt.Sprintf(
		"<!DOCTYPE html><html><head><title>%s</title></head><body><center><h1>%d %s</h1><hr><p>Iridium v%s</p></center></body></html>",
		statusText, status, statusText, VERSION,
	)
}
