package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/bimakw/multichain-rpc-proxy/internal/chain"
	"github.com/bimakw/multichain-rpc-proxy/internal/metrics"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for RPC proxy
	},
}

type Config struct {
	Enabled          bool          `yaml:"enabled"`
	PingInterval     time.Duration `yaml:"ping_interval"`
	PongTimeout      time.Duration `yaml:"pong_timeout"`
	WriteTimeout     time.Duration `yaml:"write_timeout"`
	MaxMessageSize   int64         `yaml:"max_message_size"`
	ReconnectBackoff time.Duration `yaml:"reconnect_backoff"`
	MaxReconnects    int           `yaml:"max_reconnects"`
}

func DefaultConfig() Config {
	return Config{
		Enabled:          true,
		PingInterval:     30 * time.Second,
		PongTimeout:      10 * time.Second,
		WriteTimeout:     10 * time.Second,
		MaxMessageSize:   512 * 1024, // 512KB
		ReconnectBackoff: 1 * time.Second,
		MaxReconnects:    5,
	}
}

type Handler struct {
	manager *chain.Manager
	config  Config
	pools   map[string]*ConnectionPool
}

func NewHandler(manager *chain.Manager, config Config) *Handler {
	return &Handler{
		manager: manager,
		config:  config,
		pools:   make(map[string]*ConnectionPool),
	}
}

// rpcRequest represents a JSON-RPC request
type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      interface{}     `json:"id"`
}

// rpcResponse represents a JSON-RPC response
type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
	ID      interface{}     `json:"id"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (h *Handler) HandleWebSocket(w http.ResponseWriter, r *http.Request, chainName string) {
	ch, ok := h.manager.GetChain(chainName)
	if !ok {
		http.Error(w, "Chain not found", http.StatusNotFound)
		return
	}

	clientConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WS] Failed to upgrade connection: %v", err)
		return
	}

	log.Printf("[WS][%s] New client connection from %s", chainName, r.RemoteAddr)

	session := &ClientSession{
		chainName:     chainName,
		chain:         ch,
		clientConn:    clientConn,
		handler:       h,
		subscriptions: make(map[string]bool),
		done:          make(chan struct{}),
	}

	session.Run()
}

type ClientSession struct {
	chainName     string
	chain         *chain.Chain
	clientConn    *websocket.Conn
	backendConn   *websocket.Conn
	handler       *Handler
	subscriptions map[string]bool
	done          chan struct{}
	writeMu       sync.Mutex
}

func (s *ClientSession) Run() {
	defer s.Close()

	if err := s.connectToBackend(); err != nil {
		log.Printf("[WS][%s] Failed to connect to backend: %v", s.chainName, err)
		s.sendError(nil, -32603, "Failed to connect to backend")
		return
	}

	go s.forwardFromBackend()
	s.forwardFromClient()
}

// connectToBackend establishes a WebSocket connection to a backend endpoint
func (s *ClientSession) connectToBackend() error {
	stats := s.chain.Stats()

	for _, ep := range stats.Endpoints {
		if !ep.Healthy {
			continue
		}

		// Convert HTTP URL to WebSocket URL
		wsURL := httpToWS(ep.URL)

		dialer := websocket.Dialer{
			HandshakeTimeout: 10 * time.Second,
		}

		conn, _, err := dialer.Dial(wsURL, nil)
		if err != nil {
			log.Printf("[WS][%s] Failed to connect to %s: %v", s.chainName, wsURL, err)
			continue
		}

		s.backendConn = conn
		log.Printf("[WS][%s] Connected to backend: %s", s.chainName, wsURL)
		return nil
	}

	return fmt.Errorf("no healthy WebSocket endpoints available")
}

// forwardFromClient forwards messages from client to backend
func (s *ClientSession) forwardFromClient() {
	s.clientConn.SetReadLimit(s.handler.config.MaxMessageSize)
	s.clientConn.SetReadDeadline(time.Now().Add(s.handler.config.PongTimeout))
	s.clientConn.SetPongHandler(func(string) error {
		s.clientConn.SetReadDeadline(time.Now().Add(s.handler.config.PongTimeout))
		return nil
	})

	for {
		select {
		case <-s.done:
			return
		default:
		}

		messageType, message, err := s.clientConn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("[WS][%s] Client read error: %v", s.chainName, err)
			}
			return
		}

		if messageType != websocket.TextMessage {
			continue
		}

		var req rpcRequest
		if err := json.Unmarshal(message, &req); err != nil {
			s.sendError(nil, -32700, "Parse error")
			continue
		}

		metrics.RecordRequest(s.chainName, req.Method)

		if s.backendConn == nil {
			if err := s.connectToBackend(); err != nil {
				s.sendError(req.ID, -32603, "Backend unavailable")
				continue
			}
		}

		s.backendConn.SetWriteDeadline(time.Now().Add(s.handler.config.WriteTimeout))
		if err := s.backendConn.WriteMessage(websocket.TextMessage, message); err != nil {
			log.Printf("[WS][%s] Backend write error: %v", s.chainName, err)
			s.backendConn.Close()
			s.backendConn = nil

			if err := s.connectToBackend(); err != nil {
				s.sendError(req.ID, -32603, "Backend connection lost")
				continue
			}

			if err := s.backendConn.WriteMessage(websocket.TextMessage, message); err != nil {
				s.sendError(req.ID, -32603, "Failed to send to backend")
				continue
			}
		}
	}
}

// forwardFromBackend forwards messages from backend to client
func (s *ClientSession) forwardFromBackend() {
	for {
		select {
		case <-s.done:
			return
		default:
		}

		if s.backendConn == nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		messageType, message, err := s.backendConn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("[WS][%s] Backend read error: %v", s.chainName, err)
			}

			s.backendConn.Close()
			s.backendConn = nil

			for i := 0; i < s.handler.config.MaxReconnects; i++ {
				time.Sleep(s.handler.config.ReconnectBackoff * time.Duration(i+1))
				if err := s.connectToBackend(); err == nil {
					log.Printf("[WS][%s] Reconnected to backend", s.chainName)
					break
				}
			}

			if s.backendConn == nil {
				log.Printf("[WS][%s] Failed to reconnect to backend", s.chainName)
				return
			}
			continue
		}

		if messageType != websocket.TextMessage {
			continue
		}

		s.writeMu.Lock()
		s.clientConn.SetWriteDeadline(time.Now().Add(s.handler.config.WriteTimeout))
		err = s.clientConn.WriteMessage(websocket.TextMessage, message)
		s.writeMu.Unlock()

		if err != nil {
			log.Printf("[WS][%s] Client write error: %v", s.chainName, err)
			return
		}
	}
}

// sendError sends an error response to the client
func (s *ClientSession) sendError(id interface{}, code int, message string) {
	resp := rpcResponse{
		JSONRPC: "2.0",
		Error: &rpcError{
			Code:    code,
			Message: message,
		},
		ID: id,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		return
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	s.clientConn.SetWriteDeadline(time.Now().Add(s.handler.config.WriteTimeout))
	s.clientConn.WriteMessage(websocket.TextMessage, data)
}

func (s *ClientSession) Close() {
	close(s.done)

	if s.clientConn != nil {
		s.clientConn.Close()
	}
	if s.backendConn != nil {
		s.backendConn.Close()
	}

	log.Printf("[WS][%s] Session closed", s.chainName)
}

// httpToWS converts an HTTP URL to a WebSocket URL
func httpToWS(url string) string {
	if len(url) > 7 && url[:7] == "http://" {
		return "ws://" + url[7:]
	}
	if len(url) > 8 && url[:8] == "https://" {
		return "wss://" + url[8:]
	}
	return url
}

func (s *ClientSession) StartPinger(ctx context.Context) {
	ticker := time.NewTicker(s.handler.config.PingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			return
		case <-ticker.C:
			s.writeMu.Lock()
			s.clientConn.SetWriteDeadline(time.Now().Add(s.handler.config.WriteTimeout))
			err := s.clientConn.WriteMessage(websocket.PingMessage, nil)
			s.writeMu.Unlock()

			if err != nil {
				log.Printf("[WS][%s] Ping error: %v", s.chainName, err)
				return
			}
		}
	}
}
