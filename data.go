package main

import (
	"os"
	"runtime"
)

func GetDataDirectory() string {
	if runtime.GOOS == "windows" {
		// Windows
		appData := os.Getenv("APPDATA")
		if appData != "" {
			return appData + "\\ReverseProxy"
		}
		return "."
	} else {
		// Linux and macOS
		home := os.Getenv("HOME")
		if home != "" {
			return home + "/.reverseproxy"
		}
		return "."
	}
}
