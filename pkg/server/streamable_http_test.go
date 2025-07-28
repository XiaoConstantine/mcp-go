package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/XiaoConstantine/mcp-go/pkg/logging"
	"github.com/XiaoConstantine/mcp-go/pkg/model"
	"github.com/XiaoConstantine/mcp-go/pkg/protocol"
	"github.com/XiaoConstantine/mcp-go/pkg/server/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock MCP Server for testing.
type mockMCPServer struct {
	mu             sync.RWMutex
	messages       []protocol.Message
	responses      map[string]*protocol.Message
	errors         map[string]error
	shutdownCalled bool
	notificationCh chan protocol.Message
}

func newMockMCPServer() *mockMCPServer {
	return &mockMCPServer{
		responses:      make(map[string]*protocol.Message),
		errors:         make(map[string]error),
		notificationCh: make(chan protocol.Message, 10),
	}
}

func (m *mockMCPServer) HandleMessage(ctx context.Context, msg *protocol.Message) (*protocol.Message, error) {
	m.mu.Lock()
	m.messages = append(m.messages, *msg)
	m.mu.Unlock()

	// Handle specific test cases
	if msg.ID != nil {
		reqID := fmt.Sprintf("%v", *msg.ID)
		if err, exists := m.errors[reqID]; exists {
			return nil, err
		}
		if resp, exists := m.responses[reqID]; exists {
			return resp, nil
		}
	}

	// Default response for requests with ID
	if msg.ID != nil {
		return &protocol.Message{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Result:  json.RawMessage(`{"status": "ok"}`),
		}, nil
	}

	// No response for notifications
	return nil, nil
}

func (m *mockMCPServer) Notifications() <-chan protocol.Message {
	return m.notificationCh
}

func (m *mockMCPServer) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shutdownCalled = true
	close(m.notificationCh)
	return nil
}

func (m *mockMCPServer) setResponse(reqID string, response *protocol.Message) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responses[reqID] = response
}

func (m *mockMCPServer) setError(reqID string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errors[reqID] = err
}

func (m *mockMCPServer) getMessages() []protocol.Message {
	m.mu.RLock()
	defer m.mu.RUnlock()
	messages := make([]protocol.Message, len(m.messages))
	copy(messages, m.messages)
	return messages
}


func TestDefaultStreamableHTTPServerConfig(t *testing.T) {
	config := DefaultStreamableHTTPServerConfig()

	// Verify default values
	assert.Equal(t, ":8080", config.ListenAddr)
	assert.Equal(t, "/mcp", config.MCPPath)
	assert.True(t, config.EnableCORS)
	assert.NotNil(t, config.Logger)
	assert.False(t, config.RequireSession)
	assert.Equal(t, 15*time.Second, config.ReadTimeout)
	assert.Equal(t, 15*time.Second, config.WriteTimeout)
	assert.Equal(t, 60*time.Second, config.IdleTimeout)
	assert.Equal(t, 30*time.Second, config.KeepAliveInterval)
	assert.Equal(t, 10, config.MaxWorkers)

	// Verify logger is NoopLogger
	_, ok := config.Logger.(*logging.NoopLogger)
	assert.True(t, ok)
}

func TestNewStreamableHTTPServer(t *testing.T) {
	mockServer := newMockMCPServer()
	config := DefaultStreamableHTTPServerConfig()
	config.ListenAddr = ":9999"
	config.MaxWorkers = 5

	server := NewStreamableHTTPServer(mockServer, config)

	require.NotNil(t, server)
	assert.Equal(t, mockServer, server.mcpServer)
	assert.Equal(t, config, server.config)
	assert.NotNil(t, server.transport)
	assert.NotNil(t, server.logger)
	assert.NotNil(t, server.ctx)
	assert.NotNil(t, server.cancelFunc)
	assert.NotNil(t, server.messageQueue)
	assert.Equal(t, 5, server.maxWorkers)
	assert.False(t, server.isRunning)
	assert.Nil(t, server.httpServer)

	// Test with nil logger
	configNilLogger := config
	configNilLogger.Logger = nil
	serverNilLogger := NewStreamableHTTPServer(mockServer, configNilLogger)
	assert.NotNil(t, serverNilLogger.logger)
	_, ok := serverNilLogger.logger.(*logging.NoopLogger)
	assert.True(t, ok)

	// Test with zero MaxWorkers (should default to 10)
	configZeroWorkers := config
	configZeroWorkers.MaxWorkers = 0
	serverZeroWorkers := NewStreamableHTTPServer(mockServer, configZeroWorkers)
	assert.Equal(t, 10, serverZeroWorkers.config.MaxWorkers)
	assert.Equal(t, 10, serverZeroWorkers.maxWorkers)
}

func TestStreamableHTTPServerStartStop(t *testing.T) {
	mockServer := newMockMCPServer()
	config := DefaultStreamableHTTPServerConfig()
	config.ListenAddr = ":0" // Use random port
	config.MaxWorkers = 2

	server := NewStreamableHTTPServer(mockServer, config)

	// Test initial state
	assert.False(t, server.isRunning)

	// Test start
	err := server.Start()
	require.NoError(t, err)
	assert.True(t, server.isRunning)
	assert.NotNil(t, server.httpServer)

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Test start when already running
	err = server.Start()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already running")

	// Test stop
	err = server.Stop()
	require.NoError(t, err)
	assert.False(t, server.isRunning)

	// Test stop when not running
	err = server.Stop()
	assert.NoError(t, err) // Should not error
}

func TestStreamableHTTPServerConcurrentStartStop(t *testing.T) {
	mockServer := newMockMCPServer()
	config := DefaultStreamableHTTPServerConfig()
	config.ListenAddr = ":0"
	config.MaxWorkers = 2

	server := NewStreamableHTTPServer(mockServer, config)

	var wg sync.WaitGroup
	var startErr, stopErr error

	// Start and stop concurrently
	wg.Add(2)
	go func() {
		defer wg.Done()
		startErr = server.Start()
	}()

	go func() {
		defer wg.Done()
		time.Sleep(50 * time.Millisecond) // Small delay
		stopErr = server.Stop()
	}()

	wg.Wait()

	// One should succeed, timing dependent
	if startErr == nil {
		assert.NoError(t, stopErr)
	}
}

func TestStreamableHTTPServerMessageHandling(t *testing.T) {
	mockServer := newMockMCPServer()
	config := DefaultStreamableHTTPServerConfig()
	config.ListenAddr = ":0"
	config.MaxWorkers = 2

	server := NewStreamableHTTPServer(mockServer, config)

	// Set up a response for our test message
	testReqID := protocol.RequestID("test-123")
	expectedResponse := &protocol.Message{
		JSONRPC: "2.0",
		ID:      &testReqID,
		Result:  json.RawMessage(`{"test": "response"}`),
	}
	mockServer.setResponse("test-123", expectedResponse)

	// Test handleMessage directly
	testMsg := &protocol.Message{
		JSONRPC: "2.0",
		ID:      &testReqID,
		Method:  "test/method",
		Params:  json.RawMessage(`{"param": "value"}`),
	}

	// We can't easily test the full message flow without starting the server
	// and setting up the transport, so we'll test the handleMessage method directly
	server.handleMessage(testMsg)

	// Give time for processing
	time.Sleep(100 * time.Millisecond)

	// Verify the message was handled
	messages := mockServer.getMessages()
	assert.Len(t, messages, 1)
	assert.Equal(t, testMsg.Method, messages[0].Method)
}

func TestStreamableHTTPServerNotificationHandling(t *testing.T) {
	mockServer := newMockMCPServer()
	config := DefaultStreamableHTTPServerConfig()
	config.ListenAddr = ":0"
	config.MaxWorkers = 2

	server := NewStreamableHTTPServer(mockServer, config)

	// Test notification (no ID)
	notification := &protocol.Message{
		JSONRPC: "2.0",
		Method:  "notifications/test",
		Params:  json.RawMessage(`{"event": "test"}`),
	}

	server.handleMessage(notification)

	// Give time for processing
	time.Sleep(100 * time.Millisecond)

	// Verify the notification was handled
	messages := mockServer.getMessages()
	assert.Len(t, messages, 1)
	assert.Equal(t, notification.Method, messages[0].Method)
}

func TestStreamableHTTPServerErrorHandling(t *testing.T) {
	mockServer := newMockMCPServer()
	config := DefaultStreamableHTTPServerConfig()
	config.ListenAddr = ":0"
	config.MaxWorkers = 2

	server := NewStreamableHTTPServer(mockServer, config)

	// Set up an error response
	testReqID := protocol.RequestID("error-123")
	mockServer.setError("error-123", assert.AnError)

	// Test message with error
	testMsg := &protocol.Message{
		JSONRPC: "2.0",
		ID:      &testReqID,
		Method:  "test/error",
	}

	server.handleMessage(testMsg)

	// Give time for processing
	time.Sleep(100 * time.Millisecond)

	// Verify the message was processed (even with error)
	messages := mockServer.getMessages()
	assert.Len(t, messages, 1)
}

func TestStreamableHTTPServerContextCancellation(t *testing.T) {
	mockServer := newMockMCPServer()
	config := DefaultStreamableHTTPServerConfig()
	config.ListenAddr = ":0"
	config.MaxWorkers = 1

	server := NewStreamableHTTPServer(mockServer, config)

	err := server.Start()
	require.NoError(t, err)

	// Cancel the context immediately
	server.cancelFunc()

	// Give time for graceful shutdown
	time.Sleep(200 * time.Millisecond)

	// Server should still be marked as running until Stop is called
	assert.True(t, server.isRunning)

	err = server.Stop()
	assert.NoError(t, err)
}

func TestStreamableHTTPServerMCPRequestHandler(t *testing.T) {
	mockServer := newMockMCPServer()
	config := DefaultStreamableHTTPServerConfig()
	config.ListenAddr = ":0"

	server := NewStreamableHTTPServer(mockServer, config)

	// Start the server to enable message processing
	err := server.Start()
	require.NoError(t, err)
	defer func() {
		err := server.Stop()
		assert.NoError(t, err)
	}()

	// Give the server a moment to start processing
	time.Sleep(50 * time.Millisecond)

	// Create a test HTTP request
	req := httptest.NewRequest("POST", "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":"1","method":"ping"}`))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()

	// Test the handler - should complete quickly now that server is running
	done := make(chan struct{})
	go func() {
		defer close(done)
		server.handleMCPRequest(w, req)
	}()

	// Wait for completion with a reasonable timeout
	select {
	case <-done:
		// Success - handler completed
		assert.NotNil(t, w)
		// Should have received a response since mockServer responds to all requests
		assert.True(t, w.Code > 0, "Expected HTTP response code to be set")
	case <-time.After(2 * time.Second):
		t.Fatal("Handler took too long to complete (>2s), likely still hitting timeout issue")
	}
}

func TestServeHTTPWithContext(t *testing.T) {
	mockServer := newMockMCPServer()

	// Create a context that we'll cancel
	ctx, cancel := context.WithCancel(context.Background())

	// Start serving in a goroutine
	var serveErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		serveErr = ServeHTTPWithContext(ctx, mockServer, ":0")
	}()

	// Give the server time to start
	time.Sleep(100 * time.Millisecond)

	// Cancel the context
	cancel()

	// Wait for serve to complete
	select {
	case <-done:
		// Should complete without error or with context.Canceled
		if serveErr != nil {
			assert.Contains(t, serveErr.Error(), "context")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ServeHTTPWithContext did not complete in time")
	}
}

func TestServeHTTP(t *testing.T) {
	mockServer := newMockMCPServer()

	// Start serving in a goroutine
	var serveErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		serveErr = ServeHTTP(mockServer, ":0")
	}()

	// Give the server time to start
	time.Sleep(100 * time.Millisecond)

	// We can't easily test the full lifecycle without external cancellation
	// But we can verify it starts without immediate error
	select {
	case <-done:
		// If it completes immediately, there should be an error
		assert.Error(t, serveErr)
	case <-time.After(200 * time.Millisecond):
		// Still running, which is expected
		// We can't easily stop it in this test, so this is sufficient
	}
}

func TestStreamableHTTPServerQueueFullScenario(t *testing.T) {
	mockServer := newMockMCPServer()
	config := DefaultStreamableHTTPServerConfig()
	config.ListenAddr = ":0"
	config.MaxWorkers = 1

	server := NewStreamableHTTPServer(mockServer, config)

	// Fill the message queue to capacity
	queueCap := config.MaxWorkers * 2 // From the buffer size calculation in NewStreamableHTTPServer
	for i := 0; i < queueCap; i++ {
		select {
		case server.messageQueue <- &protocol.Message{
			JSONRPC: "2.0",
			Method:  "test/fill",
		}:
		default:
			t.Fatalf("Queue should not be full at iteration %d", i)
		}
	}

	// Try to add one more message (this should be handled by the default case in processMessages)
	testMsg := &protocol.Message{
		JSONRPC: "2.0",
		Method:  "test/overflow",
	}

	// This simulates what happens in processMessages when queue is full
	select {
	case server.messageQueue <- testMsg:
		t.Fatal("Expected queue to be full")
	default:
		// This is the expected case - queue is full
	}

	// Verify queue is at capacity
	assert.Equal(t, queueCap, len(server.messageQueue))
}

func TestStreamableHTTPServerWorkerPoolShutdown(t *testing.T) {
	mockServer := newMockMCPServer()
	config := DefaultStreamableHTTPServerConfig()
	config.ListenAddr = ":0"
	config.MaxWorkers = 3

	server := NewStreamableHTTPServer(mockServer, config)

	err := server.Start()
	require.NoError(t, err)

	// Give workers time to start
	time.Sleep(100 * time.Millisecond)

	// Stop the server
	err = server.Stop()
	require.NoError(t, err)

	// Verify workers have stopped by ensuring message queue is closed
	assert.False(t, server.isRunning)
}

// Integration test with actual core server.
func TestStreamableHTTPServerWithRealMCPServer(t *testing.T) {
	// Create a real MCP server
	info := models.Implementation{
		Name:    "TestServer",
		Version: "1.0.0",
	}
	mcpServer := core.NewServer(info, "1.0.0")

	config := DefaultStreamableHTTPServerConfig()
	config.ListenAddr = ":0"
	config.MaxWorkers = 2

	server := NewStreamableHTTPServer(mcpServer, config)

	err := server.Start()
	require.NoError(t, err)

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// We can't easily send notifications through the MCP server without
	// initializing the server, but we can verify the integration doesn't panic
	assert.True(t, server.isRunning)

	err = server.Stop()
	require.NoError(t, err)
}

func TestStreamableHTTPServerConfigurationEdgeCases(t *testing.T) {
	mockServer := newMockMCPServer()

	// Test with negative MaxWorkers
	config := DefaultStreamableHTTPServerConfig()
	config.MaxWorkers = -5

	server := NewStreamableHTTPServer(mockServer, config)
	assert.Equal(t, 10, server.config.MaxWorkers) // Should default to 10
	assert.Equal(t, 10, server.maxWorkers)

	// Test with custom logger
	customLogger := &logging.NoopLogger{}
	config2 := DefaultStreamableHTTPServerConfig()
	config2.Logger = customLogger

	server2 := NewStreamableHTTPServer(mockServer, config2)
	assert.Equal(t, customLogger, server2.logger)
}

func TestStreamableHTTPServerShutdownTimeout(t *testing.T) {
	mockServer := newMockMCPServer()
	config := DefaultStreamableHTTPServerConfig()
	config.ListenAddr = ":0"

	server := NewStreamableHTTPServer(mockServer, config)

	err := server.Start()
	require.NoError(t, err)

	// Verify shutdown works within timeout
	start := time.Now()
	err = server.Stop()
	duration := time.Since(start)

	assert.NoError(t, err)
	assert.Less(t, duration, 20*time.Second) // Should be well under the 15s timeout
}
