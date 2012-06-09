package main

import (
	"math/rand"
	"time"
)

func main() {
	rand.Seed(int64(time.Now().Nanosecond()))
	setting := readSetting()
	server := NewWebServer(setting.browserPort)
	go server.ListenAndServe()
	openClientServer(setting.commandPort, server)
}
