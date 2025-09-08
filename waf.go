package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// Common User-Agent regex patterns.
var (
	// Libraries and tools
	BunUserAgent            = regexp.MustCompile(`^Bun/\d+.\d+.\d+`)
	InsomniaUserAgent       = regexp.MustCompile(`^Insomnia/\d+.\d+.\d+`)
	PostmanUserAgent        = regexp.MustCompile(`^PostmanRuntime/\d+.\d+.\d+`)
	GoHttpClientUserAgent   = regexp.MustCompile(`^Go-http-client/\d+.\d+.\d+`)
	CurlUserAgent           = regexp.MustCompile(`^curl/\d+.\d+.\d+`)
	WgetUserAgent           = regexp.MustCompile(`^Wget/\d+.\d+.\d+`)
	AxiosUserAgent          = regexp.MustCompile(`^axios/\d+.\d+.\d+`)
	HttpxUserAgent          = regexp.MustCompile(`^httpx/\d+.\d+.\d+`)
	PythonRequestsUserAgent = regexp.MustCompile(`^python-requests/\d+\.\d+(\.\d+)?`)
	JavaUserAgent           = regexp.MustCompile(`^Java/\d+\.\d+`)
	PhpUserAgent            = regexp.MustCompile(`^PHP/\d+\.\d+(\.\d+)?`)
	PerlUserAgent           = regexp.MustCompile(`^libwww-perl/\d+\.\d+`)

	// Crawlers and bots
	GooglebotUserAgent   = regexp.MustCompile(`Googlebot/\d+\.\d+ \(\+http://www.google.com/bot.html\)`)
	BingbotUserAgent     = regexp.MustCompile(`bingbot/\d+\.\d+`)
	BaiduspiderUserAgent = regexp.MustCompile(`Baiduspider/\d+\.\d+`)
	YandexUserAgent      = regexp.MustCompile(`YandexBot/\d+\.\d+`)
	DuckduckbotUserAgent = regexp.MustCompile(`DuckDuckBot/\d+\.\d+`)
	FacebookbotUserAgent = regexp.MustCompile(`facebookexternalhit/\d+\.\d+`)
	TwitterbotUserAgent  = regexp.MustCompile(`Twitterbot/\d+\.\d+`)
)

type WAFResult struct {
	// Whether the request was blocked by WAF rules.
	Blocked bool
	// If blocked, the reason for blocking. Only used for internal logging, not sent to clients.
	Reason *string
	// Whether to close the connection immediately without sending a response. Clients will see a connection reset.
	CloseConnection bool
	// If the request was modified (e.g. after passing CAPTCHA), this will contain the new request to process.
	ModifiedRequest *HttpRequest
	// If the request was allowed after passing CAPTCHA, this will contain the new iridium_clearance token to set in a cookie.
	ClearanceToken *string
}

type WAFBody struct {
	Method          string `json:"method"`
	Path            string `json:"path"`
	Headers         string `json:"headers"`
	Body            string `json:"body"`
	CaptchaProvider string `json:"captcha_provider"`
	IP              string `json:"ip"`
	UserAgent       string `json:"user_agent"`
}

// MakeWAFChecks applies WAF rules to the incoming HTTP request based on configuration settings.
// Returns the WAFResult indicating if the request is blocked and the reason.
func MakeWAFChecks(request HttpRequest) WAFResult {
	cookies := ParseCookies(request.Headers["cookie"])
	if val, ok := cookies["iridium_clearance"]; ok {
		// Validate the token
		tokenMap, err := DecompressWAFData(val)
		if err == nil && tokenMap.UserAgent == request.Headers["user-agent"] && tokenMap.IP == request.Headers["x-forwarded-for"] {
			println("WAF: Valid clearance token, allowing request")
			// Valid token, allow the request
			return WAFResult{Blocked: false}
		}
		// Invalid token, proceed with further checks
	}

	// Check if this is a CAPTCHA response submission
	// Expecting POST with form data containing "xxx-xxxxxxxxx-response" and "data" fields
	// Content-Type should be "application/x-www-form-urlencoded"
	if request.Method == "POST" && strings.Contains(request.Body, "response=") && strings.Contains(request.Body, "data=") &&
		request.Headers["content-type"] == "application/x-www-form-urlencoded" {
		parsed, err := url.ParseQuery(request.Body)
		if err != nil {
			AppendLog("waf", fmt.Sprintf("Error parsing CAPTCHA request body: %v\n", err))
			// Just continue processing the request if parsing fails, it's likely not a WAF request
		} else {
			wafBody, err := DecompressWAFData(parsed.Get("data"))
			if err != nil {
				AppendLog("waf", fmt.Sprintf("Error decompressing CAPTCHA request data: %v\n", err))
				// Just continue processing the request if decompression fails, it's likely not a WAF request
			} else {
				if request.Headers["user-agent"] != wafBody.UserAgent || request.Path != wafBody.Path {
					AppendLog("waf", fmt.Sprintf("CAPTCHA request data does not match original request from IP %s\n", wafBody.IP))
					return WAFResult{Blocked: true, Reason: StrPtr("captcha data mismatch")}
				}

				// Validate the CAPTCHA response
				var captchaResponse string
				if wafBody.CaptchaProvider == "hcaptcha" {
					captchaResponse = parsed.Get("h-captcha-response")
				} else if wafBody.CaptchaProvider == "recaptcha" {
					captchaResponse = parsed.Get("g-recaptcha-response")
				} else if wafBody.CaptchaProvider == "turnstile" {
					captchaResponse = parsed.Get("cf-turnstile-response")
				} else {
					AppendLog("waf", fmt.Sprintf("Unsupported CAPTCHA provider in request from IP %s: %s\n", wafBody.IP, wafBody.CaptchaProvider))
					return WAFResult{Blocked: true, Reason: StrPtr("unsupported captcha provider")}
				}
				secretKey := GetConfigValue("waf.captcha.secret_key", "").(string)
				if secretKey == "" || secretKey == "your-secret-key" {
					ErrorLog(errors.New("captcha secret key is not configured"))
					return WAFResult{Blocked: true, Reason: StrPtr("captcha not configured")}
				}

				isCaptchaValid := CheckCaptchaSolution(captchaResponse, wafBody.CaptchaProvider, secretKey)
				if !isCaptchaValid {
					AppendLog("waf", fmt.Sprintf("Invalid CAPTCHA solution from IP %s using %s\n", wafBody.IP, wafBody.CaptchaProvider))
					return WAFResult{Blocked: true, Reason: StrPtr("invalid captcha")}
				}
				AppendLog("waf", fmt.Sprintf("Successful CAPTCHA solution from IP %s using %s\n", wafBody.IP, wafBody.CaptchaProvider))
				modifiedRequest := request
				modifiedRequest.Body = wafBody.Body
				modifiedRequest.Method = wafBody.Method
				modifiedRequest.Path = wafBody.Path
				modifiedRequest.Headers = make(map[string]string)
				headers, err := base64.StdEncoding.DecodeString(wafBody.Headers)
				if err != nil {
					AppendLog("waf", fmt.Sprintf("Error decoding CAPTCHA request headers from IP %s: %v\n", wafBody.IP, err))
					return WAFResult{Blocked: true, Reason: StrPtr("captcha headers error")}
				}

				err = json.Unmarshal(headers, &modifiedRequest.Headers)
				if err != nil {
					AppendLog("waf", fmt.Sprintf("Error unmarshaling CAPTCHA request headers from IP %s: %v\n", wafBody.IP, err))
					return WAFResult{Blocked: true, Reason: StrPtr("captcha headers error")}
				}
				AppendLog("waf", fmt.Sprintf("Passed CAPTCHA, allowing request from IP %s\n", wafBody.IP))

				return WAFResult{Blocked: false, ModifiedRequest: &modifiedRequest, ClearanceToken: StrPtr(CreateWAFSuccessToken(modifiedRequest))}
			}
		}
	}

	return WAFResult{Blocked: true}
	enabled := GetConfigValue("waf.enabled", false).(bool)
	if !enabled {
		return WAFResult{Blocked: false}
	}

	blockLibraries := GetConfigValue("waf.block_libraries", true).(bool)
	blockCrawlers := GetConfigValue("waf.block_crawlers", true).(bool)
	blockEmptyUA := GetConfigValue("waf.block_empty_ua", true).(bool)
	ua := request.Headers["user-agent"]

	if blockEmptyUA && strings.TrimSpace(ua) == "" {
		AppendLog("waf", "Blocked request with empty User-Agent")
		return WAFResult{Blocked: true, Reason: StrPtr("empty User-Agent"), CloseConnection: true}
	}
	if blockLibraries && ua != "" {
		if BunUserAgent.MatchString(ua) || InsomniaUserAgent.MatchString(ua) || PostmanUserAgent.MatchString(ua) ||
			GoHttpClientUserAgent.MatchString(ua) || CurlUserAgent.MatchString(ua) || WgetUserAgent.MatchString(ua) ||
			AxiosUserAgent.MatchString(ua) || HttpxUserAgent.MatchString(ua) || PythonRequestsUserAgent.MatchString(ua) ||
			JavaUserAgent.MatchString(ua) || PhpUserAgent.MatchString(ua) || PerlUserAgent.MatchString(ua) {
			AppendLog("waf", fmt.Sprintf("Blocked request with library/tool User-Agent: %s\n", ua))
			return WAFResult{Blocked: true, Reason: StrPtr("library/tool User-Agent"), CloseConnection: true}
		}
	}
	if blockCrawlers && ua != "" {
		if GooglebotUserAgent.MatchString(ua) || BingbotUserAgent.MatchString(ua) || BaiduspiderUserAgent.MatchString(ua) ||
			YandexUserAgent.MatchString(ua) || DuckduckbotUserAgent.MatchString(ua) || FacebookbotUserAgent.MatchString(ua) ||
			TwitterbotUserAgent.MatchString(ua) {
			AppendLog("waf", fmt.Sprintf("Blocked request with crawler/bot User-Agent: %s\n", ua))
			return WAFResult{Blocked: true, Reason: StrPtr("crawler/bot User-Agent"), CloseConnection: true}
		}
	}

	return WAFResult{Blocked: false}
}

func GetCaptchaHTML(siteKey string, provider string, data interface{}) string {
	var scriptUrl string
	var captchaHtml string
	if provider == "hcaptcha" {
		scriptUrl = "https://hcaptcha.com/1/api.js"
		captchaHtml = `<div class="h-captcha" data-sitekey="` + siteKey + `" data-callback="onSubmit" data-theme="dark"></div>`
	} else if provider == "recaptcha" {
		scriptUrl = "https://www.google.com/recaptcha/api.js"
		captchaHtml = `<div class="g-recaptcha" data-sitekey="` + siteKey + `" data-callback="onSubmit"></div>`
	} else if provider == "turnstile" {
		scriptUrl = "https://challenges.cloudflare.com/turnstile/v0/api.js"
		captchaHtml = `<div class="cf-turnstile" data-sitekey="` + siteKey + `" data-callback="onSubmit" data-theme="dark"></div>`
	}
	stringifiedData, _ := json.Marshal(data)
	encryptionKey := GetConfigValue("waf.encryption_key", generateWAFEncryptionKey()).(string)
	encrypted, err := encryptAESGCM(stringifiedData, []byte(encryptionKey))
	if err != nil {
		ErrorLog(fmt.Errorf("error encrypting WAF data: %v", err))
		return ErrorHTML(500)
	}
	encodedData := base64.StdEncoding.EncodeToString(encrypted)
	html := MinifyHTML(`<!DOCTYPE html>
<html lang="en">
  <head>
	<meta charset="UTF-8" />
	<meta name="viewport" content="width=device-width, initial-scale=1.0" />
	<title>Captcha Verification</title>
	<script src="` + scriptUrl + `" async defer></script>
	<style>` + MinifyCSS(`
	  body {
		font-family: Segoe UI, system-ui, -apple-system, BlinkMacSystemFont, Roboto, Helvetica Neue, Arial, sans-serif;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		height: 100vh;
		margin: 0;
		background: #1b2123;
		color: #ffffff;
	  }
	  .container {
		text-align: center;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
	  }
	  h1 {
		margin-bottom: 20px;
	  }`) + `
	</style>
  </head>
  <body>
	<div class="container">
	  <h1>Please complete the CAPTCHA to continue to the site</h1>
	  <form method="POST">` + captchaHtml + `<input type="hidden" name="data" value="` + encodedData + `" /></form>
	</div>
<div>
<p>Security & protection by <a href="https://github.com/IridiumProxy/iridium" target="_blank" style="color: #4ea1f3;">Iridium</a></p>
</div>
<script>` + MinifyJS(`
window.onSubmit = (token) => {
  const form = document.querySelector('form');
  form.submit();
  form.innerHTML = 'Waiting for '+window.location.hostname+' to respond...';
}`) + `
</script>
</body>
</html>`)

	return html
}

func CheckCaptchaSolution(captchaResponse string, provider string, secretKey string) bool {
	client := &http.Client{}
	if provider == "hcaptcha" {
		req, err := http.NewRequest("POST", "https://hcaptcha.com/siteverify", strings.NewReader(fmt.Sprintf("response=%s&secret=%s", captchaResponse, secretKey)))
		if err != nil {
			AppendLog("waf", fmt.Sprintf("Error creating hCaptcha verification request: %v\n", err))
			return false
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, err := client.Do(req)
		if err != nil {
			AppendLog("waf", fmt.Sprintf("Error sending hCaptcha verification request: %v\n", err))
			return false
		}
		defer resp.Body.Close()
		var result struct {
			Success bool `json:"success"`
		}
		err = json.NewDecoder(resp.Body).Decode(&result)
		if err != nil {
			AppendLog("waf", fmt.Sprintf("Error decoding hCaptcha verification response: %v\n", err))
			return false
		}
		return result.Success
	} else if provider == "recaptcha" {
		req, err := http.NewRequest("POST", "https://www.google.com/recaptcha/api/siteverify", strings.NewReader(fmt.Sprintf("response=%s&secret=%s", captchaResponse, secretKey)))
		if err != nil {
			AppendLog("waf", fmt.Sprintf("Error creating reCAPTCHA verification request: %v\n", err))
			return false
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, err := client.Do(req)
		if err != nil {
			AppendLog("waf", fmt.Sprintf("Error sending reCAPTCHA verification request: %v\n", err))
			return false
		}
		defer resp.Body.Close()
		var result struct {
			Success bool `json:"success"`
		}
		err = json.NewDecoder(resp.Body).Decode(&result)
		if err != nil {
			AppendLog("waf", fmt.Sprintf("Error decoding reCAPTCHA verification response: %v\n", err))
			return false
		}
		return result.Success
	} else if provider == "turnstile" {
		req, err := http.NewRequest("POST", "https://challenges.cloudflare.com/turnstile/v0/siteverify", strings.NewReader(fmt.Sprintf("response=%s&secret=%s", captchaResponse, secretKey)))
		if err != nil {
			AppendLog("waf", fmt.Sprintf("Error creating Turnstile verification request: %v\n", err))
			return false
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, err := client.Do(req)
		if err != nil {
			AppendLog("waf", fmt.Sprintf("Error sending Turnstile verification request: %v\n", err))
			return false
		}
		defer resp.Body.Close()
		var result struct {
			Success bool `json:"success"`
		}
		err = json.NewDecoder(resp.Body).Decode(&result)
		if err != nil {
			AppendLog("waf", fmt.Sprintf("Error decoding Turnstile verification response: %v\n", err))
			return false
		}
		return result.Success
	}

	return false
}

func CreateWAFSuccessToken(request HttpRequest) string {
	data := make(map[string]interface{})
	data["user_agent"] = request.Headers["user-agent"]
	data["ip"] = request.Headers["x-forwarded-for"]
	data["accept_language"] = request.Headers["accept-language"]
	data["accept_encoding"] = request.Headers["accept-encoding"]
	return CompressWAFData(data)
}

// CompressWAFData compresses and encodes the given data: JSON stringify -> base64 encode -> AES-GCM encrypt -> base64 encode
func CompressWAFData(data interface{}) string {
	stringifiedData, _ := json.Marshal(data)
	encryptionKey := GetConfigValue("waf.encryption_key", generateWAFEncryptionKey()).(string)
	encoded, err := encryptAESGCM(stringifiedData, []byte(encryptionKey))
	if err != nil {
		ErrorLog(fmt.Errorf("error encrypting WAF data: %v", err))
		return ""
	}
	return base64.StdEncoding.EncodeToString(encoded)
}

// DecompressWAFData reverses the compression and encoding done by CompressWAFData.
func DecompressWAFData(compressedStr string) (*WAFBody, error) {
	if compressedStr == "" {
		return nil, errors.New("empty compressed string")
	}
	data, err := base64.StdEncoding.DecodeString(compressedStr)
	if err != nil {
		return nil, err
	}
	encryptionKey := GetConfigValue("waf.encryption_key", generateWAFEncryptionKey()).(string)
	decoded, err := decryptAESGCM(data, []byte(encryptionKey))
	if err != nil {
		return nil, err
	}
	var result WAFBody
	err = json.Unmarshal(decoded, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}
