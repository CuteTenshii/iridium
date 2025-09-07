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
	"time"
)

const VERSION = "1.0.0"

func handleConnection(conn net.Conn, hosts []Host) {
	defer conn.Close()
	request, err := ReadRequest(conn)
	if err != nil {
		ErrorLog(err)
		return
	}
	remoteIp := conn.RemoteAddr().String()
	host := request.Headers["host"]
	if host == "" {
		ServeError(conn, request, 400)
		return
	}
	matchedHost := FindHost(hosts, host)
	if matchedHost == nil {
		ServeResponse(conn, request, ResponseServed{Status: 200, Body: FallbackHtml()})
		return
	}
	waf := MakeWAFChecks(request)
	if waf.Blocked {
		if waf.CloseConnection {
			return
		}

		serveCaptcha := GetConfigValue("waf.captcha.enabled", false).(bool)
		if serveCaptcha {
			sitekey := GetConfigValue("waf.captcha.site_key", "").(string)
			if sitekey == "" {
				ErrorLog(errors.New("captcha sitekey is not set in config"))
				ServeError(conn, request, 403)
				return
			}
			provider := GetConfigValue("waf.captcha.provider", "").(string)
			if provider == "" {
				ErrorLog(errors.New("captcha provider is not set in config"))
				ServeError(conn, request, 403)
				return
			}
			page := GetCaptchaHTML(sitekey, provider)
			ServeResponse(conn, request, ResponseServed{Status: 403, Body: page})
		} else {
			ServeError(conn, request, 403)
		}
		return
	}
	RequestLog(request.Method, request.Path, request.Version, remoteIp)

	if matchedHost.Domain == host {
		for _, location := range matchedHost.Locations {
			if IsLocationMatching(location.Match, request.Path) {
				isCacheable := matchedHost.EdgeCache.Enabled && IsEdgeCacheEligible(request.Path, matchedHost.EdgeCache.Extensions)

				if isCacheable {
					if data, found := GetFileFromEdgeCache(request.Path); found {
						mimeType := "application/octet-stream"
						ext := filepath.Ext(request.Path)
						if ext != "" {
							if mt := mime.TypeByExtension(ext); mt != "" {
								mimeType = mt
							}
						}

						headers := make(map[string]string)
						if location.Headers != nil {
							for k, v := range *location.Headers {
								k = strings.ToLower(k)
								headers[k] = v
							}
						}
						for k, v := range data.Headers {
							k = strings.ToLower(k)
							headers[k] = v
						}

						if strings.HasPrefix(mimeType, "video/") || strings.HasPrefix(mimeType, "audio/") {
							headers["accept-ranges"] = "bytes"
						}

						headers["x-cache"] = "HIT"
						headers["age"] = fmt.Sprintf("%d", int(time.Since(data.AddedAt).Seconds()))
						ServeResponse(conn, request, ResponseServed{
							Status:      200,
							Body:        string(data.Data),
							ContentType: &mimeType,
							Headers:     headers,
						})
						return
					}
				}

				if location.Content != nil {
					ServeResponse(conn, request, ResponseServed{Status: 200, Body: *location.Content})
					return
				} else if location.Root != nil {
					stat, err := os.Stat(*location.Root)
					if err != nil || !stat.IsDir() {
						if err != nil {
							ErrorLog(err)
						}
						ServeError(conn, request, 500)
						return
					}

					unesc, err := url.QueryUnescape(request.Path[1:])
					if err != nil {
						ServeError(conn, request, 400)
						return
					}
					filePath := *location.Root + string(os.PathSeparator) + unesc
					stat, err = os.Stat(filePath)
					if err != nil || stat.IsDir() {
						ServeError(conn, request, 404)
						return
					}

					data, err := os.ReadFile(filePath)
					if err != nil {
						if errors.Is(err, os.ErrNotExist) {
							ErrorLog(err)
							ServeError(conn, request, 404)
							return
						} else if errors.Is(err, os.ErrPermission) {
							ErrorLog(err)
							ServeError(conn, request, 403)
							return
						} else if errors.Is(err, os.ErrInvalid) {
							ErrorLog(err)
							ServeError(conn, request, 400)
							return
						} else {
							ErrorLog(err)
							ServeError(conn, request, 500)
							return
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
							k = strings.ToLower(k)
							headers[k] = v
						}
					}

					if strings.HasPrefix(mimeType, "video/") || strings.HasPrefix(mimeType, "audio/") {
						headers["accept-ranges"] = "bytes"
					}
					if request.Headers["range"] != "" {
						headers["accept-ranges"] = "bytes"
						counts := regexp.MustCompile(`bytes=(\d*)-(\d*)`).FindStringSubmatch(request.Headers["range"])
						if len(counts) != 3 {
							ServeError(conn, request, 400)
							return
						}
						startStr, endStr := counts[1], counts[2]
						var start, end int
						if startStr == "" && endStr == "" {
							ServeError(conn, request, 400)
							return
						} else if startStr == "" {
							n, err := fmt.Sscanf(endStr, "%d", &end)
							if n != 1 || err != nil {
								ServeError(conn, request, 400)
								return
							}
							if end > len(data) {
								end = len(data)
							}
							start = len(data) - end
							end = len(data) - 1
						} else if endStr == "" {
							n, err := fmt.Sscanf(startStr, "%d", &start)
							if n != 1 || err != nil || start >= len(data) || start < 0 {
								ServeError(conn, request, 400)
								return
							}
							end = len(data) - 1
						} else {
							n1, err1 := fmt.Sscanf(startStr, "%d", &start)
							n2, err2 := fmt.Sscanf(endStr, "%d", &end)
							if n1 != 1 || err1 != nil || n2 != 1 || err2 != nil || start < 0 || end < 0 || start >= len(data) || end >= len(data) || start > end {
								ServeError(conn, request, 400)
								return
							}
						}

						if isCacheable {
							headers["x-cache"] = "MISS"
							cacheDuration := matchedHost.EdgeCache.Duration
							err = AddFileToEdgeCache(EdgeCacheFile{
								Data:     data,
								Duration: time.Duration(cacheDuration) * time.Second,
								Path:     request.Path,
								Headers:  headers,
							})
							if err != nil {
								ErrorLog(err)
								ServeError(conn, request, 500)
								return
							}
						}

						data = data[start : end+1]
						headers["content-range"] = fmt.Sprintf("bytes %d-%d/%d", start, end, stat.Size())
						ServeResponse(conn, request, ResponseServed{
							Status:      206,
							Body:        string(data),
							ContentType: &mimeType,
							Headers:     headers,
						})
						return
					}

					if isCacheable {
						headers["x-cache"] = "MISS"
						cacheDuration := matchedHost.EdgeCache.Duration
						err = AddFileToEdgeCache(EdgeCacheFile{
							Data:     data,
							Duration: time.Duration(cacheDuration) * time.Second,
							Path:     request.Path,
							Headers:  headers,
						})
						if err != nil {
							ErrorLog(err)
							ServeError(conn, request, 500)
							return
						}
					}

					ServeResponse(conn, request, ResponseServed{
						Status:      200,
						Body:        string(data),
						ContentType: &mimeType,
						Headers:     headers,
					})
					return
				} else if location.Proxy != nil {
					response := MakeProxyRequest(conn, request, *location.Proxy)
					if response.Headers == nil {
						ServeError(conn, request, 500)
						return
					}

					if isCacheable && response.Status == 200 {
						if _, found := GetFileFromEdgeCache(request.Path); !found {
							response.Headers["x-cache"] = "MISS"
							cacheDuration := matchedHost.EdgeCache.Duration
							err = AddFileToEdgeCache(EdgeCacheFile{
								Data:     []byte(response.Body),
								Duration: time.Duration(cacheDuration) * time.Second,
								Path:     request.Path,
								Headers:  response.Headers,
							})
						}
					}

					contentType, _ := response.Headers["content-type"]
					ServeResponse(conn, request, ResponseServed{
						Status:      response.Status,
						Body:        response.Body,
						ContentType: &contentType,
						Headers:     response.Headers,
					})
					return
				}
			} else {
				ServeError(conn, request, 404)
				return
			}
		}
	} else {
		return
	}
}

func main() {
	if len(os.Args) > 1 {
		if os.Args[1] == "--version" || os.Args[1] == "-v" {
			fmt.Printf("Iridium version %s\n", VERSION)
			return
		} else if os.Args[1] == "--help" || os.Args[1] == "-h" {
			fmt.Println("Usage: iridium [options]")
			fmt.Println("")
			fmt.Println("Options:")
			fmt.Println("  --version, -v    Show version information")
			fmt.Println("  --help, -h       Show this help message")
			fmt.Println("  validate         Validate the configuration file")
			return
		} else if os.Args[1] == "validate" {
			println("Validating configuration...")
			configPath := GetConfigPath()
			if _, err := os.Stat(configPath); errors.Is(err, os.ErrNotExist) {
				println("Configuration file does not exist. Did you run Iridium at least once?")
				return
			}

			// TODO

			return
		}
	}

	port := 8080
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		if errors.Is(err, net.ErrClosed) {
			println("Network is closed")
		} else {
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
		go handleConnection(conn, hosts)
	}
}
