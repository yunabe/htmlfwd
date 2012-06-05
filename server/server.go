package main

import (
	"encoding/json"
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

	"code.google.com/p/go.net/websocket"
)

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
}

type WebServer struct {
	mux            *http.ServeMux
	fowardMap      map[uint32]*httputil.ReverseProxy
	fowardMapMutex sync.Mutex
	baChans        map[chan *BrowserAction]bool
	baChansMutex   sync.Mutex
}

func NewWebServer() *WebServer {
	mux := http.NewServeMux()
	server := WebServer{
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
			if (err == io.EOF) {
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
	log.Print("WebSocket connection is established.")
	defer ws.Close()
	wsEncoder := json.NewEncoder(ws)

	bachan := make(chan *BrowserAction, 100)
	server.baChansMutex.Lock()
	server.baChans[bachan] = true
	server.baChansMutex.Unlock()

	wsCloseChan := createWebSocketCloseChannel(ws)

Loop:
	for {
		select {
		case ba, ok := <-bachan:
			if !ok {
				panic("bachan is closed unexpectedly.")
				break Loop
			}
			wsEncoder.Encode(ba)
		case _, ok := <-wsCloseChan:
			if !ok {
				log.Print("WebSocket is closed by peer.")
				server.baChansMutex.Lock()
				close(bachan)
				for {
					if _, ok := <-bachan; ok {
						log.Print("BrowserAction is discarded.")
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
		}
	}
}

func (server *WebServer) handleForward(w http.ResponseWriter, req *http.Request) {
	server.fowardMapMutex.Lock()
	defer server.fowardMapMutex.Unlock()
	log.Print("r.URL =", req.URL)
	pattern, _ := regexp.Compile("^/fwd/(\\d+)(/.*)$")
	var matches []string = pattern.FindStringSubmatch(req.URL.String())
	log.Print(matches)
	id, _ := strconv.ParseUint(matches[1], 10, 32)
	proxy, ok := server.fowardMap[uint32(id)]
	if !ok {
		log.Print("....")
		return
	}
	req.URL, _ = url.Parse(matches[2])
	log.Print("proxy.ServeHTTP(w, req)")
	proxy.ServeHTTP(w, req)
}

func (server *WebServer) ListenAndServe() {
	http.ListenAndServe(":8080", server.mux)
}

func (server *WebServer) RegisterProxy(host string) uint32 {
	server.fowardMapMutex.Lock()
	defer server.fowardMapMutex.Unlock()
	id := rand.Uint32()
	log.Print("register", id, host)
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
	log.Print("Unregister", id)
	delete(server.fowardMap, id)
	server.sendBrowserAction(&BrowserAction{Id: id, CloseTabs: true})
}

func (server *WebServer) sendBrowserAction(action *BrowserAction) {
	server.baChansMutex.Lock()
	defer server.baChansMutex.Unlock()
	if len(server.baChans) == 0 {
		log.Print("No websocket connection exists.")
		return
	}
	for baChan, _ := range server.baChans {
		// Checks cap and len of baChan to avoid deadlock.
		// TODO: There might be a better way to synchronize and avoid deadlock.
		if len(baChan) != cap(baChan) {
			baChan <- action
		} else {
			log.Print("baChan is full. Skipping...")
		}
	}
}

func handleClientConn(server *WebServer, conn net.Conn) {
	log.Print("handleClientConn")
	defer conn.Close()
	decoder := json.NewDecoder(conn)
	registered := false
	var id uint32
	for {
		req := new(ClientReq)
		err := decoder.Decode(req)
		if err != nil {
			log.Print(err)
			break
		}
		log.Print(*req)
		if len(req.Host) > 0 {
			if registered {
				log.Print("Host is already registered.")
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
				log.Print("Can not open url because no host is registered.")
			}
		}
		if len(req.Notification) > 0 {
			if registered {
				action := BrowserAction{Id: id, Notification: req.Notification}
				server.sendBrowserAction(&action)
			} else {
				log.Print("Can not show notification because no host is registered.")
			}
		}
	}
	if registered {
		server.UnregisterProxy(id)
	}
}

func openClientServer(server *WebServer) {
	ln, err := net.Listen("tcp", ":9999")
	if err != nil {
		log.Print("error:", err)
		return
	}
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Print("error:", err)
			continue
		}
		go handleClientConn(server, conn)
	}
}
