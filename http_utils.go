package main

import (
	"fmt"
	"io"
	"net"
	"slices"
	"strings"
	"time"
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
func GetContentBody(body []byte, encoding string) (string, int) {
	if encoding == "zstd" || encoding == "gzip" || encoding == "deflate" {
		enc, err := CompressData(strings.NewReader(string(body)), encoding)
		if err != nil {
			fmt.Printf("Error compressing response: %v\n", err)
			return string(body), len(body)
		}
		ioData, err := io.ReadAll(enc)
		if err != nil {
			fmt.Printf("Error reading compressed response: %v\n", err)
			return string(body), len(body)
		}
		return string(ioData), len(ioData)
	}

	return string(body), len(body)
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
	conn.Write([]byte(fmt.Sprintf("HTTP/1.1 %d\r\n", resp.Status)))
	conn.Write([]byte(fmt.Sprintf("server: Iridium/%s\r\n", VERSION)))
	conn.Write([]byte("connection: keep-alive\r\n"))
	conn.Write([]byte(fmt.Sprintf("content-length: %d\r\n", contentLength)))
	conn.Write([]byte(fmt.Sprintf("content-type: %s\r\n", *resp.ContentType)))
	conn.Write([]byte("vary: Accept-Encoding\r\n"))
	conn.Write([]byte(fmt.Sprintf("date: %s\r\n", time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT"))))
	if isValidEncoding {
		conn.Write([]byte(fmt.Sprintf("content-encoding: %s\r\n", encoding)))
	}
	if resp.Headers != nil {
		for k, v := range resp.Headers {
			k = strings.TrimSpace(strings.ToLower(k))
			if slices.Contains(ServerIgnoredHeaders, k) {
				continue
			}
			conn.Write([]byte(fmt.Sprintf("%s: %s\r\n", k, v)))
		}
	}
	conn.Write([]byte("\r\n"))
	conn.Write([]byte(contentBody))
	conn.Close()
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
