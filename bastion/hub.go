package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
)

// wsCheckOrigin returns true if the request origin is allowed. allowed is from config (CORSOrigin); "" or "*" allows all.
func wsCheckOrigin(allowed string) func(*http.Request) bool {
	return func(r *http.Request) bool {
		if allowed == "*" {
			return true
		}
		origin := r.Header.Get("Origin")
		// No origin header (e.g. non-browser clients like the daemon) — allow.
		if origin == "" {
			return true
		}
		// If CORS_ORIGIN is not configured, reject browser origins.
		if allowed == "" {
			return false
		}
		for _, o := range strings.Split(allowed, ",") {
			if strings.TrimSpace(o) == origin {
				return true
			}
		}
		return false
	}
}

// Hub holds connected daemons by daemon ID.
type Hub struct {
	mu      sync.RWMutex
	daemons map[string]*DaemonConn
}

// binaryResponse pairs a JSON control frame with an optional binary data frame.
type binaryResponse struct {
	JSON   json.RawMessage
	Binary []byte
}

// DaemonConn is a single daemon WebSocket with request/response pairing.
type DaemonConn struct {
	DaemonID      string
	conn          *websocket.Conn
	mu            sync.Mutex // guards pending and pendingBinary maps
	writeMu       sync.Mutex // serializes WebSocket writes
	pending       map[string]chan json.RawMessage
	pendingBinary map[string]chan binaryResponse
	done          chan struct{}
}

func NewHub() *Hub {
	return &Hub{daemons: make(map[string]*DaemonConn)}
}

func (h *Hub) Register(daemonID string, conn *websocket.Conn) *DaemonConn {
	ac := &DaemonConn{
		DaemonID:      daemonID,
		conn:          conn,
		pending:       make(map[string]chan json.RawMessage),
		pendingBinary: make(map[string]chan binaryResponse),
		done:          make(chan struct{}),
	}
	h.mu.Lock()
	if old, ok := h.daemons[daemonID]; ok {
		old.close()
	}
	h.daemons[daemonID] = ac
	h.mu.Unlock()
	return ac
}

func (h *Hub) Unregister(daemonID string) {
	h.mu.Lock()
	delete(h.daemons, daemonID)
	h.mu.Unlock()
}

func (h *Hub) Get(daemonID string) *DaemonConn {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.daemons[daemonID]
}

func (h *Hub) Connected(daemonID string) bool {
	return h.Get(daemonID) != nil
}

func (ac *DaemonConn) close() {
	ac.mu.Lock()
	for _, ch := range ac.pending {
		select {
		case ch <- nil:
		default:
		}
	}
	ac.pending = nil
	for _, ch := range ac.pendingBinary {
		select {
		case ch <- binaryResponse{}:
		default:
		}
	}
	ac.pendingBinary = nil
	ac.mu.Unlock()
	ac.conn.Close()
	close(ac.done)
}

// Request sends a JSON message to the daemon and waits for the response (by request_id).
func (ac *DaemonConn) Request(ctx context.Context, requestID string, req interface{}) (json.RawMessage, error) {
	if requestID == "" {
		return nil, errNoRequestID
	}
	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	ch := make(chan json.RawMessage, 1)
	ac.mu.Lock()
	if ac.pending == nil {
		ac.mu.Unlock()
		return nil, errConnClosed
	}
	ac.pending[requestID] = ch
	ac.mu.Unlock()
	defer func() {
		ac.mu.Lock()
		delete(ac.pending, requestID)
		ac.mu.Unlock()
	}()
	ac.writeMu.Lock()
	err = ac.conn.WriteMessage(websocket.TextMessage, data)
	ac.writeMu.Unlock()
	if err != nil {
		return nil, err
	}
	select {
	case resp := <-ch:
		if resp == nil {
			return nil, errConnClosed
		}
		return resp, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-ac.done:
		return nil, errConnClosed
	}
}

// RequestWithBinary sends a JSON text frame followed by a binary frame atomically,
// then waits for the response. Used for chunked uploads to avoid base64 overhead.
func (ac *DaemonConn) RequestWithBinary(ctx context.Context, requestID string, req interface{}, payload []byte) (json.RawMessage, error) {
	if requestID == "" {
		return nil, errNoRequestID
	}
	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	ch := make(chan json.RawMessage, 1)
	ac.mu.Lock()
	if ac.pending == nil {
		ac.mu.Unlock()
		return nil, errConnClosed
	}
	ac.pending[requestID] = ch
	ac.mu.Unlock()
	defer func() {
		ac.mu.Lock()
		delete(ac.pending, requestID)
		ac.mu.Unlock()
	}()
	// Atomic two-frame write: JSON control + binary payload
	ac.writeMu.Lock()
	err = ac.conn.WriteMessage(websocket.TextMessage, data)
	if err == nil {
		err = ac.conn.WriteMessage(websocket.BinaryMessage, payload)
	}
	ac.writeMu.Unlock()
	if err != nil {
		return nil, err
	}
	select {
	case resp := <-ch:
		if resp == nil {
			return nil, errConnClosed
		}
		return resp, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-ac.done:
		return nil, errConnClosed
	}
}

// RequestExpectBinary sends a JSON request and waits for a JSON control frame
// followed by a binary data frame. Used for streaming downloads.
func (ac *DaemonConn) RequestExpectBinary(ctx context.Context, requestID string, req interface{}) (json.RawMessage, []byte, error) {
	if requestID == "" {
		return nil, nil, errNoRequestID
	}
	data, err := json.Marshal(req)
	if err != nil {
		return nil, nil, err
	}
	ch := make(chan binaryResponse, 1)
	ac.mu.Lock()
	if ac.pendingBinary == nil {
		ac.mu.Unlock()
		return nil, nil, errConnClosed
	}
	ac.pendingBinary[requestID] = ch
	ac.mu.Unlock()
	defer func() {
		ac.mu.Lock()
		delete(ac.pendingBinary, requestID)
		ac.mu.Unlock()
	}()
	ac.writeMu.Lock()
	err = ac.conn.WriteMessage(websocket.TextMessage, data)
	ac.writeMu.Unlock()
	if err != nil {
		return nil, nil, err
	}
	select {
	case resp := <-ch:
		if resp.JSON == nil {
			return nil, nil, errConnClosed
		}
		return resp.JSON, resp.Binary, nil
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	case <-ac.done:
		return nil, nil, errConnClosed
	}
}

var errNoRequestID = fmt.Errorf("request_id required")
var errConnClosed = fmt.Errorf("connection closed")

// readLoop reads responses and dispatches to pending channels. Run in goroutine.
func (ac *DaemonConn) readLoop(hub *Hub) {
	defer func() {
		hub.Unregister(ac.DaemonID)
		ac.close()
	}()
	for {
		msgType, data, err := ac.conn.ReadMessage()
		if err != nil {
			return
		}
		if msgType == websocket.BinaryMessage {
			// Unexpected standalone binary frame — skip
			continue
		}
		var envelope struct {
			RequestID string `json:"request_id"`
			Error     string `json:"error,omitempty"`
		}
		if json.Unmarshal(data, &envelope) != nil {
			continue
		}
		ac.mu.Lock()
		bch := ac.pendingBinary[envelope.RequestID]
		ch := ac.pending[envelope.RequestID]
		ac.mu.Unlock()

		if bch != nil {
			// This request expects a binary follow-up frame
			if envelope.Error != "" {
				// Error response — no binary frame follows
				select {
				case bch <- binaryResponse{JSON: data}:
				default:
				}
			} else {
				// Read the binary data frame
				_, binData, err := ac.conn.ReadMessage()
				if err != nil {
					return
				}
				select {
				case bch <- binaryResponse{JSON: data, Binary: binData}:
				default:
				}
			}
		} else if ch != nil {
			select {
			case ch <- data:
			default:
			}
		}
	}
}
