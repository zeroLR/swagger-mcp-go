package websocket

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestHub_BasicFunctionality(t *testing.T) {
	logger := zap.NewNop()
	config := Config{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     true,
		PingInterval:    10 * time.Second,
		PongWait:        15 * time.Second,
		WriteWait:       5 * time.Second,
		MaxMessageSize:  512,
	}

	hub := NewHub(config, logger)

	// Test initial state
	if count := hub.GetClientCount(); count != 0 {
		t.Errorf("Expected 0 clients, got %d", count)
	}

	// Test message handler registration
	hub.RegisterHandler("test-message", func(client *Client, message Message) error {
		return nil
	})

	// Test default config values
	if hub.config.ReadBufferSize != 1024 {
		t.Errorf("Expected ReadBufferSize 1024, got %d", hub.config.ReadBufferSize)
	}
}

func TestMessage(t *testing.T) {
	// Test message creation
	message := Message{
		Type: MessageTypeEvent,
		ID:   "test-id",
		Data: map[string]interface{}{
			"test": "data",
		},
		Timestamp: time.Now(),
	}

	if message.Type != MessageTypeEvent {
		t.Errorf("Expected event type")
	}
	if message.ID != "test-id" {
		t.Errorf("Expected test-id")
	}
	if message.Data["test"] != "data" {
		t.Errorf("Expected test data")
	}
}

func TestClient_Subscriptions(t *testing.T) {
	logger := zap.NewNop()
	config := Config{}
	hub := NewHub(config, logger)

	// Create a client with nil connection for testing subscriptions only
	client := &Client{
		ID:            "test-client",
		conn:          nil, // We're only testing subscription logic
		send:          make(chan Message, 256),
		hub:           hub,
		subscriptions: make(map[string]bool),
		logger:        logger.Named("client").With(zap.String("clientId", "test-client")),
	}

	// Test subscription
	client.Subscribe("test-topic")
	if !client.IsSubscribed("test-topic") {
		t.Errorf("Client should be subscribed to test-topic")
	}

	// Test unsubscription
	client.Unsubscribe("test-topic")
	if client.IsSubscribed("test-topic") {
		t.Errorf("Client should not be subscribed to test-topic after unsubscribe")
	}

	// Test multiple subscriptions
	client.Subscribe("topic1")
	client.Subscribe("topic2")
	if !client.IsSubscribed("topic1") || !client.IsSubscribed("topic2") {
		t.Errorf("Client should be subscribed to both topics")
	}
}

func TestDefaultHandlers(t *testing.T) {
	logger := zap.NewNop()
	config := Config{}
	hub := NewHub(config, logger)

	// Create a test client for handler testing
	client := &Client{
		ID:            "test-client",
		conn:          nil,
		send:          make(chan Message, 256),
		hub:           hub,
		subscriptions: make(map[string]bool),
		logger:        logger.Named("client").With(zap.String("clientId", "test-client")),
	}

	// Test ping handler
	pingMessage := Message{
		Type: MessageTypePing,
		ID:   "ping-id",
	}

	err := hub.handlePing(client, pingMessage)
	if err != nil {
		t.Errorf("Ping handler should not return error: %v", err)
	}

	// Check pong response
	select {
	case msg := <-client.send:
		if msg.Type != MessageTypePong {
			t.Errorf("Expected pong message, got %s", msg.Type)
		}
		if msg.ID != "ping-id" {
			t.Errorf("Expected pong message with same ID")
		}
	case <-time.After(100 * time.Millisecond):
		t.Errorf("Expected pong message to be sent")
	}

	// Test subscribe handler
	subscribeMessage := Message{
		Type: MessageTypeSubscribe,
		ID:   "sub-id",
		Data: map[string]interface{}{
			"topic": "test-topic",
		},
	}

	err = hub.handleSubscribe(client, subscribeMessage)
	if err != nil {
		t.Errorf("Subscribe handler should not return error: %v", err)
	}

	if !client.IsSubscribed("test-topic") {
		t.Errorf("Client should be subscribed to test-topic")
	}

	// Check subscribe response
	select {
	case msg := <-client.send:
		if msg.Type != MessageTypeResponse {
			t.Errorf("Expected response message, got %s", msg.Type)
		}
	case <-time.After(100 * time.Millisecond):
		t.Errorf("Expected response message to be sent")
	}

	// Test unsubscribe handler
	unsubscribeMessage := Message{
		Type: MessageTypeUnsubscribe,
		ID:   "unsub-id",
		Data: map[string]interface{}{
			"topic": "test-topic",
		},
	}

	err = hub.handleUnsubscribe(client, unsubscribeMessage)
	if err != nil {
		t.Errorf("Unsubscribe handler should not return error: %v", err)
	}

	if client.IsSubscribed("test-topic") {
		t.Errorf("Client should not be subscribed to test-topic after unsubscribe")
	}

	// Test subscribe with missing topic
	invalidSubscribe := Message{
		Type: MessageTypeSubscribe,
		ID:   "invalid-sub",
		Data: map[string]interface{}{
			"notTopic": "test-topic",
		},
	}

	err = hub.handleSubscribe(client, invalidSubscribe)
	if err == nil {
		t.Errorf("Subscribe handler should return error for missing topic")
	}
}

func TestMessageHandling(t *testing.T) {
	logger := zap.NewNop()
	config := Config{}
	hub := NewHub(config, logger)

	client := &Client{
		ID:            "test-client",
		conn:          nil,
		send:          make(chan Message, 256),
		hub:           hub,
		subscriptions: make(map[string]bool),
		logger:        logger.Named("client").With(zap.String("clientId", "test-client")),
	}

	// Test unknown message type
	unknownMessage := Message{
		Type: "unknown-type",
		ID:   "test-id",
	}

	hub.handleMessage(client, unknownMessage)

	// Check if error message was sent
	select {
	case msg := <-client.send:
		if msg.Type != MessageTypeError {
			t.Errorf("Expected error message, got %s", msg.Type)
		}
		if msg.ID != "test-id" {
			t.Errorf("Expected error message with same ID")
		}
	case <-time.After(100 * time.Millisecond):
		t.Errorf("Expected error message to be sent")
	}

	// Test custom handler
	handlerCalled := false
	hub.RegisterHandler("custom-type", func(client *Client, message Message) error {
		handlerCalled = true
		return nil
	})

	customMessage := Message{
		Type: "custom-type",
		Data: map[string]interface{}{"test": "data"},
	}

	hub.handleMessage(client, customMessage)

	if !handlerCalled {
		t.Errorf("Custom handler should have been called")
	}
}

func TestServer(t *testing.T) {
	logger := zap.NewNop()
	config := Config{
		CheckOrigin: true,
	}

	server := NewServer(config, logger)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server.Start(ctx)

	// Test handler registration
	server.RegisterHandler("custom", func(client *Client, message Message) error {
		return nil
	})

	// Test stats
	stats := server.GetStats()
	if stats["connectedClients"].(int) != 0 {
		t.Errorf("Expected 0 connected clients")
	}

	// Test broadcast
	message := Message{
		Type: MessageTypeEvent,
		Data: map[string]interface{}{"test": "broadcast"},
	}

	server.Broadcast("test-topic", message)
}

func TestMCPEventMessage(t *testing.T) {
	data := map[string]interface{}{
		"specId": "test-spec",
		"name":   "Test Spec",
	}

	message := MCPEventMessage(EventTypeSpecAdded, data)

	if message.Type != MessageTypeEvent {
		t.Errorf("Expected event message type")
	}

	eventType, ok := message.Data["eventType"].(string)
	if !ok || eventType != EventTypeSpecAdded {
		t.Errorf("Expected event type to be %s", EventTypeSpecAdded)
	}

	payload, ok := message.Data["payload"].(map[string]interface{})
	if !ok {
		t.Errorf("Expected payload in message data")
	}

	if payload["specId"] != "test-spec" {
		t.Errorf("Expected specId in payload")
	}
}

func TestConfig(t *testing.T) {
	logger := zap.NewNop()
	
	// Test with default config
	emptyConfig := Config{}
	hub := NewHub(emptyConfig, logger)

	// Check that defaults were set
	if hub.config.ReadBufferSize != 1024 {
		t.Errorf("Expected default ReadBufferSize 1024, got %d", hub.config.ReadBufferSize)
	}
	if hub.config.WriteBufferSize != 1024 {
		t.Errorf("Expected default WriteBufferSize 1024, got %d", hub.config.WriteBufferSize)
	}
	if hub.config.PingInterval != 54*time.Second {
		t.Errorf("Expected default PingInterval 54s, got %v", hub.config.PingInterval)
	}
	if hub.config.PongWait != 60*time.Second {
		t.Errorf("Expected default PongWait 60s, got %v", hub.config.PongWait)
	}

	// Test with custom config
	customConfig := Config{
		ReadBufferSize:  2048,
		WriteBufferSize: 2048,
		PingInterval:    30 * time.Second,
		PongWait:        45 * time.Second,
	}

	customHub := NewHub(customConfig, logger)
	if customHub.config.ReadBufferSize != 2048 {
		t.Errorf("Expected custom ReadBufferSize 2048, got %d", customHub.config.ReadBufferSize)
	}
	if customHub.config.PingInterval != 30*time.Second {
		t.Errorf("Expected custom PingInterval 30s, got %v", customHub.config.PingInterval)
	}
}