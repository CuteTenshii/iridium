package main

import "strings"

// MinifyHTML performs a basic minification of HTML content by removing unnecessary whitespace, newlines, and tabs.
func MinifyHTML(html string) string {
	minified := strings.ReplaceAll(html, "\n", "")
	minified = strings.ReplaceAll(minified, "\t", "")
	minified = strings.ReplaceAll(minified, "  ", " ")
	minified = strings.ReplaceAll(minified, "> <", "><")
	minified = strings.ReplaceAll(minified, " />", "/>")
	return minified
}

func MinifyCSS(css string) string {
	minified := strings.ReplaceAll(css, "\n", "")
	minified = strings.ReplaceAll(minified, "\t", "")
	minified = strings.ReplaceAll(minified, "  ", " ")
	minified = strings.ReplaceAll(minified, " {", "{")
	minified = strings.ReplaceAll(minified, "{ ", "{")
	minified = strings.ReplaceAll(minified, " }", "}")
	minified = strings.ReplaceAll(minified, "} ", "}")
	minified = strings.ReplaceAll(minified, " ;", ";")
	minified = strings.ReplaceAll(minified, "; ", ";")
	minified = strings.ReplaceAll(minified, " :", ":")
	minified = strings.ReplaceAll(minified, ": ", ":")
	minified = strings.ReplaceAll(minified, ", ", ",")
	minified = strings.ReplaceAll(minified, ";}", "}")
	return minified
}

func MinifyJS(js string) string {
	minified := strings.ReplaceAll(js, "\n", "")
	minified = strings.ReplaceAll(minified, "\t", "")
	minified = strings.ReplaceAll(minified, "  ", " ")
	minified = strings.ReplaceAll(minified, " => ", "=>")
	minified = strings.ReplaceAll(minified, " {", "{")
	minified = strings.ReplaceAll(minified, "{ ", "{")
	minified = strings.ReplaceAll(minified, " }", "}")
	minified = strings.ReplaceAll(minified, "} ", "}")
	minified = strings.ReplaceAll(minified, "true", "!0")
	minified = strings.ReplaceAll(minified, "false", "!1")
	minified = strings.ReplaceAll(minified, " = ", "=")
	return minified
}
