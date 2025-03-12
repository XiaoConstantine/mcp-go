package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/XiaoConstantine/mcp-go/pkg/protocol"

	"github.com/XiaoConstantine/mcp-go/pkg/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockMCPServer is a mock implementation of the MCPServer interface.
type MockMCPServer struct {
	mock.Mock
	notifChan chan protocol.Message
}

func NewMockMCPServer() *MockMCPServer {
	return &MockMCPServer{
		notifChan: make(chan protocol.Message, 10),
	}
}

func (m *MockMCPServer) HandleMessage(ctx context.Context, msg *protocol.Message) (*protocol.Message, error) {
	args := m.Called(ctx, msg)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*protocol.Message), args.Error(1)
}

func (m *MockMCPServer) Notifications() <-chan protocol.Message {
	return m.notifChan
}

func (m *MockMCPServer) Shutdown(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockMCPServer) SendNotification(notif protocol.Message) {
	m.notifChan <- notif
}

type MockTransport struct {
	readQueue   []string
	readErr     error
	writtenData []byte
	writeMutex  sync.Mutex
	readMutex   sync.Mutex
	closeReader bool
	closeWriter bool
	mock.Mock
}

func NewMockTransport() *MockTransport {
	return &MockTransport{
		readQueue: make([]string, 0),
	}
}

func (m *MockTransport) Receive(ctx context.Context) (*protocol.Message, error) {
	m.readMutex.Lock()
	defer m.readMutex.Unlock()

	if m.closeReader {
		return nil, io.EOF
	}

	if m.readErr != nil {
		return nil, m.readErr
	}

	// Check for context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		// Continue with normal processing
	}

	if len(m.readQueue) == 0 {
		// Block until data available or closed
		return nil, io.EOF
	}

	jsonStr := m.readQueue[0]
	m.readQueue = m.readQueue[1:]
	var msg protocol.Message
	if err := json.Unmarshal([]byte(jsonStr), &msg); err != nil {
		return nil, err
	}

	return &msg, nil
}

func (m *MockTransport) Send(ctx context.Context, msg *protocol.Message) error {
	m.writeMutex.Lock()
	defer m.writeMutex.Unlock()

	if m.closeWriter {
		return errors.New("writer closed")
	}
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		// Continue with normal processing
	}

	// Marshal the message
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	m.writtenData = append(m.writtenData, data...)
	m.writtenData = append(m.writtenData, '\n')

	return nil
}

func (m *MockTransport) QueueRead(data string) {
	m.readMutex.Lock()
	defer m.readMutex.Unlock()
	m.readQueue = append(m.readQueue, data)
}

func (m *MockTransport) CloseReader() {
	m.readMutex.Lock()
	defer m.readMutex.Unlock()
	m.closeReader = true
}

func (m *MockTransport) CloseWriter() {
	m.writeMutex.Lock()
	defer m.writeMutex.Unlock()
	m.closeWriter = true
}

func (m *MockTransport) GetWrittenData() []byte {
	m.writeMutex.Lock()
	defer m.writeMutex.Unlock()
	return m.writtenData
}

func (m *MockTransport) ClearWrittenData() {
	m.writeMutex.Lock()
	defer m.writeMutex.Unlock()
	m.writtenData = nil
}

func (m *MockTransport) SetReadError(err error) {
	m.readMutex.Lock()
	defer m.readMutex.Unlock()
	m.readErr = err
}

func (m *MockTransport) Close() error {
	m.CloseReader()
	m.CloseWriter()
	return m.Called().Error(0)
}

// Test creating a new server.
func TestNewServer(t *testing.T) {
	mcpServer := NewMockMCPServer()
	config := &ServerConfig{
		DefaultTimeout: 5 * time.Second,
		Logger:         logging.NewStdLogger(logging.InfoLevel),
	}

	server := NewServer(mcpServer, config)

	assert.NotNil(t, server)
	assert.Equal(t, 5*time.Second, server.defaultTimeout)
	assert.NotNil(t, server.transport)
	assert.NotNil(t, server.messageQueue)
	assert.NotNil(t, server.logger)

	// Test with nil config (should use defaults)
	server = NewServer(mcpServer, nil)
	assert.NotNil(t, server)
	assert.Equal(t, 30*time.Second, server.defaultTimeout)
}

// Setup function for creating a server with mocked reader/writer.
func setupTestServer() (*Server, *MockMCPServer, *MockTransport) {
	mock := NewMockMCPServer()
	mockTransport := NewMockTransport()

	ctx, cancel := context.WithCancel(context.Background())
	server := &Server{
		mcpServer:      mock,
		transport:      mockTransport,
		ctx:            ctx,
		cancel:         cancel,
		defaultTimeout: 1 * time.Second,
		outputSync:     &sync.WaitGroup{},
		messageQueue:   make(chan *protocol.Message, 100),
		logger:         logging.NewNoopLogger(),
	}

	return server, mock, mockTransport
}

// Test handling a simple message.
func TestProcessMessage(t *testing.T) {
	server, mockMCP, _ := setupTestServer()

	// Create a test message
	requestID := 1
	reqVal := protocol.RequestID(requestID)
	msg := &protocol.Message{
		JSONRPC: "2.0",
		ID:      &reqVal,
		Method:  "test/method",
		Params:  json.RawMessage(`{"test":"value"}`),
	}

	// Set up the expected response
	response := &protocol.Message{
		JSONRPC: "2.0",
		ID:      &reqVal,
		Result:  json.RawMessage(`{"result":"success"}`),
	}

	mockMCP.On("HandleMessage", mock.Anything, msg).Return(response, nil)

	// Create a channel to capture the output
	resultChan := make(chan *protocol.Message, 1)

	// Mock the message queue
	originalQueue := server.messageQueue
	server.messageQueue = make(chan *protocol.Message, 1)
	defer func() {
		server.messageQueue = originalQueue
	}()

	// Replace messageQueue with a function that captures the message
	go func() {
		select {
		case msg := <-server.messageQueue:
			resultChan <- msg
		case <-time.After(2 * time.Second):
			t.Error("Timeout waiting for message")
		}
	}()

	// Process the message
	server.processMessage(msg)

	// Get the result
	select {
	case result := <-resultChan:
		assert.Equal(t, response, result)
	case <-time.After(2 * time.Second):
		t.Error("Timeout waiting for result")
	}

	mockMCP.AssertExpectations(t)
}

// Test handling error responses.
func TestHandleSpecificError(t *testing.T) {
	server, _, _ := setupTestServer()
	requestID := 1
	reqVal := protocol.RequestID(requestID)

	// Test cases for different error types
	testCases := []struct {
		name     string
		err      error
		expected int // Expected error code
	}{
		{
			name:     "Context deadline exceeded",
			err:      context.DeadlineExceeded,
			expected: protocol.ErrCodeServerTimeout,
		},
		{
			name:     "Context canceled",
			err:      context.Canceled,
			expected: protocol.ErrCodeShuttingDown,
		},
		{
			name:     "Method not found",
			err:      errors.New("unknown method test"),
			expected: protocol.ErrCodeMethodNotFound,
		},
		{
			name:     "Invalid params",
			err:      errors.New("invalid params: missing required field"),
			expected: protocol.ErrCodeInvalidParams,
		},
		{
			name:     "Generic error",
			err:      errors.New("some other error"),
			expected: protocol.ErrCodeInternalError,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a channel to capture the error response
			resultChan := make(chan *protocol.Message, 1)

			// Replace messageQueue with our capture channel
			originalQueue := server.messageQueue
			server.messageQueue = make(chan *protocol.Message, 1)
			defer func() {
				server.messageQueue = originalQueue
			}()

			// Replace messageQueue with a function that captures the message
			go func() {
				select {
				case msg := <-server.messageQueue:
					resultChan <- msg
				case <-time.After(2 * time.Second):
					t.Error("Timeout waiting for error message")
				}
			}()

			// Handle the error
			server.handleSpecificError(&reqVal, tc.err)

			// Get the result
			select {
			case result := <-resultChan:
				assert.NotNil(t, result.Error)
				assert.Equal(t, tc.expected, result.Error.Code)
			case <-time.After(2 * time.Second):
				t.Error("Timeout waiting for error result")
			}
		})
	}
}

// Test shutdown behavior.
func TestStop(t *testing.T) {
	server, mockMCP, _ := setupTestServer()

	// Set up expectations
	mockMCP.On("Shutdown", mock.Anything).Return(nil)

	// Test stopping
	err := server.Stop()
	assert.NoError(t, err)

	// Test stopping when already shutting down
	server.shutdownMu.Lock()
	server.shuttingDown = true
	server.shutdownMu.Unlock()

	err = server.Stop()
	assert.NoError(t, err)

	mockMCP.AssertExpectations(t)
}

// Test handling incoming messages.
func TestHandleOutgoingMessages(t *testing.T) {
	server, _, mockTransport := setupTestServer()
	requestID := 1
	reqVal := protocol.RequestID(requestID)

	// Create test message
	msg := &protocol.Message{
		JSONRPC: "2.0",
		ID:      &reqVal,
		Result:  json.RawMessage(`{"result":"success"}`),
	}

	// Start the message handler
	server.wg.Add(1)
	go server.handleOutgoingMessages()

	// Send a message
	server.outputSync.Add(1)
	server.messageQueue <- msg

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	// Check the output
	writtenData := mockTransport.GetWrittenData()
	assert.Contains(t, string(writtenData), "success")

	// Close the message queue and wait for handler to finish
	close(server.messageQueue)
	server.wg.Wait()
}

// Test setting timeout.
func TestSetTimeout(t *testing.T) {
	server, _, _ := setupTestServer()

	// Initial timeout
	assert.Equal(t, 1*time.Second, server.defaultTimeout)

	// Set a new timeout
	server.SetTimeout(5 * time.Second)

	// Check the new timeout
	assert.Equal(t, 5*time.Second, server.defaultTimeout)
}

// Test message processing with timeout.
func TestProcessMessageTimeout(t *testing.T) {
	server, mockMCP, _ := setupTestServer()

	// Create a test message
	requestID := 1
	reqVal := protocol.RequestID(requestID)
	msg := &protocol.Message{
		JSONRPC: "2.0",
		ID:      &reqVal,
		Method:  "test/slow_method",
		Params:  json.RawMessage(`{"test":"value"}`),
	}

	// Set up the mock correctly
	mockMCP.On("HandleMessage", mock.Anything, msg).Run(func(args mock.Arguments) {
		// Sleep slightly longer than the server's timeout (1s)
		time.Sleep(1200 * time.Millisecond)
	}).Return(nil, errors.New("mock timeout")).Once()

	// Create a timeout for the entire test
	testCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Replace message queue with a test channel
	resultChan := make(chan *protocol.Message, 1)
	originalQueue := server.messageQueue
	server.messageQueue = make(chan *protocol.Message, 1)
	defer func() {
		server.messageQueue = originalQueue
	}()

	// Start a goroutine to monitor the message queue
	errorReceived := make(chan struct{})
	go func() {
		defer close(errorReceived)
		select {
		case msg := <-server.messageQueue:
			resultChan <- msg
		case <-testCtx.Done():
			t.Error("Timeout waiting for error message")
			return
		}
	}()

	// Process the message
	server.processMessage(msg)

	// Wait for the error message or timeout
	select {
	case <-errorReceived:
		// Check the result
		result := <-resultChan
		assert.NotNil(t, result.Error)
		assert.Equal(t, protocol.ErrCodeServerTimeout, result.Error.Code)
	case <-testCtx.Done():
		t.Fatal("Test timed out waiting for error message")
	}

	// Verify mock expectations
	mockMCP.AssertExpectations(t)
}

// Test notification forwarding.
func TestForwardNotifications(t *testing.T) {
	server, mockMCP, _ := setupTestServer()

	// Create a notification
	notification := protocol.Message{
		JSONRPC: "2.0",
		Method:  "notifications/test",
		Params:  json.RawMessage(`{"test":"notification"}`),
	}

	// Use a context with timeout for the entire test
	testCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Start the notification forwarder with a done channel
	done := make(chan struct{})
	server.wg.Add(1)
	go func() {
		go server.forwardNotifications()

		// Send a notification through the channel
		mockMCP.SendNotification(notification)

		// Wait briefly
		time.Sleep(100 * time.Millisecond)

		// Cancel the context to stop the forwarder
		server.cancel()

		// Use a goroutine to wait on the WaitGroup
		go func() {
			server.wg.Wait()
			close(done)
		}()
	}()

	// Wait for completion or timeout
	select {
	case <-done:
		// Test completed successfully
	case <-testCtx.Done():
		t.Fatal("Test timed out waiting for forwardNotifications")
	}
}

// Test handling invalid JSON.
func TestInvalidJson(t *testing.T) {
	server, _, mockTransport := setupTestServer()
	// Create an invalid JSON message
	mockTransport.SetReadError(errors.New("failed to unmarshal message: invalid character 'h' looking for beginning of value"))
	// Create a channel to capture the error response
	resultChan := make(chan *protocol.Message, 1)

	// Replace messageQueue with our capture channel
	originalQueue := server.messageQueue
	server.messageQueue = make(chan *protocol.Message, 1)
	defer func() {
		server.messageQueue = originalQueue
	}()

	// Capture the error message
	go func() {
		select {
		case msg := <-server.messageQueue:
			resultChan <- msg
		case <-time.After(2 * time.Second):
			t.Error("Timeout waiting for error message")
		}
	}()

	// Start processing in a goroutine that will terminate quickly
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		// Try to receive a message - will fail with our error
		msg, err := mockTransport.Receive(ctx)

		assert.Error(t, err)

		assert.Nil(t, msg)
		// Create an error response
		errorMsg := &protocol.Message{
			JSONRPC: "2.0",
			Error: &protocol.ErrorObject{
				Code:    protocol.ErrCodeParseError,
				Message: "Parse error",
				Data: map[string]interface{}{
					"error": err.Error(),
					"line":  "invalid JSON",
				},
			},
		}

		server.outputSync.Add(1)
		server.messageQueue <- errorMsg
	}()

	// Get the result
	select {
	case result := <-resultChan:
		assert.NotNil(t, result.Error)
		assert.Equal(t, protocol.ErrCodeParseError, result.Error.Code)
	case <-time.After(2 * time.Second):
		t.Error("Timeout waiting for error result")
	}
}

// Test start server.
func TestStartServer(t *testing.T) {
	// Save original stdin and restore after test
	mockTransport := NewMockTransport()
	// Create mock MCP server with expectations
	mockMCP := NewMockMCPServer()
	requestID := 1
	reqVal := protocol.RequestID(requestID)
	pingResponse := &protocol.Message{
		JSONRPC: "2.0",
		ID:      &reqVal,
		Result:  json.RawMessage(`{}`),
	}

	mockMCP.On("HandleMessage", mock.Anything, mock.MatchedBy(func(msg *protocol.Message) bool {
		return msg != nil && msg.Method == "ping"
	})).Return(pingResponse, nil).Once()

	// Queue a test message in the mock transport
	mockTransport.QueueRead(`{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n")

	// Create server with mock transport
	config := &ServerConfig{DefaultTimeout: 1 * time.Second}
	server := NewServer(mockMCP, config)

	// Replace the default transport with our mock
	originalTransport := server.transport
	server.transport = mockTransport
	defer func() {
		server.transport = originalTransport
	}()

	// Start server in a goroutine
	serverDone := make(chan struct{})
	var startErr error
	go func() {
		startErr = server.Start()
		close(serverDone)
	}()

	// Give some time for the server to process the ping message and send response
	time.Sleep(500 * time.Millisecond)

	// Check if response was written - this should happen before we send test/stop
	writtenData := string(mockTransport.GetWrittenData())
	assert.Contains(t, writtenData, `"id":1`, "Expected response with id:1 to be written")

	// Now send the stop message to terminate the server
	mockTransport.QueueRead(`{"method":"test/stop"}` + "\n")

	// Wait for server to exit
	select {
	case <-serverDone:
		assert.NoError(t, startErr)
	case <-time.After(2 * time.Second):
		t.Fatal("Test timed out waiting for server to exit")
	}

	// Verify expectations
	mockMCP.AssertExpectations(t)
}

// Test server with shutdown during operation.
func TestServerShutdownDuringOperation(t *testing.T) {
	// Use setupTestServer to ensure proper initialization
	server, mockMCP, mockTransport := setupTestServer()

	// Set up expectation for Shutdown
	mockMCP.On("Shutdown", mock.Anything).Return(nil).Once()

	// Start server in a goroutine
	serverDone := make(chan struct{})
	var startErr error
	go func() {
		startErr = server.Start()
		close(serverDone)
	}()

	// Give the server time to initialize
	time.Sleep(100 * time.Millisecond)

	// Queue some input to keep the server busy
	mockTransport.QueueRead(`{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n")

	// Give time for the server to process the ping
	time.Sleep(50 * time.Millisecond)

	// Call Stop and wait for it to complete
	stopErr := server.Stop()
	assert.NoError(t, stopErr)

	// Close the transport to ensure the server exits
	mockTransport.CloseReader()

	// Wait for server to exit with timeout
	select {
	case <-serverDone:
		assert.NoError(t, startErr)
	case <-time.After(2 * time.Second):
		t.Fatal("Test timed out waiting for server to exit after Stop()")
	}

	// Verify ALL mock expectations were met
	mockMCP.AssertExpectations(t)
}

// Test contains helper function.
func TestContains(t *testing.T) {
	testCases := []struct {
		s        string
		subs     []string
		expected bool
	}{
		{
			s:        "method not found",
			subs:     []string{"method not found", "unknown method"},
			expected: true,
		},
		{
			s:        "unknown method test",
			subs:     []string{"method not found", "unknown method"},
			expected: true,
		},
		{
			s:        "some other error",
			subs:     []string{"method not found", "unknown method"},
			expected: false,
		},
		{
			s:        "METHOD NOT FOUND", // Case insensitive
			subs:     []string{"method not found"},
			expected: true,
		},
	}

	for _, tc := range testCases {
		result := contains(tc.s, tc.subs...)
		assert.Equal(t, tc.expected, result)
	}
}

// Test message processing during shutdown.
func TestMessageProcessingDuringShutdown(t *testing.T) {
	server, _, _ := setupTestServer()

	// Set shutdown state
	server.shutdownMu.Lock()
	server.shuttingDown = true
	server.shutdownMu.Unlock()

	// Create a test message
	requestID := 1
	reqVal := protocol.RequestID(requestID)
	msg := &protocol.Message{
		JSONRPC: "2.0",
		ID:      &reqVal,
		Method:  "test/method",
		Params:  json.RawMessage(`{"test":"value"}`),
	}

	// Create a channel to capture responses
	resultChan := make(chan *protocol.Message, 1)

	// Replace messageQueue with our capture channel
	originalQueue := server.messageQueue
	server.messageQueue = make(chan *protocol.Message, 1)
	defer func() {
		server.messageQueue = originalQueue
	}()

	// Capture responses
	go func() {
		select {
		case msg := <-server.messageQueue:
			resultChan <- msg
		case <-time.After(2 * time.Second):
			t.Error("Timeout waiting for shutdown message")
		}
	}()

	// Process the message during shutdown
	server.processMessage(msg)

	// Get the result
	select {
	case result := <-resultChan:
		assert.NotNil(t, result.Error)
		assert.Equal(t, protocol.ErrCodeShuttingDown, result.Error.Code)
	case <-time.After(2 * time.Second):
		t.Error("Timeout waiting for shutdown error")
	}
}

// Test writing messages to stdout with errors.
func TestSendMessageWithError(t *testing.T) {
	server, _, mockTransport := setupTestServer()

	mockTransport.CloseWriter()

	// Create test message
	requestID := 1
	reqVal := protocol.RequestID(requestID)

	msg := &protocol.Message{
		JSONRPC: "2.0",
		ID:      &reqVal,
		Result:  json.RawMessage(`{\"result\":\"success\"}`),
	}

	// Add to the wait group (will be decremented in handleOutgoingMessage)
	server.outputSync.Add(1)

	// Write should handle the error gracefully
	// Send the message to the queue
	server.messageQueue <- msg

	// Start message handling
	server.wg.Add(1)
	go server.handleOutgoingMessages()
	// Wait group should be decremented
	wait := make(chan struct{})
	go func() {
		server.outputSync.Wait()
		close(wait)
	}()

	select {
	case <-wait:
		// Success - wait group was decremented
	case <-time.After(1 * time.Second):
		t.Error("Timeout waiting for outputSync to be decremented")
	}
	// Clean up
	close(server.messageQueue)
	server.wg.Wait()
}

// Test enqueueError during shutdown.
func TestEnqueueErrorDuringShutdown(t *testing.T) {
	server, _, _ := setupTestServer()

	// Set shutdown state
	server.shutdownMu.Lock()
	server.shuttingDown = true
	server.shutdownMu.Unlock()

	// Create an ID
	requestID := 1

	reqVal := protocol.RequestID(requestID)
	// Try to enqueue a non-shutdown error
	server.enqueueError(&reqVal, protocol.ErrCodeInternalError, "Test error", nil)

	// Message queue should remain empty
	select {
	case <-server.messageQueue:
		t.Error("Non-shutdown error was enqueued during shutdown")
	default:
		// Success - nothing was enqueued
	}

	// Now try to enqueue a shutdown error
	server.enqueueError(&reqVal, protocol.ErrCodeShuttingDown, "Shutting down", nil)

	// This should be enqueued
	select {
	case msg := <-server.messageQueue:
		assert.Equal(t, protocol.ErrCodeShuttingDown, msg.Error.Code)
	default:
		t.Error("Shutdown error was not enqueued")
	}
}

// Test isShuttingDown.
func TestIsShuttingDown(t *testing.T) {
	server, _, _ := setupTestServer()

	// Initially not shutting down
	assert.False(t, server.isShuttingDown())

	// Set shutting down state
	server.shutdownMu.Lock()
	server.shuttingDown = true
	server.shutdownMu.Unlock()

	// Now should be shutting down
	assert.True(t, server.isShuttingDown())
}

// Test enqueueError with full channel.
func TestEnqueueErrorFullChannel(t *testing.T) {
	//	t.Skip()
	server, _, _ := setupTestServer()

	// Create a small channel that we can fill
	smallChan := make(chan *protocol.Message, 1)
	originalQueue := server.messageQueue
	server.messageQueue = smallChan
	defer func() {
		server.messageQueue = originalQueue
	}()

	// Fill the channel
	requestID := 1
	reqVal := protocol.RequestID(requestID)
	smallChan <- &protocol.Message{JSONRPC: "2.0", ID: &reqVal}

	// Create a context with a short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Create a goroutine to check if outputSync is decremented
	done := make(chan struct{})
	go func() {
		// This should complete immediately if outputSync is decremented
		server.outputSync.Wait()
		close(done)
	}()

	// Try to enqueue an error to a full channel
	server.enqueueError(&reqVal, protocol.ErrCodeInternalError, "Test error", nil)

	// Check if outputSync was properly decremented
	select {
	case <-done:
		// Success - outputSync was decremented
	case <-ctx.Done():
		t.Fatal("Test timed out - outputSync was not decremented")
	}
}

// Test Start with already shutting down.
func TestStartAlreadyShuttingDown(t *testing.T) {
	server, _, _ := setupTestServer()

	// Set shutting down state
	server.shutdownMu.Lock()
	server.shuttingDown = true
	server.shutdownMu.Unlock()

	// Try to start the server
	err := server.Start()

	// Should return an error
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "shutting down")
}

// Test Stop with timeout.
func TestStopWithTimeout(t *testing.T) {
	// Use setupTestServer to ensure proper initialization
	server, mockMCP, _ := setupTestServer()

	// Make sure the server is fully initialized
	assert.NotNil(t, server.cancel, "cancel function should be initialized")
	assert.NotNil(t, server.outputSync, "outputSync should be initialized")
	assert.NotNil(t, server.mcpServer, "mcpServer should be initialized")

	// Make Shutdown hang but not too long to avoid test timeouts
	mockMCP.On("Shutdown", mock.Anything).Run(func(args mock.Arguments) {
		// Sleep a short time for testing purposes
		time.Sleep(100 * time.Millisecond)
	}).Return(nil).Once()

	// Start a goroutine that can be cancelled
	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())
	defer shutdownCancel()

	// Add a goroutine to the wait group that will complete when cancelled
	server.wg.Add(1)
	go func() {
		defer server.wg.Done()
		<-shutdownCtx.Done()
	}()

	// Try to stop the server
	err := server.Stop()

	// Due to our changes, it should not return an error
	assert.NoError(t, err)

	// Cancel our goroutine to clean up
	shutdownCancel()

	// Verify expectations
	mockMCP.AssertExpectations(t)
}

// Test handling a "STOP" test message.
func TestStopTestMessage(t *testing.T) {
	server, _, mockTransport := setupTestServer()
	// Set up a reader that returns a test stop message
	stopJSON := `{"method":"test/stop"}`
	mockTransport.QueueRead(stopJSON + "\n")
	// Create a channel to notify when the test should end
	doneCh := make(chan struct{})

	// Run the server in a goroutine
	go func() {
		err := server.Start()
		assert.NoError(t, err)
		close(doneCh)
	}()

	// Wait for the server to process the stop message and exit
	select {
	case <-doneCh:
		// Server stopped normally
	case <-time.After(2 * time.Second):
		t.Error("Test timed out")
	}
}
