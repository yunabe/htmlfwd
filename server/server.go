package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strconv"
	"sync"
	"time"

	"code.google.com/p/go.net/websocket"
)

const keep_alive_interval = 60

type ClientReq struct {
	Host         string
	OpenUrl      string
	Notification string
}

type BrowserAction struct {
	Id           uint32
	OpenUrl      string
	CloseTabs    bool
	Notification string
	KeepAlive    bool
}

func (ba *BrowserAction) String() string {
	return fmt.Sprintf("[Id: %d, OpenUrl: %s, CloseTabs: %t, Notification: %s]",
		ba.Id, ba.OpenUrl, ba.CloseTabs, ba.Notification)
}

type WebServer struct {
	port           int32
	mux            *http.ServeMux
	fowardMap      map[uint32]*httputil.ReverseProxy
	fowardMapMutex sync.Mutex
	baChans        map[chan *BrowserAction]bool
	baChansMutex   sync.Mutex
}

func NewWebServer(port int32) *WebServer {
	mux := http.NewServeMux()
	server := WebServer{
		port:      port,
		fowardMap: make(map[uint32]*httputil.ReverseProxy),
		mux:       mux,
		baChans:   make(map[chan *BrowserAction]bool),
	}
	mux.Handle("/ws", websocket.Handler(
		func(ws *websocket.Conn) {
			// This looks very redandant :{
			server.handleWebSocket(ws)
		}))
	mux.HandleFunc("/fwd/", func(w http.ResponseWriter, req *http.Request) {
		server.handleForward(w, req)
	})
	return &server
}

func detectWebSocketClose(ws *websocket.Conn, ch chan bool) {
	var msg [1024]byte
	for {
		n, err := ws.Read(msg[:])
		if err != nil {
			if err == io.EOF {
				log.Println("Reached to WebSocket EOF")
			} else {
				log.Println("WebSocket read error:", err)
			}
			break
		}
		if n == 0 {
			break
		}
	}
	close(ch)
}

func createWebSocketCloseChannel(ws *websocket.Conn) chan bool {
	ch := make(chan bool)
	go detectWebSocketClose(ws, ch)
	return ch
}

func (server *WebServer) handleWebSocket(ws *websocket.Conn) {
	log.Println("WebSocket connection is established.")
	defer ws.Close()
	wsEncoder := json.NewEncoder(ws)

	bachan := make(chan *BrowserAction, 100)
	server.baChansMutex.Lock()
	server.baChans[bachan] = true
	server.baChansMutex.Unlock()

	wsCloseChan := createWebSocketCloseChannel(ws)

Loop:
	for {
		timer := time.After(keep_alive_interval * 1000 * 1000 * 1000)
		select {
		case ba, ok := <-bachan:
			if !ok {
				panic("bachan is closed unexpectedly.")
				break Loop
			}
			log.Println("Writing a browser action to websocket:", ba)
			wsEncoder.Encode(ba)
		case _, ok := <-wsCloseChan:
			if !ok {
				log.Println("WebSocket is closed by peer.")
				server.baChansMutex.Lock()
				close(bachan)
				for {
					if _, ok := <-bachan; ok {
						log.Println("BrowserAction is discarded.")
					} else {
						break
					}
				}
				delete(server.baChans, bachan)
				server.baChansMutex.Unlock()
				break Loop
			} else {
				panic("wsCloseChan had data")
			}
		case <-timer:
			// TODO: timer channel is deleted by GC correctly?
			log.Println("Sending keep-alive traffic.")
			wsEncoder.Encode(&BrowserAction{KeepAlive: true})
		}
	}
}

func (server *WebServer) handleForward(w http.ResponseWriter, req *http.Request) {
	server.fowardMapMutex.Lock()
	defer server.fowardMapMutex.Unlock()
	log.Println("r.URL =", req.URL)
	pattern, _ := regexp.Compile("^/fwd/(\\d+)(/.*)$")
	var matches []string = pattern.FindStringSubmatch(req.URL.String())
	log.Println(matches)
	id, _ := strconv.ParseUint(matches[1], 10, 32)
	proxy, ok := server.fowardMap[uint32(id)]
	if !ok {
		log.Println("....")
		return
	}
	req.URL, _ = url.Parse(matches[2])
	log.Println("proxy.ServeHTTP(w, req)")
	proxy.ServeHTTP(w, req)
}

func (server *WebServer) ListenAndServe() {
	log.Println("Listening to websockets on", server.port)
	err := http.ListenAndServe(fmt.Sprintf(":%d", server.port), server.mux)
	if err != nil {
		log.Println("Failed to listen:", err)
	}
}

func (server *WebServer) RegisterProxy(host string) uint32 {
	server.fowardMapMutex.Lock()
	defer server.fowardMapMutex.Unlock()
	id := rand.Uint32()
	log.Println("Register:", id, host)
	target := url.URL{
		Scheme: "http",
		Host:   host,
	}
	server.fowardMap[id] = httputil.NewSingleHostReverseProxy(&target)
	return id
}

func (server *WebServer) UnregisterProxy(id uint32) {
	server.fowardMapMutex.Lock()
	defer server.fowardMapMutex.Unlock()
	log.Println("Unregister: ", id)
	delete(server.fowardMap, id)
	server.sendBrowserAction(&BrowserAction{Id: id, CloseTabs: true})
}

func (server *WebServer) sendBrowserAction(action *BrowserAction) {
	server.baChansMutex.Lock()
	defer server.baChansMutex.Unlock()
	log.Println("Sending browser action:", action)
	if len(server.baChans) == 0 {
		log.Println("No websocket connection exists.")
		return
	}
	for baChan, _ := range server.baChans {
		// Checks cap and len of baChan to avoid deadlock.
		// TODO: There might be a better way to synchronize and avoid deadlock.
		if len(baChan) != cap(baChan) {
			baChan <- action
			log.Println("Browser action is added to channel:", action)
		} else {
			log.Println("baChan is full. Skipping...")
		}
	}
}

func handleClientConn(server *WebServer, conn net.Conn) {
	log.Println("An connection with a client is established.")
	defer conn.Close()
	decoder := json.NewDecoder(conn)
	registered := false
	var id uint32
	for {
		req := new(ClientReq)
		err := decoder.Decode(req)
		if err != nil {
			if err == io.EOF {
				log.Println("Reached EOF of client connection.")
			} else {
				log.Println("Failed to read client connection:", err)
			}
			break
		}
		log.Println(*req)
		if len(req.Host) > 0 {
			if registered {
				log.Println("Host is already registered.")
			} else {
				id = server.RegisterProxy(req.Host)
				registered = true
			}
		}
		if len(req.OpenUrl) > 0 {
			if registered {
				action := BrowserAction{Id: id, OpenUrl: req.OpenUrl}
				server.sendBrowserAction(&action)
			} else {
				log.Println("Can not open url because no host is registered.")
			}
		}
		if len(req.Notification) > 0 {
			if registered {
				action := BrowserAction{Id: id, Notification: req.Notification}
				server.sendBrowserAction(&action)
			} else {
				log.Println("Can not show notification because no host is registered.")
			}
		}
	}
	if registered {
		server.UnregisterProxy(id)
	}
}

func openClientServer(port int32, server *WebServer) {
	log.Println("Listening to command connections on", port)
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Println("Failed to listen at client server port:", err)
		return
	}
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Println("Failed to accept a client connection:", err)
			continue
		}
		go handleClientConn(server, conn)
	}
}
