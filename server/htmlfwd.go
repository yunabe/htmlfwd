package main

import (
	"math/rand"
	"time"
)

func main() {
	rand.Seed(int64(time.Now().Nanosecond()))
	server := NewWebServer()
	go server.ListenAndServe()
	openClientServer(server)
}
