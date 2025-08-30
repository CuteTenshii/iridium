package main

import "os"

var LOG_FILE = "proxy.log"

func RequestLog(method, url, protocol, host string) {
	line := method + " " + url + " " + protocol + " - Host: " + host
	AppendLog("INFO", line)
}

func ErrorLog(err error) {
	line := "Error: " + err.Error()
	AppendLog("ERROR", line)
}

func AppendLog(logType, entry string) {
	entry = "[" + logType + "] " + entry
	println(entry)

	f, err := os.OpenFile(LOG_FILE, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		println("Failed to open log file:", err.Error())
		return
	}
	defer f.Close()

	if _, err := f.WriteString(entry + "\n"); err != nil {
		println("Failed to write to log file:", err.Error())
	}
}
