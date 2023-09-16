package main

import "time"

func handleCommand(msg string) (newMsg string, cmd string) {
	cmd = ""
	newMsg = msg
	if msg == "/utc" {
		newMsg = "UTC time is " + time.Now().UTC().Format("3:04 PM")
		cmd = "/utc"
	}
	return newMsg, cmd
}
