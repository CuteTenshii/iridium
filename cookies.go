package main

import (
	"fmt"
	"strings"
)

func ParseCookies(cookieHeader string) map[string]string {
	cookies := make(map[string]string)
	pairs := strings.Split(cookieHeader, ";")
	for _, pair := range pairs {
		parts := strings.SplitN(strings.TrimSpace(pair), "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			cookies[key] = value
		}
	}
	return cookies
}

func SetCookie(name, value string, path, domain *string, maxAge *int, secure, httpOnly bool) string {
	cookie := fmt.Sprintf("%s=%s", name, value)
	if path != nil {
		cookie += fmt.Sprintf("; Path=%s", *path)
	}
	if domain != nil {
		cookie += fmt.Sprintf("; Domain=%s", *domain)
	}
	if maxAge != nil {
		cookie += fmt.Sprintf("; Max-Age=%d", *maxAge)
	}
	if secure {
		cookie += "; Secure"
	}
	if httpOnly {
		cookie += "; HttpOnly"
	}
	return cookie
}
