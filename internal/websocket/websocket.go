package websocket

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// Config represents WebSocket server configuration
type Config struct {
	ReadBufferSize  int           `yaml:"readBufferSize" json:"readBufferSize"`
	WriteBufferSize int           `yaml:"writeBufferSize" json:"writeBufferSize"`
	CheckOrigin     bool          `yaml:"checkOrigin" json:"checkOrigin"`
	PingInterval    time.Duration `yaml:"pingInterval" json:"pingInterval"`
	PongWait        time.Duration `yaml:"pongWait" json:"pongWait"`
	WriteWait       time.Duration `yaml:"writeWait" json:"writeWait"`
	MaxMessageSize  int64         `yaml:"maxMessageSize" json:"maxMessageSize"`
}

// Message represents a WebSocket message
type Message struct {
	Type      string                 `json:"type"`
	ID        string                 `json:"id,omitempty"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Error     string                 `json:"error,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
}

// MessageType constants
const (
	MessageTypeRequest     = "request"
	MessageTypeResponse    = "response"
	MessageTypeError       = "error"
	MessageTypeEvent       = "event"
	MessageTypePing        = "ping"
	MessageTypePong        = "pong"
	MessageTypeSubscribe   = "subscribe"
	MessageTypeUnsubscribe = "unsubscribe"
)

// Client represents a WebSocket client connection
type Client struct {
	ID            string
	conn          *websocket.Conn
	send          chan Message
	hub           *Hub
	subscriptions map[string]bool
	mutex         sync.RWMutex
	logger        *zap.Logger
}

// NewClient creates a new WebSocket client
func NewClient(id string, conn *websocket.Conn, hub *Hub, logger *zap.Logger) *Client {
	return &Client{
		ID:            id,
		conn:          conn,
		send:          make(chan Message, 256),
		hub:           hub,
		subscriptions: make(map[string]bool),
		logger:        logger.Named("client").With(zap.String("clientId", id)),
	}
}

// Subscribe adds a subscription for the client
func (c *Client) Subscribe(topic string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.subscriptions[topic] = true
	c.logger.Debug("Client subscribed to topic", zap.String("topic", topic))
}

// Unsubscribe removes a subscription for the client
func (c *Client) Unsubscribe(topic string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	delete(c.subscriptions, topic)
	c.logger.Debug("Client unsubscribed from topic", zap.String("topic", topic))
}

// IsSubscribed checks if client is subscribed to a topic
func (c *Client) IsSubscribed(topic string) bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.subscriptions[topic]
}

// Send sends a message to the client
func (c *Client) Send(message Message) {
	select {
	case c.send <- message:
	default:
		close(c.send)
		c.hub.unregister <- c
	}
}

// ReadPump pumps messages from the websocket connection to the hub
func (c *Client) ReadPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(c.hub.config.MaxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(c.hub.config.PongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(c.hub.config.PongWait))
		return nil
	})

	for {
		var message Message
		err := c.conn.ReadJSON(&message)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.logger.Error("WebSocket error", zap.Error(err))
			}
			break
		}

		message.Timestamp = time.Now()
		c.hub.handleMessage(c, message)
	}
}

// WritePump pumps messages from the hub to the websocket connection
func (c *Client) WritePump() {
	ticker := time.NewTicker(c.hub.config.PingInterval)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(c.hub.config.WriteWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.conn.WriteJSON(message); err != nil {
				c.logger.Error("Failed to write message", zap.Error(err))
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(c.hub.config.WriteWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// Hub maintains the set of active clients and broadcasts messages to the clients
type Hub struct {
	clients    map[*Client]bool
	broadcast  chan BroadcastMessage
	register   chan *Client
	unregister chan *Client
	config     Config
	logger     *zap.Logger
	handlers   map[string]MessageHandler
	mutex      sync.RWMutex
}

// BroadcastMessage represents a message to be broadcast
type BroadcastMessage struct {
	Topic   string
	Message Message
}

// MessageHandler represents a function that handles WebSocket messages
type MessageHandler func(client *Client, message Message) error

// NewHub creates a new WebSocket hub
func NewHub(config Config, logger *zap.Logger) *Hub {
	// Set defaults
	if config.ReadBufferSize == 0 {
		config.ReadBufferSize = 1024
	}
	if config.WriteBufferSize == 0 {
		config.WriteBufferSize = 1024
	}
	if config.PingInterval == 0 {
		config.PingInterval = 54 * time.Second
	}
	if config.PongWait == 0 {
		config.PongWait = 60 * time.Second
	}
	if config.WriteWait == 0 {
		config.WriteWait = 10 * time.Second
	}
	if config.MaxMessageSize == 0 {
		config.MaxMessageSize = 512
	}

	hub := &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan BroadcastMessage),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		config:     config,
		logger:     logger.Named("websocket-hub"),
		handlers:   make(map[string]MessageHandler),
	}

	// Register default handlers
	hub.RegisterHandler(MessageTypePing, hub.handlePing)
	hub.RegisterHandler(MessageTypeSubscribe, hub.handleSubscribe)
	hub.RegisterHandler(MessageTypeUnsubscribe, hub.handleUnsubscribe)

	return hub
}

// RegisterHandler registers a message handler for a specific message type
func (h *Hub) RegisterHandler(messageType string, handler MessageHandler) {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	h.handlers[messageType] = handler
	h.logger.Info("Registered WebSocket message handler", zap.String("messageType", messageType))
}

// handleMessage processes incoming messages from clients
func (h *Hub) handleMessage(client *Client, message Message) {
	h.mutex.RLock()
	handler, exists := h.handlers[message.Type]
	h.mutex.RUnlock()

	if !exists {
		errorMsg := Message{
			Type:      MessageTypeError,
			ID:        message.ID,
			Error:     fmt.Sprintf("Unknown message type: %s", message.Type),
			Timestamp: time.Now(),
		}
		client.Send(errorMsg)
		return
	}

	if err := handler(client, message); err != nil {
		h.logger.Error("Message handler error",
			zap.String("messageType", message.Type),
			zap.String("clientId", client.ID),
			zap.Error(err))

		errorMsg := Message{
			Type:      MessageTypeError,
			ID:        message.ID,
			Error:     err.Error(),
			Timestamp: time.Now(),
		}
		client.Send(errorMsg)
	}
}

// handlePing handles ping messages
func (h *Hub) handlePing(client *Client, message Message) error {
	response := Message{
		Type:      MessageTypePong,
		ID:        message.ID,
		Timestamp: time.Now(),
	}
	client.Send(response)
	return nil
}

// handleSubscribe handles subscription requests
func (h *Hub) handleSubscribe(client *Client, message Message) error {
	topic, ok := message.Data["topic"].(string)
	if !ok {
		return fmt.Errorf("missing or invalid topic in subscribe message")
	}

	client.Subscribe(topic)

	response := Message{
		Type: MessageTypeResponse,
		ID:   message.ID,
		Data: map[string]interface{}{
			"action": "subscribed",
			"topic":  topic,
		},
		Timestamp: time.Now(),
	}
	client.Send(response)
	return nil
}

// handleUnsubscribe handles unsubscription requests
func (h *Hub) handleUnsubscribe(client *Client, message Message) error {
	topic, ok := message.Data["topic"].(string)
	if !ok {
		return fmt.Errorf("missing or invalid topic in unsubscribe message")
	}

	client.Unsubscribe(topic)

	response := Message{
		Type: MessageTypeResponse,
		ID:   message.ID,
		Data: map[string]interface{}{
			"action": "unsubscribed",
			"topic":  topic,
		},
		Timestamp: time.Now(),
	}
	client.Send(response)
	return nil
}

// Run starts the hub and handles client registration/unregistration and message broadcasting
func (h *Hub) Run(ctx context.Context) {
	h.logger.Info("Starting WebSocket hub")

	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
			h.logger.Info("Client registered", zap.String("clientId", client.ID))

		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
				h.logger.Info("Client unregistered", zap.String("clientId", client.ID))
			}

		case broadcastMsg := <-h.broadcast:
			for client := range h.clients {
				if broadcastMsg.Topic == "" || client.IsSubscribed(broadcastMsg.Topic) {
					select {
					case client.send <- broadcastMsg.Message:
					default:
						close(client.send)
						delete(h.clients, client)
					}
				}
			}

		case <-ctx.Done():
			h.logger.Info("Stopping WebSocket hub")
			return
		}
	}
}

// Broadcast sends a message to all subscribed clients
func (h *Hub) Broadcast(topic string, message Message) {
	broadcastMsg := BroadcastMessage{
		Topic:   topic,
		Message: message,
	}

	select {
	case h.broadcast <- broadcastMsg:
	default:
		h.logger.Warn("Broadcast channel full, dropping message",
			zap.String("topic", topic),
			zap.String("messageType", message.Type))
	}
}

// GetClientCount returns the number of connected clients
func (h *Hub) GetClientCount() int {
	return len(h.clients)
}

// Server represents a WebSocket server
type Server struct {
	hub      *Hub
	upgrader websocket.Upgrader
	logger   *zap.Logger
}

// NewServer creates a new WebSocket server
func NewServer(config Config, logger *zap.Logger) *Server {
	hub := NewHub(config, logger)

	upgrader := websocket.Upgrader{
		ReadBufferSize:  config.ReadBufferSize,
		WriteBufferSize: config.WriteBufferSize,
		CheckOrigin: func(r *http.Request) bool {
			return config.CheckOrigin || true // Allow all origins for now
		},
	}

	return &Server{
		hub:      hub,
		upgrader: upgrader,
		logger:   logger.Named("websocket-server"),
	}
}

// RegisterHandler registers a message handler
func (s *Server) RegisterHandler(messageType string, handler MessageHandler) {
	s.hub.RegisterHandler(messageType, handler)
}

// Broadcast sends a message to all clients subscribed to a topic
func (s *Server) Broadcast(topic string, message Message) {
	s.hub.Broadcast(topic, message)
}

// Start starts the WebSocket hub
func (s *Server) Start(ctx context.Context) {
	go s.hub.Run(ctx)
}

// HandleWebSocket handles WebSocket upgrade requests
func (s *Server) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Error("WebSocket upgrade failed", zap.Error(err))
		return
	}

	// Generate client ID
	clientID := fmt.Sprintf("client_%d", time.Now().UnixNano())

	client := NewClient(clientID, conn, s.hub, s.logger)
	s.hub.register <- client

	// Start client goroutines
	go client.WritePump()
	go client.ReadPump()
}

// GetStats returns WebSocket server statistics
func (s *Server) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"connectedClients": s.hub.GetClientCount(),
		"config":           s.hub.config,
	}
}

// Event types for MCP integration
const (
	EventTypeSpecAdded     = "spec.added"
	EventTypeSpecUpdated   = "spec.updated"
	EventTypeSpecRemoved   = "spec.removed"
	EventTypeRequestMetric = "request.metric"
	EventTypeErrorOccurred = "error.occurred"
)

// MCPEventMessage creates a message for MCP events
func MCPEventMessage(eventType string, data map[string]interface{}) Message {
	return Message{
		Type: MessageTypeEvent,
		Data: map[string]interface{}{
			"eventType": eventType,
			"payload":   data,
		},
		Timestamp: time.Now(),
	}
}
