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
			ServeResponse(conn, ResponseServed{
				Status: 503,
				Body:   "No matching host configuration. In your config, ensure you have a host entry for '" + host + "'.",
			})
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
						ServeResponse(conn, ResponseServed{
							Status:      200,
							Body:        string(data),
							ContentType: &mimeType,
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
