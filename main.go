package main

import (
	"errors"
	"fmt"
	"log"
	"mime"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const VERSION = "1.0.0"

func main() {
	port := 8080
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		if errors.Is(err, net.ErrClosed) {
			// Handle the specific case where the network is closed
			println("Network is closed")
		} else {
			// Handle other types of errors
			println("Error occurred:", err.Error())
		}
	}

	hosts, err := LoadHosts()
	if err != nil {
		panic("Failed to load hosts:" + err.Error())
	}
	log.Printf("Loaded %d host(s)\n", len(hosts))
	defer listener.Close()
	println(fmt.Sprintf("Reverse proxy running on port %d", port))

	for {
		conn, err := listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				println("Listener has been closed")
				break
			}
			println("Error accepting connection:", err.Error())
			continue
		}
		request, err := ReadRequest(conn)
		if err != nil {
			ErrorLog(err)
			// Close the connection on error. Client will see a connection reset error.
			conn.Close()
			continue
		}
		remoteIp := conn.RemoteAddr().String()
		host := request.Headers["Host"]
		if host == "" {
			ServeResponse(conn, ResponseServed{
				Status: 400,
				Body:   "Bad Request: Missing Host header",
			})
			continue
		}
		matchedHost := FindHost(hosts, host)
		if matchedHost == nil {
			ServeResponse(conn, ResponseServed{Status: 200, Body: FallbackHtml()})
			continue
		}
		waf := MakeWAFChecks(request)
		if waf.Blocked {
			if waf.CloseConnection {
				conn.Close()
				continue
			}
			ServeError(conn, 403)
			continue
		}
		if matchedHost.Domain == host {
			for _, location := range matchedHost.Locations {
				if IsLocationMatching(location.Match, request.Path) {
					if location.Content != nil {
						ServeResponse(conn, ResponseServed{Status: 200, Body: *location.Content})
						break
					} else if location.Root != nil {
						stat, err := os.Stat(*location.Root)
						if err != nil || !stat.IsDir() {
							ServeResponse(conn, ResponseServed{
								Status: 500,
								Body:   "Internal Server Error: Invalid root directory",
							})
							break
						}
						unesc, err := url.QueryUnescape(request.Path[1:])
						if err != nil {
							ServeError(conn, 400)
							break
						}
						filePath := *location.Root + string(os.PathSeparator) + unesc
						stat, err = os.Stat(filePath)
						if err != nil || stat.IsDir() {
							ServeError(conn, 404)
							break
						}
						data, err := os.ReadFile(filePath)
						if err != nil {
							if errors.Is(err, os.ErrNotExist) {
								ServeError(conn, 404)
								break
							} else if errors.Is(err, os.ErrPermission) {
								ServeError(conn, 403)
								break
							} else if errors.Is(err, os.ErrInvalid) {
								ServeError(conn, 400)
								break
							} else {
								ServeResponse(conn, ResponseServed{
									Status: 500,
									Body:   "Internal Server Error: " + err.Error(),
								})
								break
							}
						}
						ext := filepath.Ext(filePath)
						mimeType := mime.TypeByExtension(ext)
						if mimeType == "" {
							mimeType = "application/octet-stream"
						}
						headers := make(map[string]string)
						if location.Headers != nil {
							for k, v := range *location.Headers {
								headers[k] = v
							}
						}
						if strings.HasPrefix(mimeType, "video/") || strings.HasPrefix(mimeType, "audio/") {
							headers["Accept-Ranges"] = "bytes"
						}
						if request.Headers["Range"] != "" {
							headers["Accept-Ranges"] = "bytes"
							counts := regexp.MustCompile(`bytes=(\d*)-(\d*)`).FindStringSubmatch(request.Headers["Range"])
							if len(counts) != 3 {
								ServeError(conn, 400)
								break
							}
							startStr, endStr := counts[1], counts[2]
							var start, end int
							if startStr == "" && endStr == "" {
								ServeError(conn, 400)
								break
							} else if startStr == "" {
								// suffix byte range: bytes=-N
								n, err := fmt.Sscanf(endStr, "%d", &end)
								if n != 1 || err != nil {
									ServeError(conn, 400)
									break
								}
								if end > len(data) {
									end = len(data)
								}
								start = len(data) - end
								end = len(data) - 1
							} else if endStr == "" {
								// open-ended byte range: bytes=N-
								n, err := fmt.Sscanf(startStr, "%d", &start)
								if n != 1 || err != nil || start >= len(data) || start < 0 {
									ServeError(conn, 400)
									break
								}
								end = len(data) - 1
							} else {
								// specific byte range: bytes=N-M
								n1, err1 := fmt.Sscanf(startStr, "%d", &start)
								n2, err2 := fmt.Sscanf(endStr, "%d", &end)
								if n1 != 1 || err1 != nil || n2 != 1 || err2 != nil || start < 0 || end < 0 || start >= len(data) || end >= len(data) || start > end {
									ServeError(conn, 400)
									break
								}
							}
							data = data[start : end+1]
							headers["Content-Range"] = fmt.Sprintf("bytes %d-%d/%d", start, end, stat.Size())
							ServeResponse(conn, ResponseServed{
								Status:      206,
								Body:        string(data),
								ContentType: &mimeType,
								Headers:     headers,
							})
							break
						}
						ServeResponse(conn, ResponseServed{
							Status:      200,
							Body:        string(data),
							ContentType: &mimeType,
							Headers:     headers,
						})
						break
					} else if location.Proxy != nil {
						proxyRequest := MakeProxyRequest(conn, request, *location.Proxy)
						if proxyRequest.Headers == nil {
							ServeError(conn, 500)
							break
						}
						// Successfully proxied the request, close the original connection
						conn.Close()
						break
					}
				} else {
					ServeError(conn, 404)
					break
				}
			}
		} else {
			conn.Close()
			continue
		}

		RequestLog(request.Method, request.Path, request.Version, remoteIp)
	}
}
