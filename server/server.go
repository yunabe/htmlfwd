package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strconv"

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
	fowardMap           map[uint32]*httputil.ReverseProxy
	mux                 *http.ServeMux
	wsBrowserActionChan chan *BrowserAction
}

func NewWebServer() *WebServer {
	mux := http.NewServeMux()
	server := WebServer{fowardMap: make(map[uint32]*httputil.ReverseProxy), mux: mux}
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

func (server *WebServer) handleWebSocket(ws *websocket.Conn) {
	// TODO: Handle a case where websocket is closed by peer.
	defer ws.Close()
	wsEncoder := json.NewEncoder(ws)

	bachan := make(chan *BrowserAction)
	if server.wsBrowserActionChan != nil {
		close(server.wsBrowserActionChan)
	}
	server.wsBrowserActionChan = bachan

	for {
		ba, ok := <-bachan
		if !ok {
			break
		}
		wsEncoder.Encode(ba)
	}
}

func (server *WebServer) handleForward(w http.ResponseWriter, req *http.Request) {
	fmt.Println("r.URL =", req.URL)
	pattern, _ := regexp.Compile("^/fwd/(\\d+)(/.*)$")
	var matches []string = pattern.FindStringSubmatch(req.URL.String())
	fmt.Println(matches)
	id, _ := strconv.ParseUint(matches[1], 10, 32)
	proxy, ok := server.fowardMap[uint32(id)]
	if !ok {
		fmt.Println("....")
		return
	}
	req.URL, _ = url.Parse(matches[2])
	fmt.Println("proxy.ServeHTTP(w, req)")
	proxy.ServeHTTP(w, req)
}

func (server *WebServer) ListenAndServe() {
	http.ListenAndServe(":8080", server.mux)
}

func (server *WebServer) RegisterProxy(host string) uint32 {
	id := rand.Uint32()
	fmt.Println("register", id, host)
	target := url.URL{
		Scheme: "http",
		Host:   host,
	}
	server.fowardMap[id] = httputil.NewSingleHostReverseProxy(&target)
	return id
}

func (server *WebServer) UnregisterProxy(id uint32) {
	fmt.Println("Unregister", id)
	delete(server.fowardMap, id)
	if server.wsBrowserActionChan != nil {
		server.wsBrowserActionChan <- &BrowserAction{Id: id, CloseTabs: true}
	}
}

func (server *WebServer) sendBrowserAction(action *BrowserAction) {
	if server.wsBrowserActionChan == nil {
		fmt.Println("No websocket connection exists.")
		return
	}
	server.wsBrowserActionChan <- action
}

func handleClientConn(server *WebServer, conn net.Conn) {
	fmt.Println("handleClientConn")
	defer conn.Close()
	decoder := json.NewDecoder(conn)
	registered := false
	var id uint32
	for {
		req := new(ClientReq)
		err := decoder.Decode(req)
		if err != nil {
			fmt.Println(err)
			break
		}
		fmt.Println(*req)
		if len(req.Host) > 0 {
			if registered {
				fmt.Println("Host is already registered.")
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
				fmt.Println("Can not open url because no host is registered.")
			}
		}
		if len(req.Notification) > 0 {
			if registered {
				action := BrowserAction{Id: id, Notification: req.Notification}
				server.sendBrowserAction(&action)
			} else {
				fmt.Println("Can not show notification because no host is registered.")
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
		fmt.Println("error:", err)
		return
	}
	for {
		conn, err := ln.Accept()
		if err != nil {
			fmt.Println("error:", err)
			continue
		}
		go handleClientConn(server, conn)
	}
}
