package client

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	mcperrors "github.com/XiaoConstantine/mcp-go/pkg/errors"
	"github.com/XiaoConstantine/mcp-go/pkg/logging"
	models "github.com/XiaoConstantine/mcp-go/pkg/model"
	"github.com/XiaoConstantine/mcp-go/pkg/protocol"
)

// MockTransport is a thread-safe mock implementation of the Transport interface for testing
// This implementation is designed to work with the client's background polling loop.
type MockTransport struct {
	t                *testing.T // for logging
	responsesLock    sync.Mutex
	pendingResponses map[protocol.RequestID]*protocol.Message
	notifyChannel    chan *protocol.Message // For notifications

	// For tracking the last method call
	lastID     protocol.RequestID
	lastMethod string
	lastLock   sync.RWMutex

	// Handlers for custom behavior
	sendFunc  func(ctx context.Context, msg *protocol.Message) error
	closeFunc func() error
}

// NewMockTransport creates a new mock transport for testing.
func NewMockTransport(t *testing.T) *MockTransport {
	return &MockTransport{
		t:                t,
		pendingResponses: make(map[protocol.RequestID]*protocol.Message),
		notifyChannel:    make(chan *protocol.Message, 5),
	}
}

// Send captures the request and prepares a response based on the method.
func (m *MockTransport) Send(ctx context.Context, msg *protocol.Message) error {
	// If there's a custom send function, use it
	if m.sendFunc != nil {
		return m.sendFunc(ctx, msg)
	}

	// Handle request message
	if msg.ID != nil && msg.Method != "" {
		m.lastLock.Lock()
		m.lastID = *msg.ID
		m.lastMethod = msg.Method
		m.lastLock.Unlock()

		m.t.Logf("📤 SEND Request: ID=%v, Method=%s", *msg.ID, msg.Method)

		// Auto-generate responses for known methods
		if msg.Method == "initialize" {
			// Create initialize response
			initResult := models.InitializeResult{
				ServerInfo: models.Implementation{
					Name:    "test-server",
					Version: "1.0.0",
				},
				Capabilities:    protocol.ServerCapabilities{},
				Instructions:    "Test instructions",
				ProtocolVersion: "2024-11-05",
			}

			resultBytes, _ := json.Marshal(initResult)
			var rawResult json.RawMessage = resultBytes

			// Create response message
			response := &protocol.Message{
				JSONRPC: "2.0",
				ID:      msg.ID,
				Result:  rawResult,
			}

			// Store response for later delivery
			m.responsesLock.Lock()
			m.pendingResponses[*msg.ID] = response
			m.responsesLock.Unlock()

			// Let the message loop detect this immediately
			go func() {
				time.Sleep(10 * time.Millisecond)
				m.t.Logf("🔄 Response ready for ID=%v", *msg.ID)
			}()
		} else if msg.Method == "ping" {
			// Prepare ping response
			response := &protocol.Message{
				JSONRPC: "2.0",
				ID:      msg.ID,
				Result:  json.RawMessage("{}"),
			}

			// Store response for later delivery
			m.responsesLock.Lock()
			m.pendingResponses[*msg.ID] = response
			m.responsesLock.Unlock()
		}
	} else if msg.Method != "" {
		// Handle notification
		m.t.Logf("📤 SEND Notification: Method=%s", msg.Method)
		m.lastLock.Lock()
		m.lastMethod = msg.Method
		m.lastLock.Unlock()

		// For notifications/initialized, we notify success
		if msg.Method == "notifications/initialized" {
			m.t.Logf("✅ Received notifications/initialized notification")
		}
	}

	return nil
}

// Receive delivers a response or timeout.
func (m *MockTransport) Receive(ctx context.Context) (*protocol.Message, error) {
	// Check for pending responses
	m.responsesLock.Lock()
	m.lastLock.RLock()
	lastID := m.lastID
	m.lastLock.RUnlock()

	if response, ok := m.pendingResponses[lastID]; ok {
		delete(m.pendingResponses, lastID)
		m.responsesLock.Unlock()
		m.t.Logf("📥 RECEIVE Response: ID=%v", lastID)
		return response, nil
	}
	m.responsesLock.Unlock()

	// No response ready, return deadline exceeded to continue polling
	return nil, context.DeadlineExceeded
}

// Close is a no-op by default, or calls the custom function if set.
func (m *MockTransport) Close() error {
	if m.closeFunc != nil {
		return m.closeFunc()
	}
	return nil
}

// GetLastMethod returns the last sent method.
func (m *MockTransport) GetLastMethod() string {
	m.lastLock.RLock()
	defer m.lastLock.RUnlock()
	return m.lastMethod
}

// GetLastRequest returns the last request ID and method.
func (m *MockTransport) GetLastRequest() (protocol.RequestID, string) {
	m.lastLock.RLock()
	defer m.lastLock.RUnlock()
	return m.lastID, m.lastMethod
}

// SetSendFunc sets a custom function for Send.
func (m *MockTransport) SetSendFunc(f func(ctx context.Context, msg *protocol.Message) error) {
	m.sendFunc = f
}

// SetCloseFunc sets a custom function for Close.
func (m *MockTransport) SetCloseFunc(f func() error) {
	m.closeFunc = f
}

// AddResponse manually adds a response that will be returned for a specific request ID.
func (m *MockTransport) AddResponse(id protocol.RequestID, response *protocol.Message) {
	m.responsesLock.Lock()
	defer m.responsesLock.Unlock()
	m.pendingResponses[id] = response
}

// setUp creates a new client with mock implementation and ensures proper shutdown.
func setUp(t *testing.T) (*Client, *MockTransport) {
	// Create a test logger and mock transport
	testLogger := logging.NewNoopLogger()

	transport := NewMockTransport(t)

	// Create client with the transport and logger
	client := NewClient(transport, WithLogger(testLogger))

	// Register cleanup to ensure client is shut down after test
	t.Cleanup(func() {
		if err := client.Shutdown(); err != nil {
			t.Logf("Error shutting down client: %v", err)
		}
	})

	return client, transport
}

func TestNewClient(t *testing.T) {
	// Create a mock transport
	transport := NewMockTransport(t)

	// Test default initialization
	client := NewClient(transport)
	if client == nil {
		t.Fatal("Expected non-nil client")
	}
	defer func() {
		if err := client.Shutdown(); err != nil {
			t.Logf("Error shutting down client: %v", err)
		}
	}()

	// Check default values
	if client.clientInfo.Name != "mcp-go-client" {
		t.Errorf("Expected default client name to be 'mcp-go-client', got %s", client.clientInfo.Name)
	}

	// Test with options
	logger := logging.NewNoopLogger()
	customName := "custom-client"
	customVersion := "1.0.0"

	transport2 := NewMockTransport(t)
	client2 := NewClient(transport2, WithLogger(logger), WithClientInfo(customName, customVersion))
	defer func() {
		if err := client2.Shutdown(); err != nil {
			t.Logf("Error shutting down client2: %v", err)
		}
	}()

	if client2.clientInfo.Name != customName {
		t.Errorf("Expected client name to be %s, got %s", customName, client2.clientInfo.Name)
	}
	if client2.clientInfo.Version != customVersion {
		t.Errorf("Expected client version to be %s, got %s", customVersion, client2.clientInfo.Version)
	}
}

func TestClientInitialize(t *testing.T) {
	// Create a client and transport with our setup function
	client, _ := setUp(t)

	t.Logf("Starting Initialize() test call")

	// Create a context with timeout to prevent hanging
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Call Initialize
	initResult, err := client.Initialize(ctx)
	if err != nil {
		t.Fatalf("Failed to initialize client: %v", err)
	}
	t.Logf("Initialize() test call completed successfully")

	// Verify initialization result
	if initResult.ServerInfo.Name != "test-server" {
		t.Errorf("Expected server name 'test-server', got %s", initResult.ServerInfo.Name)
	}
	if initResult.Instructions != "Test instructions" {
		t.Errorf("Expected instructions 'Test instructions', got %s", initResult.Instructions)
	}
	if initResult.ProtocolVersion != "2024-11-05" {
		t.Errorf("Expected protocol version '2024-11-05', got %s", initResult.ProtocolVersion)
	}

	// Check client state after initialization
	if !client.isInitialized() {
		t.Error("Expected client to be initialized after successful initialization")
	}

	// Test error handling for double initialization
	_, err = client.Initialize(ctx)
	if err == nil {
		t.Error("Expected error for double initialization, got nil")
	}
}

func TestClientPing(t *testing.T) {
	client, _ := setUp(t)

	// Test ping before initialization
	err := client.Ping(context.Background())
	if err != mcperrors.ErrNotInitialized {
		t.Errorf("Expected ErrNotInitialized, got %v", err)
	}

	// Initialize the client first
	ctx := context.Background()
	_, err = client.Initialize(ctx)
	if err != nil {
		t.Fatalf("Failed to initialize client: %v", err)
	}

	// Test successful ping
	err = client.Ping(ctx)
	if err != nil {
		t.Errorf("Expected successful ping, got error: %v", err)
	}

	// Test ping with network error
	transport := NewMockTransport(t)
	transport.SetSendFunc(func(ctx context.Context, msg *protocol.Message) error {
		return errors.New("network error")
	})

	clientWithError := NewClient(transport)
	defer func() {
		if err := clientWithError.Shutdown(); err != nil {
			t.Logf("Error shutting down clientWithError: %v", err)
		}
	}()

	// Mark as initialized to allow ping
	clientWithError.setInitialized(true)

	err = clientWithError.Ping(ctx)
	if err == nil {
		t.Error("Expected error for ping with network failure, got nil")
	}
}

func TestClientShutdown(t *testing.T) {
	closeCalled := false
	transport := NewMockTransport(t)

	transport.SetCloseFunc(func() error {
		closeCalled = true
		return nil
	})

	client := NewClient(transport)

	// Test successful shutdown
	err := client.Shutdown()
	if err != nil {
		t.Errorf("Expected successful shutdown, got error: %v", err)
	}
	if !closeCalled {
		t.Error("Expected transport Close() to be called")
	}
	if !client.isShutdown() {
		t.Error("Expected client to be marked as shutdown")
	}

	// Test double shutdown (should be no-op)
	closeCalled = false
	err = client.Shutdown()
	if err != nil {
		t.Errorf("Expected successful double shutdown, got error: %v", err)
	}
	if closeCalled {
		t.Error("Expected transport Close() not to be called on double shutdown")
	}
}

func TestClientGetters(t *testing.T) {
	client, _ := setUp(t)

	// Initialize with test values
	capabilities := protocol.ServerCapabilities{}
	serverInfo := models.Implementation{
		Name:    "test-server",
		Version: "1.0.0",
	}
	instructions := "Test instructions"
	protocolVersion := "2024-11-05"

	client.capabilities = &capabilities
	client.serverInfo = &serverInfo
	client.instructions = instructions
	client.protocolVersion = protocolVersion

	// Test getters
	if client.GetCapabilities() != &capabilities {
		t.Error("GetCapabilities() did not return expected capabilities")
	}
	if client.GetServerInfo() != &serverInfo {
		t.Error("GetServerInfo() did not return expected server info")
	}
	if client.GetInstructions() != instructions {
		t.Errorf("GetInstructions() returned %s, expected %s", client.GetInstructions(), instructions)
	}
	if client.GetProtocolVersion() != protocolVersion {
		t.Errorf("GetProtocolVersion() returned %s, expected %s", client.GetProtocolVersion(), protocolVersion)
	}
}
