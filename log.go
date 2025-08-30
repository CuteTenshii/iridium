package main

import (
	"fmt"
	"os"
	"time"
)

var AccessLogFile = GetDataDirectory() + string(os.PathSeparator) + "logs" + string(os.PathSeparator) + "access.log"
var ErrorLogFile = GetDataDirectory() + string(os.PathSeparator) + "logs" + string(os.PathSeparator) + "error.log"
var WafLogFile = GetDataDirectory() + string(os.PathSeparator) + "logs" + string(os.PathSeparator) + "waf.log"

func RequestLog(method, url, protocol, host string) {
	line := method + " " + url + " " + protocol + " - Host: " + host
	AppendLog("INFO", line)
}

func ErrorLog(err error) {
	line := "Error: " + err.Error()
	AppendLog("error", line)
}

func AppendLog(logType, entry string) {
	now := time.Now().Format("2006/01/02 15:04:05")
	entry = fmt.Sprintf("[%s] %s", now, entry)
	println(entry)
	file := AccessLogFile

	if logType == "error" {
		file = ErrorLogFile
	} else if logType == "waf" {
		file = WafLogFile
	}
	// Ensure the log directory exists
	logDir := GetDataDirectory() + string(os.PathSeparator) + "logs"
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		err := os.MkdirAll(logDir, 0755)
		if err != nil {
			println("Failed to create log directory:", err.Error())
			return
		}
	}

	f, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		println("Failed to open log file:", err.Error())
		return
	}
	defer f.Close()

	if _, err := f.WriteString(entry + "\n"); err != nil {
		println("Failed to write to log file:", err.Error())
	}
}
