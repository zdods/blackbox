package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Hub holds connected agents by agent ID.
type Hub struct {
	mu     sync.RWMutex
	agents map[string]*AgentConn
}

// AgentConn is a single agent WebSocket with request/response pairing.
type AgentConn struct {
	AgentID string
	conn   *websocket.Conn
	mu     sync.Mutex
	pending map[string]chan json.RawMessage
	done   chan struct{}
}

func NewHub() *Hub {
	return &Hub{agents: make(map[string]*AgentConn)}
}

func (h *Hub) Register(agentID string, conn *websocket.Conn) *AgentConn {
	ac := &AgentConn{
		AgentID: agentID,
		conn:    conn,
		pending: make(map[string]chan json.RawMessage),
		done:    make(chan struct{}),
	}
	h.mu.Lock()
	if old, ok := h.agents[agentID]; ok {
		old.close()
	}
	h.agents[agentID] = ac
	h.mu.Unlock()
	return ac
}

func (h *Hub) Unregister(agentID string) {
	h.mu.Lock()
	delete(h.agents, agentID)
	h.mu.Unlock()
}

func (h *Hub) Get(agentID string) *AgentConn {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.agents[agentID]
}

func (h *Hub) Connected(agentID string) bool {
	return h.Get(agentID) != nil
}

func (ac *AgentConn) close() {
	ac.mu.Lock()
	for _, ch := range ac.pending {
		select {
		case ch <- nil:
		default:
		}
	}
	ac.pending = nil
	ac.mu.Unlock()
	ac.conn.Close()
	close(ac.done)
}

// Request sends a JSON message to the agent and waits for the response (by request_id).
func (ac *AgentConn) Request(ctx context.Context, requestID string, req interface{}) (json.RawMessage, error) {
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
	if err := ac.conn.WriteMessage(websocket.TextMessage, data); err != nil {
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

var errNoRequestID = fmt.Errorf("request_id required")
var errConnClosed = fmt.Errorf("connection closed")

// readLoop reads responses and dispatches to pending channels. Run in goroutine.
func (ac *AgentConn) readLoop(hub *Hub) {
	defer func() {
		hub.Unregister(ac.AgentID)
		ac.close()
	}()
	for {
		_, data, err := ac.conn.ReadMessage()
		if err != nil {
			return
		}
		var envelope struct {
			RequestID string `json:"request_id"`
		}
		if json.Unmarshal(data, &envelope) != nil {
			continue
		}
		ac.mu.Lock()
		ch := ac.pending[envelope.RequestID]
		ac.mu.Unlock()
		if ch != nil {
			select {
			case ch <- data:
			default:
			}
		}
	}
}
