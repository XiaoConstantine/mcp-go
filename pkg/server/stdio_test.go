package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/XiaoConstantine/mcp-go/pkg/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockCoreServer is a mock implementation of the core.Server interface.
type MockCoreServer struct {
	mock.Mock
	notificationCh chan protocol.Message
}

func NewMockCoreServer() *MockCoreServer {
	return &MockCoreServer{
		notificationCh: make(chan protocol.Message, 10),
	}
}

func (m *MockCoreServer) HandleMessage(ctx context.Context, msg *protocol.Message) (*protocol.Message, error) {
	args := m.Called(ctx, msg)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*protocol.Message), args.Error(1)
}

func (m *MockCoreServer) Notifications() <-chan protocol.Message {
	return m.notificationCh
}

func (m *MockCoreServer) SendNotification(notif protocol.Message) {
	m.notificationCh <- notif
}

func (m *MockCoreServer) Close() {
	close(m.notificationCh)
}

type SafeBuffer struct {
	buffer *bytes.Buffer
	mu     sync.Mutex
}

func NewSafeBuffer() *SafeBuffer {
	return &SafeBuffer{buffer: &bytes.Buffer{}}
}

func (sb *SafeBuffer) Write(p []byte) (n int, err error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buffer.Write(p)
}

func (sb *SafeBuffer) String() string {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buffer.String()
}

// customReader implements a reader that can be controlled for testing.
type customReader struct {
	buf    bytes.Buffer
	closed bool
	mu     sync.Mutex
	cond   *sync.Cond
}

func newCustomReader() *customReader {
	cr := &customReader{}
	cr.cond = sync.NewCond(&cr.mu)
	return cr
}

func (r *customReader) Read(p []byte) (n int, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// If closed and no data available, return EOF immediately.
	if r.closed && r.buf.Len() == 0 {
		return 0, io.EOF
	}

	// Wait until data is available or the reader is closed.
	for r.buf.Len() == 0 && !r.closed {
		r.cond.Wait()
	}

	// Read data from the internal buffer into p.
	return r.buf.Read(p)
}

func (r *customReader) WriteString(s string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf.WriteString(s)
	r.cond.Signal()
}

func (r *customReader) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	r.cond.Broadcast()
}

// Test helpers.
func createJSONRPCRequest(id interface{}, method string, params interface{}) string {
	req := protocol.Message{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}

	if id != nil {
		reqID := protocol.RequestID(id)
		req.ID = &reqID
	}

	bytes, _ := json.Marshal(req)
	return string(bytes) + "\n"
}

func waitForOutput(buf *SafeBuffer, contains string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if strings.Contains(buf.String(), contains) {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// Setup function to create a testable server with custom IO.
func setupTestServer(t *testing.T) (*Server, *MockCoreServer, *customReader, *SafeBuffer) {
	mockCore := NewMockCoreServer()

	// Create custom reader and writer
	customReader := newCustomReader()
	outputBuf := NewSafeBuffer()

	// Create server with short timeout for tests
	config := &ServerConfig{
		DefaultTimeout: 100 * time.Millisecond,
	}
	server := NewServer(mockCore, config)

	// Replace stdin/stdout with our test buffers
	server.reader = bufio.NewReader(customReader)
	server.writer = bufio.NewWriterSize(outputBuf, 1)

	return server, mockCore, customReader, outputBuf
}

// Test successful processing of a request.
func TestServer_ProcessRequest(t *testing.T) {
	server, mockCore, customReader, outputBuf := setupTestServer(t)

	// Setup expected response
	requestID := 123
	reqIDVal := protocol.RequestID(requestID)
	expectedResponse := &protocol.Message{
		JSONRPC: "2.0",
		ID:      &reqIDVal,
		Result:  json.RawMessage(`{"success":true}`),
	}

	// Configure mock to return expected response
	mockCore.On("HandleMessage", mock.Anything, mock.MatchedBy(func(msg *protocol.Message) bool {
		return msg.Method == "test/method"
	})).Return(expectedResponse, nil)

	// Create test request
	requestJSON := createJSONRPCRequest(requestID, "test/method", map[string]string{"param": "value"})

	// Start server in a goroutine
	var serverErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		serverErr = server.Start()
	}()

	// Wait for server to start
	time.Sleep(50 * time.Millisecond)

	// Write request to input
	customReader.WriteString(requestJSON)

	// Give server time to process
	time.Sleep(100 * time.Millisecond)
	customReader.Close()

	// Wait for server to stop
	wg.Wait()

	// Verify the output
	assert.NoError(t, serverErr, "Server should exit without error")
	// Wait for output to appear (with timeout)
	outputStr := outputBuf.String()
	assert.True(t, waitForOutput(outputBuf, `"result":{"success":true}`, 500*time.Millisecond),
		"Response should contain expected result, got: %s", outputStr)
	assert.True(t, waitForOutput(outputBuf, `"id":123`, 500*time.Millisecond),
		"Response should contain request ID, got: %s", outputStr)

	// Verify the mock was called
	mockCore.AssertExpectations(t)
}

// Test handling of malformed JSON.
func TestServer_ParseError(t *testing.T) {
	server, mockCore, customReader, outputBuf := setupTestServer(t)

	// Create invalid JSON request
	invalidJSON := `{"jsonrpc":"2.0","id":456,"method":"bad/json",`

	// Start server in a goroutine
	var serverErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		serverErr = server.Start()
	}()

	// Wait for server to start
	time.Sleep(50 * time.Millisecond)

	// Write invalid JSON to input
	customReader.WriteString(invalidJSON + "\n")

	// Give server time to process
	time.Sleep(100 * time.Millisecond)

	// Close input to stop server
	customReader.Close()

	// Wait for server to stop
	wg.Wait()

	// Verify the output contains the parse error
	assert.NoError(t, serverErr, "Server should exit without error")
	assert.Contains(t, outputBuf.String(), `"error":{"code":-32700,"message":"Parse error"`,
		"Response should contain parse error")

	// Verify the mock was NOT called (due to parse error)
	mockCore.AssertNotCalled(t, "HandleMessage")
}

// Test handling of method errors.
func TestServer_MethodError(t *testing.T) {
	server, mockCore, customReader, outputBuf := setupTestServer(t)

	// Setup error to be returned
	methodError := errors.New("unknown method: test/unknown")

	// Configure mock to return error
	mockCore.On("HandleMessage", mock.Anything, mock.Anything).Return(nil, methodError)

	// Create test request
	requestJSON := createJSONRPCRequest(789, "test/unknown", nil)

	// Start server in a goroutine
	var serverErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		serverErr = server.Start()
	}()

	// Wait for server to start
	time.Sleep(50 * time.Millisecond)

	// Write request to input
	customReader.WriteString(requestJSON)

	// Give server time to process
	time.Sleep(100 * time.Millisecond)

	// Close input to stop server
	customReader.Close()

	// Wait for server to stop
	wg.Wait()

	// Verify the output contains the method not found error
	assert.NoError(t, serverErr, "Server should exit without error")
	assert.Contains(t, outputBuf.String(), `"error":{"code":-32601,"message":"Method not found"`,
		"Response should contain method not found error")

	// Verify the mock was called
	mockCore.AssertExpectations(t)
}

// Test notification forwarding.
func TestServer_NotificationForwarding(t *testing.T) {
	server, mockCore, customReader, outputBuf := setupTestServer(t)

	// Start server in a goroutine
	var serverErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		serverErr = server.Start()
	}()

	// Wait for server to start
	time.Sleep(50 * time.Millisecond)

	// Send a notification through the core server
	notif := protocol.Message{
		JSONRPC: "2.0",
		Method:  "notifications/test",
		Params:  map[string]string{"key": "value"},
	}
	mockCore.SendNotification(notif)

	// Give server time to process
	time.Sleep(100 * time.Millisecond)

	// Close input to stop server
	customReader.Close()

	// Wait for server to stop
	wg.Wait()

	// Verify the notification was forwarded
	assert.NoError(t, serverErr, "Server should exit without error")
	assert.Contains(t, outputBuf.String(), `"method":"notifications/test"`,
		"Output should contain forwarded notification")
	assert.Contains(t, outputBuf.String(), `"key":"value"`,
		"Output should contain notification parameters")
}

// Test request timeout handling.
func TestServer_RequestTimeout(t *testing.T) {
	server, mockCore, customReader, outputBuf := setupTestServer(t)

	// Configure mock to simulate a long-running operation
	mockCore.On("HandleMessage", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		// Sleep longer than the server timeout
		time.Sleep(200 * time.Millisecond)
	}).Return(nil, context.DeadlineExceeded)

	// Create test request
	requestJSON := createJSONRPCRequest(101, "test/slow", nil)

	// Start server in a goroutine
	var serverErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		serverErr = server.Start()
	}()

	// Wait for server to start
	time.Sleep(50 * time.Millisecond)

	// Write request to input
	customReader.WriteString(requestJSON)

	// Give server time to process (longer than timeout)
	time.Sleep(300 * time.Millisecond)

	// Close input to stop server
	customReader.Close()

	// Wait for server to stop
	wg.Wait()

	// Verify the timeout error
	assert.NoError(t, serverErr, "Server should exit without error")
	assert.Contains(t, outputBuf.String(), `"error":{"code":-32000,"message":"Request processing timed out"`,
		"Response should contain timeout error")
}

// Test graceful shutdown.
func TestServer_GracefulShutdown(t *testing.T) {
	server, mockCore, customReader, _ := setupTestServer(t)

	// Configure mock to simulate a long-running operation
	mockCore.On("HandleMessage", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		// Sleep to simulate work
		time.Sleep(100 * time.Millisecond)
	}).Return(&protocol.Message{JSONRPC: "2.0"}, nil)

	// Start server in a goroutine
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := server.Start(); err != nil {
			t.Errorf("Server start failed: %v", err)
		}
	}()

	// Wait for server to start
	time.Sleep(50 * time.Millisecond)

	// Write a request to input
	requestJSON := createJSONRPCRequest(999, "test/work", nil)
	customReader.WriteString(requestJSON)

	// Give server time to start processing
	time.Sleep(50 * time.Millisecond)

	// Initiate graceful shutdown
	shutdownErr := server.Stop()

	// Verify shutdown was successful
	assert.NoError(t, shutdownErr, "Shutdown should complete without error")

	// Verify that server properly waits for in-flight operations
	mockCore.AssertExpectations(t)
}

// Test rejecting new requests during shutdown.
func TestServer_RejectDuringShutdown(t *testing.T) {
	server, mockCore, customReader, outputBuf := setupTestServer(t)

	// Start server in a goroutine
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := server.Start(); err != nil {
			t.Errorf("Server start failed: %v", err)
		}
	}()
	// Wait for server to start
	time.Sleep(50 * time.Millisecond)

	// Start shutdown but don't wait for it to complete
	go func() {
		err := server.Stop()
		if err != nil {
			// Log or handle the error
			t.Errorf("Server stop failed: %v", err)
		}
	}()

	// Wait a bit for shutdown to start but not complete
	time.Sleep(20 * time.Millisecond)

	// Try to send a new request during shutdown
	requestJSON := createJSONRPCRequest(888, "test/during/shutdown", nil)
	customReader.WriteString(requestJSON)

	// Give server time to process
	time.Sleep(100 * time.Millisecond)

	// Close input to ensure server stops
	customReader.Close()

	// Wait for server to stop
	wg.Wait()

	// Verify the request was rejected
	assert.Contains(t, outputBuf.String(), `"error":{"code":-32001,"message":"Server is shutting down"`,
		"Response should indicate server is shutting down")

	// Verify the core handler was not called for the rejected request
	mockCore.AssertNotCalled(t, "HandleMessage")
}

// Test handling notifications without ID.
func TestServer_ProcessNotification(t *testing.T) {
	server, mockCore, customReader, outputBuf := setupTestServer(t)

	// Configure mock for notification handling
	mockCore.On("HandleMessage", mock.Anything, mock.Anything).Return(nil, nil)

	// Create notification (no ID)
	notificationJSON := createJSONRPCRequest(nil, "test/notification", map[string]string{"event": "update"})

	// Start server in a goroutine
	var serverErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := server.Start(); err != nil {
			t.Errorf("Server start failed: %v", err)
		}
	}()

	// Wait for server to start
	time.Sleep(50 * time.Millisecond)

	// Write notification to input
	customReader.WriteString(notificationJSON)

	// Give server time to process
	time.Sleep(100 * time.Millisecond)

	// Close input to stop server
	customReader.Close()

	// Wait for server to stop
	wg.Wait()

	// Verify no response was sent (notifications don't get responses)
	assert.NoError(t, serverErr, "Server should exit without error")
	assert.Empty(t, outputBuf.String(), "No response should be sent for a notification")

	// Verify the mock was called
	mockCore.AssertExpectations(t)
}

// Test changing timeout at runtime.
func TestServer_SetTimeout(t *testing.T) {
	server, _, _, _ := setupTestServer(t)

	// Initial timeout should be the one we set in setup
	assert.Equal(t, 100*time.Millisecond, server.defaultTimeout, "Initial timeout should match config")

	// Change timeout
	newTimeout := 500 * time.Millisecond
	server.SetTimeout(newTimeout)

	// Verify timeout was changed
	assert.Equal(t, newTimeout, server.defaultTimeout, "Timeout should be updated")
}

// Test proper handling of invalid params error.
func TestServer_InvalidParams(t *testing.T) {
	server, mockCore, customReader, outputBuf := setupTestServer(t)

	// Setup error to be returned
	paramsError := errors.New("invalid params: missing required parameter 'name'")

	// Configure mock to return error
	mockCore.On("HandleMessage", mock.Anything, mock.Anything).Return(nil, paramsError)

	// Create test request
	requestJSON := createJSONRPCRequest(555, "test/method", map[string]string{})

	// Start server in a goroutine
	var serverErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		serverErr = server.Start()
	}()

	// Wait for server to start
	time.Sleep(50 * time.Millisecond)

	// Write request to input
	customReader.WriteString(requestJSON)

	// Give server time to process
	time.Sleep(100 * time.Millisecond)

	// Close input to stop server
	customReader.Close()

	// Wait for server to stop
	wg.Wait()

	// Verify the output contains the invalid params error
	assert.NoError(t, serverErr, "Server should exit without error")
	assert.Contains(t, outputBuf.String(), `"error":{"code":-32602,"message":"Invalid params"`,
		"Response should contain invalid params error")

	// Verify the mock was called
	mockCore.AssertExpectations(t)
}
