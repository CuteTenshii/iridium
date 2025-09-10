package main

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"iridium/cli"
	"mime"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const VERSION = "1.0.0"

func handleConnection(conn net.Conn, hosts []Host) {
	defer conn.Close()

	var request HttpRequest
	var err error
	if tlsConn, ok := conn.(*tls.Conn); ok {
		if err = tlsConn.Handshake(); err != nil {
			ErrorLog(err)
			return
		}
		state := tlsConn.ConnectionState()
		alpn := state.NegotiatedProtocol // "h2" for HTTP/2, "http/1.1" for HTTP/1.1
		request, err = ReadRequest(conn, alpn)
	} else {
		request, err = ReadRequest(conn, "")
	}

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
			if sitekey == "" || sitekey == "your-site-key" {
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
			// Data to be used in the captcha page to identify the request.
			data := make(map[string]string)
			data["ip"] = GetLocalIpWithoutPort(remoteIp)
			data["user_agent"] = request.Headers["user-agent"]
			data["host"] = host
			data["path"] = request.Path
			data["method"] = request.Method
			data["body"] = request.Body
			data["captcha_provider"] = provider
			jsonHeaders, _ := json.Marshal(request.Headers)
			data["headers"] = base64.StdEncoding.EncodeToString(jsonHeaders)
			page := GetCaptchaHTML(sitekey, provider, data)
			ServeResponse(conn, request, ResponseServed{Status: 403, Body: page})
		} else {
			ServeError(conn, request, 403)
		}
		return
	}
	clearanceMaxAge := 30 * time.Minute
	if waf.ModifiedRequest != nil {
		request.Headers = waf.ModifiedRequest.Headers
		request.Body = waf.ModifiedRequest.Body
		request.Method = waf.ModifiedRequest.Method
		request.Path = waf.ModifiedRequest.Path
	}
	RequestLog(request.Method, request.Path, request.Version, remoteIp)

	if matchedHost.Domain == host {
		for _, location := range matchedHost.Locations {
			if IsLocationMatching(location.Match, request.Path) {
				cacheDuration := matchedHost.EdgeCache.Duration
				isCacheable := matchedHost.EdgeCache.Enabled && IsEdgeCacheEligible(request.Path, matchedHost.EdgeCache.Extensions)

				if isCacheable {
					if data, found := GetFileFromEdgeCache(request.Path); found {
						mimeType := mime.TypeByExtension(filepath.Ext(request.Path))
						if mimeType == "" {
							mimeType = "application/octet-stream"
						}

						headers := PopulateHeaders(location.Headers, &data.Headers)
						lastModified := data.Headers["last-modified"]
						headers["x-cache"] = "HIT"
						headers["age"] = strconv.FormatFloat(time.Since(data.AddedAt).Seconds(), 'f', 0, 64)

						if waf.ClearanceToken != nil {
							headers["set-cookie"] = SetCookie("iridium_clearance", *waf.ClearanceToken, StrPtr("/"), nil, IntPtr(int(clearanceMaxAge.Seconds())), false, true)
						}

						ifModifiedSince := request.Headers["if-modified-since"]
						if ifModifiedSince != "" && ifModifiedSince == lastModified {
							headers["last-modified"] = lastModified
							ServeResponse(conn, request, ResponseServed{
								Status:  304,
								Body:    "",
								Headers: headers,
							})
							return
						}

						rangeHeader := request.Headers["range"]
						if strings.HasPrefix(mimeType, "video/") || strings.HasPrefix(mimeType, "audio/") {
							headers["accept-ranges"] = "bytes"
							if rangeHeader != "" {
								dataLength := int64(len(data.Data))
								start, end, err := GetRangeStartEnd(rangeHeader, dataLength)
								if err != nil {
									ErrorLog(err)
									ServeError(conn, request, 416)
									return
								}

								body := data.Data[start : end+1]
								headers["content-range"] = fmt.Sprintf("bytes %d-%d/%d", start, end, dataLength)
								ServeResponse(conn, request, ResponseServed{
									Status:      206,
									Body:        string(body),
									ContentType: &mimeType,
									Headers:     headers,
								})
								return
							}
						}

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
					body := *location.Content
					variables := map[string]string{
						"user_agent":  request.Headers["user-agent"],
						"remote_addr": GetLocalIpWithoutPort(remoteIp),
						"host":        host,
						"path":        request.Path,
						"method":      request.Method,
						"scheme":      "http",
					}
					for key, value := range variables {
						placeholder := fmt.Sprintf("$%s", key)
						body = strings.ReplaceAll(body, placeholder, value)
					}
					ServeResponse(conn, request, ResponseServed{Status: 200, Body: body})
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
						} else if errors.Is(err, os.ErrPermission) {
							ErrorLog(err)
							ServeError(conn, request, 403)
						} else if errors.Is(err, os.ErrInvalid) {
							ErrorLog(err)
							ServeError(conn, request, 400)
						} else {
							ErrorLog(err)
							ServeError(conn, request, 500)
						}
						return
					}

					lastModified := stat.ModTime().UTC().Format(HttpDateFormat)
					ext := filepath.Ext(filePath)
					mimeType := mime.TypeByExtension(ext)
					if mimeType == "" {
						mimeType = "application/octet-stream"
					}
					headers := PopulateHeaders(location.Headers)
					headers["last-modified"] = lastModified

					rangeHeader := request.Headers["range"]
					if strings.HasPrefix(mimeType, "video/") || strings.HasPrefix(mimeType, "audio/") {
						headers["accept-ranges"] = "bytes"
						if rangeHeader != "" {
							start, end, err := GetRangeStartEnd(rangeHeader, stat.Size())
							if err != nil {
								ErrorLog(err)
								ServeError(conn, request, 416)
								return
							}

							if isCacheable {
								headers["x-cache"] = "MISS"
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
							if waf.ClearanceToken != nil {
								headers["set-cookie"] = SetCookie("iridium_clearance", *waf.ClearanceToken, StrPtr("/"), nil, IntPtr(int(clearanceMaxAge.Seconds())), false, true)
							}
							ServeResponse(conn, request, ResponseServed{
								Status:      206,
								Body:        string(data),
								ContentType: &mimeType,
								Headers:     headers,
							})
							return
						}
					}

					if isCacheable {
						headers["x-cache"] = "MISS"
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

					if request.Headers["if-modified-since"] == lastModified {
						headers := PopulateHeaders(location.Headers)
						if waf.ClearanceToken != nil {
							headers["set-cookie"] = SetCookie("iridium_clearance", *waf.ClearanceToken, StrPtr("/"), nil, IntPtr(int(clearanceMaxAge.Seconds())), false, true)
						}
						headers["last-modified"] = lastModified
						ServeResponse(conn, request, ResponseServed{
							Status:  304,
							Body:    "",
							Headers: headers,
						})
						return
					}

					ServeResponse(conn, request, ResponseServed{
						Status:      200,
						Body:        string(data),
						ContentType: &mimeType,
						Headers:     headers,
					})
					return
				} else if location.Proxy != nil {
					response, err := MakeProxyRequest(conn, request, *location.Proxy)
					if err != nil {
						ErrorLog(err)
						return
					}
					if response.Headers == nil {
						ServeError(conn, request, 500)
						return
					}

					cacheControl := response.Headers["cache-control"]
					if cacheControl != "" {
						parts := strings.Split(cacheControl, ",")
						for _, part := range parts {
							part = strings.TrimSpace(part)
							if part == "no-store" || part == "no-cache" || part == "private" {
								isCacheable = false
								break
							} else if strings.HasPrefix(part, "max-age=") {
								ageStr := strings.TrimPrefix(part, "max-age=")
								age, err := strconv.Atoi(ageStr)
								if err == nil && age < cacheDuration {
									cacheDuration = age
								}
							}
						}
					}
					if isCacheable && response.Status == 200 {
						if _, found := GetFileFromEdgeCache(request.Path); !found {
							response.Headers["x-cache"] = "MISS"
							err = AddFileToEdgeCache(EdgeCacheFile{
								Data:     []byte(response.Body),
								Duration: time.Duration(cacheDuration) * time.Second,
								Path:     request.Path,
								Headers:  response.Headers,
							})
						}
					}

					contentType, _ := response.Headers["content-type"]
					if waf.ClearanceToken != nil {
						response.Headers["set-cookie"] = SetCookie("iridium_clearance", *waf.ClearanceToken, StrPtr("/"), nil, IntPtr(int(clearanceMaxAge.Seconds())), false, true)
					}
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

func StartListener() (net.Listener, error) {
	tlsCertFile := GetConfigValue("tls.cert_file", "").(string)
	tlsKeyFile := GetConfigValue("tls.key_file", "").(string)

	if tlsCertFile != "" && tlsKeyFile != "" {
		cert, err := tls.LoadX509KeyPair(tlsCertFile, tlsKeyFile)
		if err != nil {
			return nil, err
		}
		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
			NextProtos:   []string{"h2", "http/1.1"},
		}
		httpsListener, err := net.Listen("tcp", ":443")
		go StartHTTPSRedirector()
		if err != nil {
			return nil, err
		}
		tlsListener := tls.NewListener(httpsListener, tlsConfig)
		println("Iridium is running on port 443")
		return tlsListener, nil
	}

	listener, err := net.Listen("tcp", ":80")
	if err != nil {
		return nil, err
	}
	println("Iridium is running on port 80")
	return listener, nil
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
			fmt.Println("  cert generate <host>   Generate a self-signed TLS certificate for the specified host")
			fmt.Println("  cert obtain <host>     Obtain a TLS certificate from Let's Encrypt for the specified host")
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
		} else if os.Args[1] == "cert" {
			if len(os.Args) < 3 {
				println("Please specify 'generate' or 'obtain'. Example: iridium cert generate example.com\nRead more: https://iridiumproxy.github.io/tls/introduction/")
				return
			}
			if os.Args[2] == "generate" {
				_, _, err := cli.GenerateSelfSignedCert(os.Args[3])
				if err != nil {
					println("Failed to generate self-signed certificate:", err.Error())
					return
				}
				return
			} else if os.Args[2] == "obtain" {
				if len(os.Args) < 4 {
					println("Please specify a domain. Example: iridium cert obtain example.com")
					return
				}
				fmt.Println("Obtaining TLS certificate using Let's Encrypt...")
				_, _, err := cli.GenerateACMECert(os.Args[3])
				if err != nil {
					println("Failed to obtain TLS certificate:", err.Error())
					return
				}
			}
			return
		}
		println("Unknown argument:", os.Args[1])
		return
	}

	listener, err := StartListener()
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
	fmt.Printf("Loaded %d host(s)\n", len(hosts))
	defer listener.Close()

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
