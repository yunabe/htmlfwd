package main

import (
	"math/rand"
	"time"
)

func main() {
	rand.Seed(int64(time.Now().Nanosecond()))
	setting := readSetting()
	if setting == nil {
		return
	}
	server := NewWebServer(setting)
	go server.ListenAndServe()
	openClientServer(setting.commandPort, server)
}
