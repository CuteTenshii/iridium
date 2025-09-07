package main

import (
	"fmt"
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
}

// MakeWAFChecks applies WAF rules to the incoming HTTP request based on configuration settings.
// Returns the WAFResult indicating if the request is blocked and the reason.
func MakeWAFChecks(request HttpRequest) WAFResult {
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

func GetCaptchaHTML(siteKey string, provider string) string {
	var scriptUrl string
	var captchaHtml string
	if provider == "hcaptcha" {
		scriptUrl = "https://hcaptcha.com/1/api.js"
		captchaHtml = `<div class="h-captcha" data-sitekey="` + siteKey + `" data-callback="onSubmit" data-theme="dark"></div>`
	} else if provider == "recaptcha" {
		scriptUrl = "https://www.google.com/recaptcha/api.js"
		captchaHtml = `<div class="g-recaptcha" data-sitekey="` + siteKey + `"></div>`
	} else if provider == "turnstile" {
		scriptUrl = "https://challenges.cloudflare.com/turnstile/v0/api.js"
		captchaHtml = `<div class="cf-turnstile" data-sitekey="` + siteKey + `" data-callback="onSubmit" data-theme="dark"></div>`
	}
	html := `<!DOCTYPE html>
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
	  <form method="POST">` + captchaHtml + `</form>
</div>
<div>
<p>Security & protection by <a href="https://github.com/IridiumProxy/iridium" target="_blank" style="color: #4ea1f3;">Iridium</a></p>
</div>
<script>` + MinifyJS(`
const onSubmit = (token) => {
  const form = document.querySelector('form');
  form.submit();
  form.innerHTML = 'Waiting for'+window.location.hostname+' to respond...';
};`) + `
</script>
</body>
</html>`

	html = MinifyHTML(html)
	html = MinifyCSS(html)

	return html
}
