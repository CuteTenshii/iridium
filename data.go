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
			return appData + "\\Iridium"
		}
		return "."
	} else {
		// Linux and macOS
		home := os.Getenv("HOME")
		if home != "" {
			return home + "/.iridium"
		}
		return "."
	}
}
