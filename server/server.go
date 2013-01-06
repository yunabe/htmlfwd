package main

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"code.google.com/p/go.net/websocket"
)

type ClientReq struct {
	Host         string
	OpenUrl      string
	Notification string
	ClientId     string
}

type BrowserAction struct {
	Id                uint32
	OpenUrl           string
	CloseTabs         bool
	Notification      string
	KeepAlive         bool
	KeepAliveInterval uint32
}

func (ba *BrowserAction) String() string {
	return fmt.Sprintf("[Id: %d, OpenUrl: %s, CloseTabs: %t, Notification: %s]",
		ba.Id, ba.OpenUrl, ba.CloseTabs, ba.Notification)
}

type WebServer struct {
	setting                 *Setting
	port                    int32
	keep_alive_interval_sec int32
	mux                     *http.ServeMux
	fowardMap               map[uint32]*httputil.ReverseProxy
	sharedMap               map[string]uint32
	fowardMapMutex          sync.Mutex
	baChans                 map[chan *BrowserAction]bool
	baChansMutex            sync.Mutex
}

func NewWebServer(setting *Setting) *WebServer {
	port := setting.browserPort
	keep_alive_interval_sec := setting.keepAliveIntervalSec
	mux := http.NewServeMux()
	server := WebServer{
		setting: setting,
		port:    port,
		keep_alive_interval_sec: keep_alive_interval_sec,
		fowardMap:               make(map[uint32]*httputil.ReverseProxy),
		sharedMap:               make(map[string]uint32),
		mux:                     mux,
		baChans:                 make(map[chan *BrowserAction]bool),
	}
	mux.Handle("/ws", websocket.Handler(
		func(ws *websocket.Conn) {
			// This looks very redandant :{
			server.handleWebSocket(ws)
		}))
	mux.HandleFunc("/fwd/", func(w http.ResponseWriter, req *http.Request) {
		server.handleForward(w, req)
	})
	mux.HandleFunc("/shared/", func(w http.ResponseWriter, req *http.Request) {
		server.handleShared(w, req)
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

	log.Println("Sending keep-alive interval.")
	wsEncoder.Encode(&BrowserAction{KeepAliveInterval: uint32(server.keep_alive_interval_sec)})
Loop:
	for {
		timer := time.After(
			time.Duration(server.keep_alive_interval_sec) * 1000 * 1000 * 1000)
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
	if len(matches) == 0 {
		log.Println("Invalid url pattern.")
		return
	}
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

func (server *WebServer) handleShared(w http.ResponseWriter, req *http.Request) {
	server.fowardMapMutex.Lock()
	defer server.fowardMapMutex.Unlock()
	log.Println("r.URL =", req.URL)
	pattern, _ := regexp.Compile("^/shared/([\\w\\-]+)/(.*)$")
	var matches []string = pattern.FindStringSubmatch(req.URL.String())
	if len(matches) == 0 {
		log.Println("Invalid url pattern!")
		return
	}
	uuid := matches[1]
	id, ok := server.sharedMap[uuid]
	if !ok {
		log.Println("Client id is not registered:", uuid)
		return
	}
	proxy, ok := server.fowardMap[id]
	if !ok {
		log.Println("%d is not registered.", id)
		return
	}
	req.URL, _ = url.Parse("/shared/" + matches[2])
	log.Println("proxy.ServeHTTP(w, req)")
	proxy.ServeHTTP(w, req)
}

func (server *WebServer) ListenAndServe() {
	log.Println("Listening to websockets on", server.port)
	var err error
	if server.setting.useSsl {
		var config *tls.Config = nil
		if server.setting.authenticateBrowser {
			certPool := x509.NewCertPool()
			func() {
				fi, err := os.Open(server.setting.browserRootCert)
				if err != nil {
					panic(err)
				}
				defer fi.Close()
				buf := new(bytes.Buffer)
				reader := bufio.NewReader(fi)
				io.Copy(buf, reader)
				if ok := certPool.AppendCertsFromPEM(buf.Bytes()); !ok {
					panic("Failed to append PEM.")
				}
				config = &tls.Config{
					ClientAuth: tls.RequireAndVerifyClientCert,
					ClientCAs:  certPool,
				}
			}()
		}
		http_server := &http.Server{
			Addr:      fmt.Sprintf(":%d", server.port),
			Handler:   server.mux,
			TLSConfig: config,
		}
		err = http_server.ListenAndServeTLS(
			server.setting.serverCertificate,
			server.setting.serverPrivateKey)
	} else {
		err = http.ListenAndServe(fmt.Sprintf(":%d", server.port), server.mux)
	}
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

func (server *WebServer) RegisterSharedMap(clientId string, proxyId uint32) {
	server.fowardMapMutex.Lock()
	defer server.fowardMapMutex.Unlock()
	server.sharedMap[clientId] = proxyId
}

func (server *WebServer) UnregisterSharedMap(clientId string) {
	server.fowardMapMutex.Lock()
	defer server.fowardMapMutex.Unlock()
	delete(server.sharedMap, clientId)
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
	proxyRegistered := false
	shareMapRegistered := false
	var proxyId uint32
	var clientId string
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
		log.Println("Client request:", *req)
		if len(req.Host) > 0 {
			if proxyRegistered {
				log.Println("Host is already registered.")
			} else {
				proxyId = server.RegisterProxy(req.Host)
				proxyRegistered = true
			}
		}
		if len(req.ClientId) > 0 {
			if len(clientId) > 0 {
				log.Println("Client id is already registered. Ignored.")
			} else {
				clientId = req.ClientId
			}
		}
		if proxyRegistered && len(clientId) > 0 && !shareMapRegistered {
			server.RegisterSharedMap(clientId, proxyId)
			shareMapRegistered = true
		}
		if len(req.OpenUrl) > 0 {
			if strings.HasPrefix(req.OpenUrl, "http://") || strings.HasPrefix(req.OpenUrl, "https://") {
				action := BrowserAction{OpenUrl: req.OpenUrl}
				server.sendBrowserAction(&action)
			} else if proxyRegistered {
				action := BrowserAction{Id: proxyId, OpenUrl: req.OpenUrl}
				server.sendBrowserAction(&action)
			} else {
				log.Println("Can not open url because no host is registered.")
			}
		}
		if len(req.Notification) > 0 {
			if proxyRegistered {
				action := BrowserAction{Id: proxyId, Notification: req.Notification}
				server.sendBrowserAction(&action)
			} else {
				log.Println("Can not show notification because no host is registered.")
			}
		}
	}
	if shareMapRegistered {
		server.UnregisterSharedMap(clientId)
	}
	if proxyRegistered {
		server.UnregisterProxy(proxyId)
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
