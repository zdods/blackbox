package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestHubRegisterAndGet(t *testing.T) {
	hub := NewHub()
	// Create a real WebSocket connection via test server
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		ac := hub.Register("daemon-1", conn)
		go ac.readLoop(hub)
		// Keep handler alive until test completes
		<-ac.done
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Give the server time to register
	time.Sleep(50 * time.Millisecond)

	if !hub.Connected("daemon-1") {
		t.Error("daemon-1 should be connected")
	}
	if hub.Connected("daemon-999") {
		t.Error("daemon-999 should not be connected")
	}
	ac := hub.Get("daemon-1")
	if ac == nil {
		t.Fatal("Get should return non-nil for registered daemon")
	}
	if ac.DaemonID != "daemon-1" {
		t.Errorf("DaemonID = %q, want %q", ac.DaemonID, "daemon-1")
	}
}

func TestHubUnregister(t *testing.T) {
	hub := NewHub()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		hub.Register("daemon-2", conn)
		// Don't start readLoop so we can control lifecycle
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	time.Sleep(50 * time.Millisecond)

	if !hub.Connected("daemon-2") {
		t.Fatal("daemon-2 should be connected before unregister")
	}
	hub.Unregister("daemon-2")
	if hub.Connected("daemon-2") {
		t.Error("daemon-2 should not be connected after unregister")
	}
}

func TestDaemonConnRequestResponse(t *testing.T) {
	hub := NewHub()

	// Server: register daemon, start readLoop, and echo responses
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		ac := hub.Register("echo-daemon", conn)
		go ac.readLoop(hub)
		<-ac.done
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer clientConn.Close()

	time.Sleep(50 * time.Millisecond)

	ac := hub.Get("echo-daemon")
	if ac == nil {
		t.Fatal("daemon not found in hub")
	}

	// Simulate: bastion sends request, daemon (client) reads and replies
	go func() {
		// Client side: read the request, send back a response
		_, data, err := clientConn.ReadMessage()
		if err != nil {
			return
		}
		var envelope struct {
			RequestID string `json:"request_id"`
		}
		json.Unmarshal(data, &envelope)
		resp := map[string]string{
			"request_id": envelope.RequestID,
			"type":       "test_response",
			"result":     "ok",
		}
		clientConn.WriteJSON(resp)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	reqBody := map[string]string{
		"request_id": "req-001",
		"type":       "test_request",
	}
	respData, err := ac.Request(ctx, "req-001", reqBody)
	if err != nil {
		t.Fatalf("Request: %v", err)
	}

	var resp map[string]string
	if err := json.Unmarshal(respData, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp["result"] != "ok" {
		t.Errorf("result = %q, want %q", resp["result"], "ok")
	}
}

func TestDaemonConnRequestTimeout(t *testing.T) {
	hub := NewHub()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		ac := hub.Register("slow-daemon", conn)
		go ac.readLoop(hub)
		<-ac.done
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer clientConn.Close()

	time.Sleep(50 * time.Millisecond)

	ac := hub.Get("slow-daemon")
	if ac == nil {
		t.Fatal("daemon not found")
	}

	// Don't send any response from client — should timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err = ac.Request(ctx, "req-timeout", map[string]string{"request_id": "req-timeout"})
	if err == nil {
		t.Error("Request should fail with timeout")
	}
}

func TestWsCheckOriginAllowAll(t *testing.T) {
	check := wsCheckOrigin("*")
	req := httptest.NewRequest("GET", "/ws", nil)
	req.Header.Set("Origin", "https://evil.com")
	if !check(req) {
		t.Error("* should allow any origin")
	}
}

func TestWsCheckOriginEmptyRejectsBrowserOrigin(t *testing.T) {
	check := wsCheckOrigin("")
	req := httptest.NewRequest("GET", "/ws", nil)
	req.Header.Set("Origin", "https://evil.com")
	if check(req) {
		t.Error("empty CORS_ORIGIN should reject browser origins")
	}
}

func TestWsCheckOriginEmptyAllowsNoOrigin(t *testing.T) {
	check := wsCheckOrigin("")
	req := httptest.NewRequest("GET", "/ws", nil)
	// No Origin header (daemon client)
	if !check(req) {
		t.Error("empty CORS_ORIGIN should allow requests with no Origin header")
	}
}

func TestWsCheckOriginSpecific(t *testing.T) {
	check := wsCheckOrigin("https://app.example.com")

	req := httptest.NewRequest("GET", "/ws", nil)
	req.Header.Set("Origin", "https://app.example.com")
	if !check(req) {
		t.Error("should allow matching origin")
	}

	req.Header.Set("Origin", "https://evil.com")
	if check(req) {
		t.Error("should reject non-matching origin")
	}
}

func TestWsCheckOriginMultiple(t *testing.T) {
	check := wsCheckOrigin("https://a.com, https://b.com")

	req := httptest.NewRequest("GET", "/ws", nil)

	req.Header.Set("Origin", "https://a.com")
	if !check(req) {
		t.Error("should allow first origin")
	}

	req.Header.Set("Origin", "https://b.com")
	if !check(req) {
		t.Error("should allow second origin")
	}

	req.Header.Set("Origin", "https://c.com")
	if check(req) {
		t.Error("should reject unlisted origin")
	}
}
