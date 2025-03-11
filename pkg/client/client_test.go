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
	sendFunc        func(ctx context.Context, msg *protocol.Message) error
	receiveFunc     func(ctx context.Context) (*protocol.Message, error)
	receiveFuncLock sync.RWMutex
	closeFunc       func() error
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
	m.receiveFuncLock.RLock()
	receiveFunc := m.receiveFunc
	m.receiveFuncLock.RUnlock()

	if receiveFunc != nil {
		return receiveFunc(ctx)
	}

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

// SetReceiveFunc sets a custom function for Receive.
func (m *MockTransport) SetReceiveFunc(f func(ctx context.Context) (*protocol.Message, error)) {
	m.receiveFuncLock.Lock()
	defer m.receiveFuncLock.Unlock()
	m.receiveFunc = f
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

// Helper to make a JSON response for a specific method and ID.
func makeJSONResponse(id protocol.RequestID, resultObj interface{}) *protocol.Message {
	return &protocol.Message{
		JSONRPC: "2.0",
		ID:      &id,
		Result:  resultObj,
	}
}

// Helper to configure transport to return error for a method.
func configureMockForError(transport *MockTransport, methodToFail string) {
	transport.SetSendFunc(func(ctx context.Context, msg *protocol.Message) error {
		if msg.Method == methodToFail {
			return errors.New("mock error")
		}
		return nil
	})
}

// Helper to create and directly add a mock response for a specific request.
func addMockResponseForMethod(t *testing.T, transport *MockTransport, method string, id protocol.RequestID) {
	var response *protocol.Message

	switch method {
	case "resources/list":
		result := models.ListResourcesResult{
			Resources: []models.Resource{{
				Name: "test-resource",
				URI:  "test://resource",
			}},
		}
		response = makeJSONResponse(id, result)

	case "resources/read":
		// Create a TextResourceContents
		textContent := &models.TextResourceContents{
			ResourceContents: models.ResourceContents{
				URI:      "test://resource",
				MimeType: "text/plain",
			},
			Text: "test content",
		}

		// Put it in the result
		result := models.ReadResourceResult{
			Contents: []models.ResourceContent{textContent},
		}
		response = makeJSONResponse(id, result)

	case "resources/subscribe", "resources/unsubscribe":
		response = makeJSONResponse(id, struct{}{})

	case "tools/list":
		result := models.ListToolsResult{
			Tools: []models.Tool{{
				Name: "test-tool",
				InputSchema: models.InputSchema{
					Type:       "object",
					Properties: map[string]models.ParameterSchema{},
				},
			}},
		}
		response = makeJSONResponse(id, result)

	case "tools/call":
		textContent := models.TextContent{
			Type: "text",
			Text: "tool result",
		}

		result := models.CallToolResult{
			Content: []models.Content{textContent},
		}
		response = makeJSONResponse(id, result)

	case "prompts/list":
		result := models.ListPromptsResult{
			Prompts: []models.Prompt{{
				Name: "test-prompt",
			}},
		}
		response = makeJSONResponse(id, result)

	case "prompts/get":
		textContent := models.TextContent{
			Type: "text",
			Text: "test prompt",
		}

		result := models.GetPromptResult{
			Messages: []models.PromptMessage{{
				Role:    "user",
				Content: textContent,
			}},
		}
		response = makeJSONResponse(id, result)

	case "logging/setLevel":
		response = makeJSONResponse(id, struct{}{})
	}

	if response != nil {
		t.Logf("Adding prepared response for ID %v, method %s", id, method)
		transport.AddResponse(id, response)
	}
}

// Mock function that captures the request ID and pre-populates the response.
func captureAndRespond(t *testing.T, transport *MockTransport, method string) func(ctx context.Context, msg *protocol.Message) error {
	return func(ctx context.Context, msg *protocol.Message) error {
		// Store last method and ID
		transport.lastLock.Lock()
		transport.lastMethod = msg.Method
		if msg.ID != nil {
			transport.lastID = *msg.ID
			// If this is our target method, prepare the response right away
			if msg.Method == method {
				// First store the ID then release the lock
				id := *msg.ID
				transport.lastLock.Unlock()
				// Add the response
				addMockResponseForMethod(t, transport, method, id)
			} else {
				transport.lastLock.Unlock()
			}
		} else {
			transport.lastLock.Unlock()
		}

		return nil
	}
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

// Test all resource methods.
func TestResourceMethods(t *testing.T) {
	// Test ListResources
	t.Run("ListResources", func(t *testing.T) {
		// Success case
		client, transport := setUp(t)
		client.setInitialized(true)

		// Using our new approach to avoid race conditions
		transport.SetSendFunc(captureAndRespond(t, transport, "resources/list"))

		result, err := client.ListResources(context.Background(), nil)
		if err != nil {
			t.Errorf("ListResources() error = %v", err)
		}
		if len(result.Resources) != 1 || result.Resources[0].Name != "test-resource" {
			t.Errorf("ListResources() unexpected result: %+v", result)
		}

		// Not initialized case
		client, _ = setUp(t)
		_, err = client.ListResources(context.Background(), nil)
		if err != mcperrors.ErrNotInitialized {
			t.Errorf("Expected ErrNotInitialized, got %v", err)
		}

		// Error case
		client, transport = setUp(t)
		client.setInitialized(true)
		configureMockForError(transport, "resources/list")

		_, err = client.ListResources(context.Background(), nil)
		if err == nil {
			t.Error("Expected error from ListResources(), got nil")
		}
	})

	// Test ReadResource
	t.Run("ReadResource", func(t *testing.T) {
		// Success case
		client, transport := setUp(t)
		client.setInitialized(true)
		transport.SetSendFunc(captureAndRespond(t, transport, "resources/read"))

		result, err := client.ReadResource(context.Background(), "test://resource")
		if err != nil {
			t.Errorf("ReadResource() error = %v", err)
		}

		// Check result
		if len(result.Contents) != 1 {
			t.Errorf("ReadResource() expected 1 content, got %d", len(result.Contents))
			return
		}

		// Type assert to check text content
		if textContent, ok := result.Contents[0].(*models.TextResourceContents); ok {
			if textContent.Text != "test content" {
				t.Errorf("ReadResource() got text = %s, want %s", textContent.Text, "test content")
			}
		} else {
			t.Errorf("ReadResource() expected TextResourceContents, got %T", result.Contents[0])
		}

		// Not initialized case
		client, _ = setUp(t)
		_, err = client.ReadResource(context.Background(), "test://resource")
		if err != mcperrors.ErrNotInitialized {
			t.Errorf("Expected ErrNotInitialized, got %v", err)
		}

		// Error case
		client, transport = setUp(t)
		client.setInitialized(true)
		configureMockForError(transport, "resources/read")

		_, err = client.ReadResource(context.Background(), "test://resource")
		if err == nil {
			t.Error("Expected error from ReadResource(), got nil")
		}
	})

	// Test Subscribe
	t.Run("Subscribe", func(t *testing.T) {
		// Success case
		client, transport := setUp(t)
		client.setInitialized(true)
		transport.SetSendFunc(captureAndRespond(t, transport, "resources/subscribe"))

		err := client.Subscribe(context.Background(), "test://resource")
		if err != nil {
			t.Errorf("Subscribe() error = %v", err)
		}

		// Not initialized case
		client, _ = setUp(t)
		err = client.Subscribe(context.Background(), "test://resource")
		if err != mcperrors.ErrNotInitialized {
			t.Errorf("Expected ErrNotInitialized, got %v", err)
		}

		// Error case
		client, transport = setUp(t)
		client.setInitialized(true)
		configureMockForError(transport, "resources/subscribe")

		err = client.Subscribe(context.Background(), "test://resource")
		if err == nil {
			t.Error("Expected error from Subscribe(), got nil")
		}
	})

	// Test Unsubscribe
	t.Run("Unsubscribe", func(t *testing.T) {
		// Success case
		client, transport := setUp(t)
		client.setInitialized(true)
		transport.SetSendFunc(captureAndRespond(t, transport, "resources/unsubscribe"))

		err := client.Unsubscribe(context.Background(), "test://resource")
		if err != nil {
			t.Errorf("Unsubscribe() error = %v", err)
		}

		// Not initialized case
		client, _ = setUp(t)
		err = client.Unsubscribe(context.Background(), "test://resource")
		if err != mcperrors.ErrNotInitialized {
			t.Errorf("Expected ErrNotInitialized, got %v", err)
		}

		// Error case
		client, transport = setUp(t)
		client.setInitialized(true)
		configureMockForError(transport, "resources/unsubscribe")

		err = client.Unsubscribe(context.Background(), "test://resource")
		if err == nil {
			t.Error("Expected error from Unsubscribe(), got nil")
		}
	})
}

// Test all tool methods.
func TestToolMethods(t *testing.T) {
	// Test ListTools
	t.Run("ListTools", func(t *testing.T) {
		// Success case
		client, transport := setUp(t)
		client.setInitialized(true)
		transport.SetSendFunc(captureAndRespond(t, transport, "tools/list"))

		result, err := client.ListTools(context.Background(), nil)
		if err != nil {
			t.Errorf("ListTools() error = %v", err)
		}
		if len(result.Tools) != 1 || result.Tools[0].Name != "test-tool" {
			t.Errorf("ListTools() unexpected result: %+v", result)
		}

		// Not initialized case
		client, _ = setUp(t)
		_, err = client.ListTools(context.Background(), nil)
		if err != mcperrors.ErrNotInitialized {
			t.Errorf("Expected ErrNotInitialized, got %v", err)
		}

		// Error case
		client, transport = setUp(t)
		client.setInitialized(true)
		configureMockForError(transport, "tools/list")

		_, err = client.ListTools(context.Background(), nil)
		if err == nil {
			t.Error("Expected error from ListTools(), got nil")
		}
	})

	// Test CallTool
	t.Run("CallTool", func(t *testing.T) {
		// Success case
		client, transport := setUp(t)
		client.setInitialized(true)
		transport.SetSendFunc(captureAndRespond(t, transport, "tools/call"))

		result, err := client.CallTool(context.Background(), "test-tool", map[string]interface{}{"arg": "value"})
		if err != nil {
			t.Errorf("CallTool() error = %v", err)
		}

		if len(result.Content) != 1 {
			t.Errorf("CallTool() expected 1 content item, got %d", len(result.Content))
			return
		}

		// Check the content type and text
		textContent, ok := result.Content[0].(models.TextContent)
		if !ok {
			t.Errorf("CallTool() expected TextContent, got %T", result.Content[0])
			return
		}

		if textContent.ContentType() != "text" || textContent.Text != "tool result" {
			t.Errorf("CallTool() got incorrect content: %+v", textContent)
		}

		// Not initialized case
		client, _ = setUp(t)
		_, err = client.CallTool(context.Background(), "test-tool", nil)
		if err != mcperrors.ErrNotInitialized {
			t.Errorf("Expected ErrNotInitialized, got %v", err)
		}

		// Error case
		client, transport = setUp(t)
		client.setInitialized(true)
		configureMockForError(transport, "tools/call")

		_, err = client.CallTool(context.Background(), "test-tool", nil)
		if err == nil {
			t.Error("Expected error from CallTool(), got nil")
		}
	})
}

// Test all prompt methods.
func TestPromptMethods(t *testing.T) {
	// Test ListPrompts
	t.Run("ListPrompts", func(t *testing.T) {
		// Success case
		client, transport := setUp(t)
		client.setInitialized(true)
		transport.SetSendFunc(captureAndRespond(t, transport, "prompts/list"))

		result, err := client.ListPrompts(context.Background(), nil)
		if err != nil {
			t.Errorf("ListPrompts() error = %v", err)
		}
		if len(result.Prompts) != 1 || result.Prompts[0].Name != "test-prompt" {
			t.Errorf("ListPrompts() unexpected result: %+v", result)
		}

		// Not initialized case
		client, _ = setUp(t)
		_, err = client.ListPrompts(context.Background(), nil)
		if err != mcperrors.ErrNotInitialized {
			t.Errorf("Expected ErrNotInitialized, got %v", err)
		}

		// Error case
		client, transport = setUp(t)
		client.setInitialized(true)
		configureMockForError(transport, "prompts/list")

		_, err = client.ListPrompts(context.Background(), nil)
		if err == nil {
			t.Error("Expected error from ListPrompts(), got nil")
		}
	})

	// Test GetPrompt
	t.Run("GetPrompt", func(t *testing.T) {
		// Success case
		client, transport := setUp(t)
		client.setInitialized(true)
		transport.SetSendFunc(captureAndRespond(t, transport, "prompts/get"))

		result, err := client.GetPrompt(context.Background(), "test-prompt", map[string]string{"arg": "value"})
		if err != nil {
			t.Errorf("GetPrompt() error = %v", err)
		}
		if len(result.Messages) != 1 || result.Messages[0].Role != "user" {
			t.Errorf("GetPrompt() unexpected result: %+v", result)
		}

		// Not initialized case
		client, _ = setUp(t)
		_, err = client.GetPrompt(context.Background(), "test-prompt", nil)
		if err != mcperrors.ErrNotInitialized {
			t.Errorf("Expected ErrNotInitialized, got %v", err)
		}

		// Error case
		client, transport = setUp(t)
		client.setInitialized(true)
		configureMockForError(transport, "prompts/get")

		_, err = client.GetPrompt(context.Background(), "test-prompt", nil)
		if err == nil {
			t.Error("Expected error from GetPrompt(), got nil")
		}
	})
}

// Test SetLogLevel.
func TestSetLogLevel(t *testing.T) {
	// Success case
	client, transport := setUp(t)
	client.setInitialized(true)
	transport.SetSendFunc(captureAndRespond(t, transport, "logging/setLevel"))

	err := client.SetLogLevel(context.Background(), "debug")
	if err != nil {
		t.Errorf("SetLogLevel() error = %v", err)
	}

	// Not initialized case
	client, _ = setUp(t)
	err = client.SetLogLevel(context.Background(), "debug")
	if err != mcperrors.ErrNotInitialized {
		t.Errorf("Expected ErrNotInitialized, got %v", err)
	}

	// Error case
	client, transport = setUp(t)
	client.setInitialized(true)
	configureMockForError(transport, "logging/setLevel")

	err = client.SetLogLevel(context.Background(), "debug")
	if err == nil {
		t.Error("Expected error from SetLogLevel(), got nil")
	}
}

// Test the notifications channel functionality.
func TestNotificationsChannel(t *testing.T) {
	client, _ := setUp(t)

	// Get notifications channel
	notifChan := client.Notifications()
	if notifChan == nil {
		t.Fatal("Expected non-nil notifications channel")
	}

	// Create and send a notification
	notification := &protocol.Message{
		JSONRPC: "2.0",
		Method:  "test/notification",
		Params:  json.RawMessage(`{"key":"value"}`),
	}

	// Inject notification directly into client's channel
	client.notifChan <- notification

	// Receive on the channel
	select {
	case receivedNotif := <-notifChan:
		if receivedNotif.Method != notification.Method {
			t.Errorf("Expected notification method %s, got %s", notification.Method, receivedNotif.Method)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timed out waiting for notification")
	}
}

// Test client behavior with shutdown state.
func TestClientShutdownState(t *testing.T) {
	client, _ := setUp(t)

	// Set client as shut down
	client.setShutdown(true)

	// Attempt operations on shut down client
	ctx := context.Background()

	// Test sending request
	_, err := client.sendRequest(ctx, "test", nil)
	if err != mcperrors.ErrConnClosed {
		t.Errorf("Expected ErrConnClosed from sendRequest on shutdown client, got %v", err)
	}

	// Test sending notification
	err = client.sendNotification(ctx, "test", nil)
	if err != mcperrors.ErrConnClosed {
		t.Errorf("Expected ErrConnClosed from sendNotification on shutdown client, got %v", err)
	}

	// Test isShutdown method
	if !client.isShutdown() {
		t.Error("Expected isShutdown() to return true")
	}
}

// TestMessageHandling tests the client's message handling loop.
func TestMessageHandling(t *testing.T) {
	// Create a client and transport
	client, transport := setUp(t)

	// Test handling notifications
	t.Run("HandlesNotifications", func(t *testing.T) {
		// Create a notification to deliver
		notificationMethod := "test/notification"
		notification := &protocol.Message{
			JSONRPC: "2.0",
			Method:  notificationMethod,
			Params:  json.RawMessage(`{"test":"value"}`),
		}

		// Create a channel to signal when the notification is received
		notifReceived := make(chan bool, 1)

		// Start a goroutine to monitor the notifications channel
		go func() {
			select {
			case msg := <-client.Notifications():
				if msg.Method == notificationMethod {
					notifReceived <- true
				}
			case <-time.After(500 * time.Millisecond):
				t.Error("Timed out waiting for notification")
			}
		}()

		// Set up transport.Receive to return the notification
		var responses = []*protocol.Message{notification, nil, nil, nil}
		var responseIndex = 0
		var responseMutex sync.Mutex

		transport.SetReceiveFunc(func(ctx context.Context) (*protocol.Message, error) {
			responseMutex.Lock()
			defer responseMutex.Unlock()

			if responseIndex < len(responses) && responses[responseIndex] != nil {
				resp := responses[responseIndex]
				responseIndex++
				return resp, nil
			}
			// Default behavior without recursion
			return nil, context.DeadlineExceeded
		})

		// Wait for notification to be processed
		select {
		case <-notifReceived:
			// Success - notification was received
		case <-time.After(500 * time.Millisecond):
			t.Error("Timed out waiting for notification to be processed")
		}
	})

	// Test handling notifications when channel is full
	t.Run("HandlesFullNotificationChannel", func(t *testing.T) {
		// Create a client with very small notification channel
		testLogger := logging.NewNoopLogger() // Or any other suitable logger
		smallClient := NewClient(transport, WithLogger(testLogger))

		// Fill the notification channel
		for i := 0; i < 100; i++ {
			smallClient.notifChan <- &protocol.Message{Method: "filler"}
		}

		// Send a notification that should be dropped
		dropNotification := &protocol.Message{
			JSONRPC: "2.0",
			Method:  "should/be/dropped",
		}
		var responses = []*protocol.Message{dropNotification, nil, nil, nil}
		var responseIndex = 0
		var responseMutex sync.Mutex

		transport.SetReceiveFunc(func(ctx context.Context) (*protocol.Message, error) {
			responseMutex.Lock()
			defer responseMutex.Unlock()

			if responseIndex < len(responses) && responses[responseIndex] != nil {
				resp := responses[responseIndex]
				responseIndex++
				return resp, nil
			}
			// Default behavior without recursion
			return nil, context.DeadlineExceeded
		})

		// Wait a bit to allow processing
		time.Sleep(100 * time.Millisecond)

		// Cleanup
		if err := smallClient.Shutdown(); err != nil {
			t.Fatalf("Failed to shut down client")
		}
	})

	// Test handling responses for in-flight requests
	t.Run("HandlesResponses", func(t *testing.T) {
		// Set up a request context
		reqID := client.requestTracker.NextID()
		reqCtx := client.requestTracker.TrackRequest(reqID, "test/method")

		// Create a response to deliver
		response := &protocol.Message{
			JSONRPC: "2.0",
			ID:      &reqID,
			Result:  json.RawMessage(`{"success":true}`),
		}

		// Set up transport.Receive to return the response
		var responses = []*protocol.Message{response, nil, nil, nil}
		var responseIndex = 0
		var responseMutex sync.Mutex

		transport.SetReceiveFunc(func(ctx context.Context) (*protocol.Message, error) {
			responseMutex.Lock()
			defer responseMutex.Unlock()

			if responseIndex < len(responses) && responses[responseIndex] != nil {
				resp := responses[responseIndex]
				responseIndex++
				return resp, nil
			}
			// Default behavior without recursion
			return nil, context.DeadlineExceeded
		})

		// Wait for response with a timeout
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		resp, err := client.requestTracker.WaitForResponse(ctx, reqCtx)
		if err != nil {
			t.Errorf("Error waiting for response: %v", err)
		}
		if resp == nil {
			t.Error("Expected non-nil response")
		} else if *resp.ID != reqID {
			t.Errorf("Expected response ID %v, got %v", reqID, *resp.ID)
		}
	})

	// Test handling responses for requests that are being set up
	t.Run("HandlesPendingSetupRequests", func(t *testing.T) {
		// Set a pending setup request
		reqID := protocol.RequestID(9999)
		client.pendingSetup.Store(reqID, true)

		// Create a response for this pending request
		response := &protocol.Message{
			JSONRPC: "2.0",
			ID:      &reqID,
			Result:  json.RawMessage(`{"success":true}`),
		}

		// Create a channel to signal when the response is processed
		processed := make(chan bool, 1)

		// Set up the request tracker first
		reqCtx := client.requestTracker.TrackRequest(reqID, "test/pending")

		// Set up a goroutine to check if the response is processed
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
			defer cancel()

			resp, err := client.requestTracker.WaitForResponse(ctx, reqCtx)
			if err != nil {
				t.Errorf("Error waiting for response: %v", err)
				return
			}
			if resp == nil {
				t.Error("Expected non-nil response")
				return
			}
			if *resp.ID != reqID {
				t.Errorf("Expected response ID %v, got %v", reqID, *resp.ID)
				return
			}
			processed <- true
		}()

		// Give the goroutine time to set up
		time.Sleep(50 * time.Millisecond)

		// Set up transport.Receive to return the response
		var responses = []*protocol.Message{response, nil, nil, nil}
		var responseIndex = 0
		var responseMutex sync.Mutex

		transport.SetReceiveFunc(func(ctx context.Context) (*protocol.Message, error) {
			responseMutex.Lock()
			defer responseMutex.Unlock()

			if responseIndex < len(responses) && responses[responseIndex] != nil {
				resp := responses[responseIndex]
				responseIndex++
				return resp, nil
			}
			// Default behavior without recursion
			return nil, context.DeadlineExceeded
		})

		// Wait for processing to complete
		select {
		case <-processed:
			// Success
		case <-time.After(500 * time.Millisecond):
			t.Error("Timed out waiting for pending setup response to be processed")
		}

		wg.Wait()
	})
}
