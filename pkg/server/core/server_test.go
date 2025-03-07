package core

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/XiaoConstantine/mcp-go/pkg/models"
	"github.com/XiaoConstantine/mcp-go/pkg/protocol"
	"github.com/XiaoConstantine/mcp-go/pkg/server/logging"
	"github.com/XiaoConstantine/mcp-go/pkg/server/prompt"
	"github.com/XiaoConstantine/mcp-go/pkg/server/resource"
	"github.com/XiaoConstantine/mcp-go/pkg/server/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createRequestID(id interface{}) *protocol.RequestID {
	reqID := protocol.RequestID(id)
	return &reqID
}

func TestNewServer(t *testing.T) {
	// Test server creation with valid implementation info
	info := models.Implementation{
		Name:    "TestServer",
		Version: "1.0.0",
	}
	server := NewServer(info, "1.0.0")

	assert.NotNil(t, server)
	assert.Equal(t, info, server.info)
	assert.Equal(t, "1.0.0", server.version)
	assert.NotNil(t, server.resourceManager)
	assert.NotNil(t, server.toolsManager)
	assert.NotNil(t, server.promptManager)
	assert.NotNil(t, server.logManager)
	assert.NotNil(t, server.notificationChan)
	assert.False(t, server.initialized)
}

func TestIsInitialized(t *testing.T) {
	server := createTestServer()

	// Should start uninitialized
	assert.False(t, server.IsInitialized())

	// Simulate initialization
	server.mu.Lock()
	server.initialized = true
	server.mu.Unlock()

	assert.True(t, server.IsInitialized())
}

func TestHandleInitialize(t *testing.T) {
	server := createTestServer()
	ctx := context.Background()

	// Create a valid initialize request
	initParams := struct {
		Capabilities    protocol.ClientCapabilities `json:"capabilities"`
		ClientInfo      models.Implementation       `json:"clientInfo"`
		ProtocolVersion string                      `json:"protocolVersion"`
	}{
		Capabilities: protocol.ClientCapabilities{
			Sampling: map[string]interface{}{},
		},
		ClientInfo: models.Implementation{
			Name:    "TestClient",
			Version: "1.0.0",
		},
		ProtocolVersion: "1.0.0",
	}

	paramsBytes, err := json.Marshal(initParams)
	require.NoError(t, err)

	requestID := createRequestID("req-1")
	msg := &protocol.Message{
		JSONRPC: "2.0",
		ID:      requestID,
		Method:  "initialize",
		Params:  json.RawMessage(paramsBytes),
	}

	// Handle the initialize request
	response, err := server.HandleMessage(ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, response)

	// Verify server is now initialized
	assert.True(t, server.IsInitialized())

	// Verify response structure
	assert.Equal(t, "2.0", response.JSONRPC)
	assert.Equal(t, requestID, response.ID)
	assert.NotNil(t, response.Result)

	// Check result content
	var result models.InitializeResult
	err = json.Unmarshal(response.Result.(json.RawMessage), &result)
	require.NoError(t, err)

	assert.Equal(t, server.info, result.ServerInfo)
	assert.Equal(t, server.version, result.ProtocolVersion)
	assert.NotEmpty(t, result.Instructions)
	assert.NotNil(t, result.Capabilities)
}

func TestHandleMessageNotInitialized(t *testing.T) {
	server := createTestServer()
	ctx := context.Background()

	// Try to handle a message before initialization
	requestID := createRequestID("req-1")
	msg := &protocol.Message{
		JSONRPC: "2.0",
		ID:      requestID,
		Method:  "ping",
	}

	_, err := server.HandleMessage(ctx, msg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not initialized")
}

func TestHandlePing(t *testing.T) {
	server := createTestServer()
	ctx := context.Background()

	// Set server as initialized
	server.mu.Lock()
	server.initialized = true
	server.mu.Unlock()

	// Create ping request
	requestID := createRequestID("req-1")
	msg := &protocol.Message{
		JSONRPC: "2.0",
		ID:      requestID,
		Method:  "ping",
	}

	// Handle ping
	response, err := server.HandleMessage(ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, response)

	// Verify response
	assert.Equal(t, requestID, response.ID)
}

func TestServerLifecycle(t *testing.T) {
	info := models.Implementation{
		Name:    "test-server",
		Version: "1.0.0",
	}
	server := NewServer(info, "1.0")

	require.NotNil(t, server)
	assert.Equal(t, info, server.info)
	assert.Equal(t, "1.0", server.version)
	assert.False(t, server.IsInitialized())
	assert.NotNil(t, server.resourceManager)
	assert.NotNil(t, server.notificationChan)

	ctx := context.Background()
	pingMsg := &protocol.Message{
		JSONRPC: "2.0",
		ID:      createRequestID(1),
		Method:  "ping",
	}
	_, err := server.HandleMessage(ctx, pingMsg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not initialized")

	initResult := initializeServer(t, server)

	assert.Equal(t, info, initResult.ServerInfo)
	assert.Equal(t, "1.0", initResult.ProtocolVersion)
	assert.NotNil(t, initResult.Capabilities.Resources)
	assert.True(t, initResult.Capabilities.Resources.Subscribe)
	assert.True(t, server.IsInitialized())

	resp, err := server.HandleMessage(ctx, pingMsg)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestResourceOperations(t *testing.T) {
	// TODO: fixme
	t.Skip()
	server := createTestServer()
	initializeServer(t, server)
	ctx := context.Background()

	tempDir, cleanupFunc := setupTestResources(t)
	defer cleanupFunc()

	root := models.Root{
		URI:  "file://" + tempDir,
		Name: "test-root",
	}
	err := server.AddRoot(root)
	require.NoError(t, err)

	listMsg := &protocol.Message{
		JSONRPC: "2.0",
		ID:      createRequestID(1),
		Method:  "resources/list",
		Params:  json.RawMessage(`{}`),
	}
	resp, err := server.HandleMessage(ctx, listMsg)
	require.NoError(t, err)

	var listResult struct {
		Resources []models.Resource `json:"resources"`
	}
	err = json.Unmarshal(resp.Result.(json.RawMessage), &listResult)
	require.NoError(t, err)
	assert.NotEmpty(t, listResult.Resources)

	resource := listResult.Resources[0]
	readMsg := &protocol.Message{
		JSONRPC: "2.0",
		ID:      createRequestID(2),
		Method:  "resources/read",
		Params: json.RawMessage(`{
			"uri": "` + resource.URI + `"
		}`),
	}
	resp, err = server.HandleMessage(ctx, readMsg)
	require.NoError(t, err)

	var readResult models.ReadResourceResult
	err = json.Unmarshal(resp.Result.(json.RawMessage), &readResult)
	require.NoError(t, err)
	assert.NotEmpty(t, readResult.Contents)
}

func TestResourceSubscriptions(t *testing.T) {
	server := createTestServer()
	initializeServer(t, server)
	ctx := context.Background()

	tempDir, cleanupFunc := setupTestResources(t)
	defer cleanupFunc()

	root := models.Root{
		URI:  "file://" + tempDir,
		Name: "test-root",
	}
	err := server.AddRoot(root)
	require.NoError(t, err)

	// List resources to get a resource URI
	listMsg := &protocol.Message{
		JSONRPC: "2.0",
		ID:      createRequestID(1),
		Method:  "resources/list",
		Params:  json.RawMessage(`{}`),
	}
	resp, err := server.HandleMessage(ctx, listMsg)
	require.NoError(t, err)

	var listResult struct {
		Resources []models.Resource `json:"resources"`
	}
	err = json.Unmarshal(resp.Result.(json.RawMessage), &listResult)
	require.NoError(t, err)
	assert.NotEmpty(t, listResult.Resources)

	resource := listResult.Resources[0]

	// Subscribe to resource
	subMsg := &protocol.Message{
		JSONRPC: "2.0",
		ID:      createRequestID(2),
		Method:  "resources/subscribe",
		Params: json.RawMessage(`{
			"uri": "` + resource.URI + `"
		}`),
	}
	_, err = server.HandleMessage(ctx, subMsg)
	require.NoError(t, err)

	notifications := make(chan protocol.Message, 1)
	go func() {
		select {
		case msg := <-server.Notifications():
			notifications <- msg
		case <-time.After(time.Second):
		}
	}()

	// Update the resource through the protocol
	updateMsg := &protocol.Message{
		JSONRPC: "2.0",
		ID:      createRequestID(3),
		Method:  "resources/update",
		Params: json.RawMessage(`{
			"uri": "` + resource.URI + `",
			"content": "updated content"
		}`),
	}
	_, err = server.HandleMessage(ctx, updateMsg)
	require.NoError(t, err)

	// Wait for notification
	select {
	case notification := <-notifications:
		assert.Equal(t, "notifications/resources/updated", notification.Method)

		var notif struct {
			Params struct {
				URI string `json:"uri"`
			} `json:"params"`
		}

		err = json.Unmarshal(notification.Params, &notif)
		require.NoError(t, err)
		assert.Equal(t, resource.URI, notif.Params.URI)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for notification")
	}

	// Verify the resource was actually updated
	readMsg := &protocol.Message{
		JSONRPC: "2.0",
		ID:      createRequestID(4),
		Method:  "resources/read",
		Params: json.RawMessage(`{
			"uri": "` + resource.URI + `"
		}`),
	}
	resp, err = server.HandleMessage(ctx, readMsg)
	require.NoError(t, err)

	var readResult models.ReadResourceResult
	err = json.Unmarshal(resp.Result.(json.RawMessage), &readResult)
	require.NoError(t, err)
	assert.Len(t, readResult.Contents, 1)
	textContent := readResult.Contents[0].(*models.TextResourceContents)
	assert.Equal(t, "updated content", textContent.Text)

	// Unsubscribe from resource
	unsubMsg := &protocol.Message{
		JSONRPC: "2.0",
		ID:      createRequestID(5),
		Method:  "resources/unsubscribe",
		Params: json.RawMessage(`{
			"uri": "` + resource.URI + `"
		}`),
	}
	_, err = server.HandleMessage(ctx, unsubMsg)
	require.NoError(t, err)
}

func TestErrorHandling(t *testing.T) {
	server := createTestServer()
	initializeServer(t, server)
	ctx := context.Background()

	invalidMsg := &protocol.Message{
		JSONRPC: "2.0",
		ID:      createRequestID(1),
		Method:  "invalid/method",
	}
	_, err := server.HandleMessage(ctx, invalidMsg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported method")

	invalidParamsMsg := &protocol.Message{
		JSONRPC: "2.0",
		ID:      createRequestID(2),
		Method:  "resources/read",
		Params:  json.RawMessage(`{invalid json`),
	}
	_, err = server.HandleMessage(ctx, invalidParamsMsg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid")

	nonExistentMsg := &protocol.Message{
		JSONRPC: "2.0",
		ID:      createRequestID(3),
		Method:  "resources/read",
		Params: json.RawMessage(`{
			"uri": "file:///nonexistent/resource"
		}`),
	}
	_, err = server.HandleMessage(ctx, nonExistentMsg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestSendNotification(t *testing.T) {
	server := createTestServer()

	// Create a notification
	notification := models.NewResourceUpdatedNotification("file:///test.txt")

	// Send notification
	server.sendNotification(notification)

	// Check notification was sent to channel
	select {
	case msg := <-server.notificationChan:
		assert.Equal(t, "notifications/resources/updated", msg.Method)

		// Parse params
		var params struct {
			URI string `json:"uri"`
		}
		err := json.Unmarshal(msg.Params, &params)
		require.NoError(t, err)

		assert.Equal(t, "file:///test.txt", params.URI)
	default:
		t.Fatal("Expected notification in channel but found none")
	}
}

func TestHandleListResources(t *testing.T) {
	server := createTestServer()
	ctx := context.Background()

	// Set up test resources
	tempDir, cleanupFunc := setupTestResources(t)
	defer cleanupFunc()

	// Add a root
	root := models.Root{
		URI:  "file://" + tempDir,
		Name: "TestRoot",
	}
	err := server.resourceManager.AddRoot(root)
	require.NoError(t, err)

	// Set server as initialized
	server.mu.Lock()
	server.initialized = true
	server.mu.Unlock()

	// Create list resources request
	requestID := createRequestID("req-1")
	msg := &protocol.Message{
		JSONRPC: "2.0",
		ID:      requestID,
		Method:  "resources/list",
		Params:  json.RawMessage("{}"),
	}

	// Handle the request
	response, err := server.HandleMessage(ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, response)

	// Verify response
	var result models.ListResourcesResult
	err = json.Unmarshal(response.Result.(json.RawMessage), &result)
	require.NoError(t, err)

	assert.NotEmpty(t, result.Resources)
	assert.Nil(t, result.NextCursor)
}

func TestHandleReadResource(t *testing.T) {
	server := createTestServer()
	ctx := context.Background()

	// Set up test resources
	tempDir, cleanupFunc := setupTestResources(t)
	defer cleanupFunc()

	// Add a root
	root := models.Root{
		URI:  "file://" + tempDir,
		Name: "TestRoot",
	}
	err := server.resourceManager.AddRoot(root)
	require.NoError(t, err)

	// Set server as initialized
	server.mu.Lock()
	server.initialized = true
	server.mu.Unlock()

	// Create read resource request
	requestID := createRequestID("req-1")
	params := struct {
		URI string `json:"uri"`
	}{
		URI: "file://" + filepath.Join(tempDir, "test1.txt"),
	}

	paramsBytes, err := json.Marshal(params)
	require.NoError(t, err)

	msg := &protocol.Message{
		JSONRPC: "2.0",
		ID:      requestID,
		Method:  "resources/read",
		Params:  json.RawMessage(paramsBytes),
	}

	// Handle the request
	response, err := server.HandleMessage(ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, response)

	// Verify response
	var result models.ReadResourceResult
	err = json.Unmarshal(response.Result.(json.RawMessage), &result)
	require.NoError(t, err)

	assert.NotEmpty(t, result.Contents)
	textContent, ok := result.Contents[0].(*models.TextResourceContents)
	require.True(t, ok)
	assert.Equal(t, "test content 1", textContent.Text)
}

func TestHandleUpdateResource(t *testing.T) {
	server := createTestServer()
	ctx := context.Background()

	// Set up test resources
	tempDir, cleanupFunc := setupTestResources(t)
	defer cleanupFunc()

	// Add a root
	root := models.Root{
		URI:  "file://" + tempDir,
		Name: "TestRoot",
	}
	err := server.resourceManager.AddRoot(root)
	require.NoError(t, err)

	// Set server as initialized

	// Create update resource request
	resourceURI := "file://" + filepath.Join(tempDir, "test1.txt")

	initialContent, err := server.resourceManager.ReadResource(ctx, resourceURI)
	require.NoError(t, err, "Resource should be found in resource manager")
	require.NotEmpty(t, initialContent, "Resource should have content")

	t.Logf("Initial resource content type: %T", initialContent[0])
	server.mu.Lock()
	server.initialized = true
	server.mu.Unlock()

	requestID := createRequestID("req-1")
	params := struct {
		URI     string `json:"uri"`
		Content string `json:"content"`
	}{
		URI:     resourceURI,
		Content: "updated test content",
	}

	paramsBytes, err := json.Marshal(params)
	require.NoError(t, err)

	msg := &protocol.Message{
		JSONRPC: "2.0",
		ID:      requestID,
		Method:  "resources/update",
		Params:  json.RawMessage(paramsBytes),
	}

	// Handle the request
	response, err := server.HandleMessage(ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, response)

	// Verify content was updated
	updatedContent, err := server.resourceManager.ReadResource(ctx, resourceURI)
	require.NoError(t, err)
	require.NotEmpty(t, updatedContent, "Read resource should return non-empty contents")
	// Debug the content received
	t.Logf("Updated resource content type: %T", updatedContent[0])
	t.Logf("Content length: %d", len(updatedContent))
	textContent, ok := updatedContent[0].(*models.TextResourceContents)
	require.True(t, ok)
	assert.Equal(t, "updated test content", textContent.Text)
}

func TestHandleSetLevel(t *testing.T) {
	server := createTestServer()
	initializeServer(t, server)
	ctx := context.Background()

	// Create set level request
	requestID := createRequestID("req-1")
	params := struct {
		Level models.LogLevel `json:"level"`
	}{
		Level: models.LogLevelDebug,
	}

	paramsBytes, err := json.Marshal(params)
	require.NoError(t, err)

	msg := &protocol.Message{
		JSONRPC: "2.0",
		ID:      requestID,
		Method:  "logging/setLevel",
		Params:  json.RawMessage(paramsBytes),
	}

	// Handle the request
	response, err := server.HandleMessage(ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, response)

	// Verify log level was set
	assert.Equal(t, models.LogLevelDebug, server.logManager.GetLevel())
}

func TestHandleListTools(t *testing.T) {
	server := createTestServer()
	ctx := context.Background()

	// Set server as initialized
	initializeServer(t, server)

	// Create list tools request
	requestID := createRequestID("req-1")
	msg := &protocol.Message{
		JSONRPC: "2.0",
		ID:      requestID,
		Method:  "tools/list",
		Params:  json.RawMessage("{}"),
	}

	// Handle the request
	response, err := server.HandleMessage(ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, response)

	// Parse result
	var result models.ListToolsResult
	err = json.Unmarshal(response.Result.(json.RawMessage), &result)
	require.NoError(t, err)

	// Tools might be empty in a new server, but the request should succeed
	assert.NotNil(t, result.Tools)
}

// func TestHandleCallTool(t *testing.T) {
// 	server := createTestServer()
// 	ctx := context.Background()
//
// 	// Set server as initialized
// 	initializeServer(t, server)
//
// 	// Register a test tool handler
// 	testTool := models.Tool{
// 		Name:        "test_echo",
// 		Description: "Echoes back the input",
// 		InputSchema: models.InputSchema{
// 			Type: "object",
// 			Properties: map[string]models.ParameterSchema{
// 				"message": {
// 					Type:        "string",
// 					Description: "Message to echo",
// 				},
// 			},
// 		},
// 	}
//
// 	// Add the tool to the tools manager
// 	// This assumes your implementation has a way to add tools directly
// 	// You might need to adapt this based on your actual implementation
// 	server.toolsManager.RegisterTestTool(testTool)
//
// 	// Create call tool request
// 	requestID := createRequestID("req-1")
// 	params := struct {
// 		Name      string                 `json:"name"`
// 		Arguments map[string]interface{} `json:"arguments"`
// 	}{
// 		Name: "test_echo",
// 		Arguments: map[string]interface{}{
// 			"message": "Hello, world!",
// 		},
// 	}
//
// 	paramsBytes, err := json.Marshal(params)
// 	require.NoError(t, err)
//
// 	msg := &protocol.Message{
// 		JSONRPC: "2.0",
// 		ID:      requestID,
// 		Method:  "tools/call",
// 		Params:  json.RawMessage(paramsBytes),
// 	}
//
// 	// Handle the request
// 	response, err := server.HandleMessage(ctx, msg)
// 	require.NoError(t, err)
// 	require.NotNil(t, response)
//
// 	// Parse result
// 	var result models.CallToolResult
// 	err = json.Unmarshal(response.Result.(json.RawMessage), &result)
// 	require.NoError(t, err)
//
// 	// Verify the response contains the echoed message
// 	assert.False(t, result.IsError)
// 	assert.NotEmpty(t, result.Content)
// }

func TestHandleComplete(t *testing.T) {
	server := createTestServer()

	ctx := context.Background()

	// Set up test resources
	tempDir, cleanupFunc := setupTestResources(t)
	defer cleanupFunc()

	completionFile := filepath.Join(tempDir, "completions.txt")
	err := os.WriteFile(completionFile, []byte("apple banana cherry date"), 0644)
	require.NoError(t, err)
	// Add a root with a file that has predictable content for completions
	root := models.Root{
		URI:  "file://" + tempDir,
		Name: "TestRoot",
	}
	err = server.resourceManager.AddRoot(root)
	require.NoError(t, err)

	resourceURI := "file://" + completionFile
	content, err := server.resourceManager.ReadResource(ctx, resourceURI)
	require.NoError(t, err, "Resource should be found in resource manager")
	require.NotEmpty(t, content, "Resource content should be available")

	// Set server as initialized
	initializeServer(t, server)

	// Create complete request with resource reference
	requestID := createRequestID("req-1")
	params := struct {
		Argument struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"argument"`
		Ref map[string]string `json:"ref"`
	}{
		Argument: struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		}{
			Name:  "word",
			Value: "ba",
		},
		Ref: map[string]string{
			"type": "ref/resource",
			"uri":  resourceURI,
		},
	}

	paramsBytes, err := json.Marshal(params)
	require.NoError(t, err)

	msg := &protocol.Message{
		JSONRPC: "2.0",
		ID:      requestID,
		Method:  "completion/complete",
		Params:  json.RawMessage(paramsBytes),
	}

	// Handle the request
	response, err := server.HandleMessage(ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, response)

	// Parse result
	var result models.CompleteResult
	err = json.Unmarshal(response.Result.(json.RawMessage), &result)
	require.NoError(t, err)

	// Verify the completion values
	assert.Contains(t, result.Completion.Values, "banana")
}

func TestHandleUnsupportedMethod(t *testing.T) {
	server := createTestServer()
	ctx := context.Background()

	// Set server as initialized
	initializeServer(t, server)

	// Create request with unsupported method
	requestID := createRequestID("req-1")
	msg := &protocol.Message{
		JSONRPC: "2.0",
		ID:      requestID,
		Method:  "unsupported/method",
		Params:  json.RawMessage("{}"),
	}

	// Handle the request
	_, err := server.HandleMessage(ctx, msg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported method")
}

func TestHandleInvalidParamsFormat(t *testing.T) {
	server := createTestServer()
	ctx := context.Background()

	// Set server as initialized
	initializeServer(t, server)

	// Create request with invalid JSON in params
	requestID := createRequestID("req-1")
	msg := &protocol.Message{
		JSONRPC: "2.0",
		ID:      requestID,
		Method:  "resources/read",
		Params:  json.RawMessage("{invalid-json}"),
	}

	// Handle the request
	_, err := server.HandleMessage(ctx, msg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid")
}

func TestHandleInvalidInitialize(t *testing.T) {
	server := createTestServer()
	ctx := context.Background()

	// Create initialize request with invalid params
	requestID := createRequestID("req-1")
	msg := &protocol.Message{
		JSONRPC: "2.0",
		ID:      requestID,
		Method:  "initialize",
		Params:  json.RawMessage("{invalid}"),
	}

	// Handle the initialize request
	_, err := server.HandleMessage(ctx, msg)
	assert.Error(t, err)
}

func TestDoubleInitialize(t *testing.T) {
	server := createTestServer()
	ctx := context.Background()

	// Create a valid initialize request
	initParams := struct {
		Capabilities    protocol.ClientCapabilities `json:"capabilities"`
		ClientInfo      models.Implementation       `json:"clientInfo"`
		ProtocolVersion string                      `json:"protocolVersion"`
	}{
		Capabilities: protocol.ClientCapabilities{},
		ClientInfo: models.Implementation{
			Name:    "TestClient",
			Version: "1.0.0",
		},
		ProtocolVersion: "1.0.0",
	}

	paramsBytes, err := json.Marshal(initParams)
	require.NoError(t, err)

	// First initialization
	requestID1 := createRequestID("req-1")
	msg1 := &protocol.Message{
		JSONRPC: "2.0",
		ID:      requestID1,
		Method:  "initialize",
		Params:  json.RawMessage(paramsBytes),
	}

	_, err = server.HandleMessage(ctx, msg1)
	require.NoError(t, err)

	// Second initialization attempt
	requestID2 := createRequestID("req-2")
	msg2 := &protocol.Message{
		JSONRPC: "2.0",
		ID:      requestID2,
		Method:  "initialize",
		Params:  json.RawMessage(paramsBytes),
	}

	_, err = server.HandleMessage(ctx, msg2)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already initialized")
}

func TestResourceNotFound(t *testing.T) {
	server := createTestServer()
	ctx := context.Background()

	// Set server as initialized
	initializeServer(t, server)

	// Create read request for nonexistent resource
	requestID := createRequestID("req-1")
	params := struct {
		URI string `json:"uri"`
	}{
		URI: "file:///nonexistent/resource.txt",
	}

	paramsBytes, err := json.Marshal(params)
	require.NoError(t, err)

	msg := &protocol.Message{
		JSONRPC: "2.0",
		ID:      requestID,
		Method:  "resources/read",
		Params:  json.RawMessage(paramsBytes),
	}

	// Handle the request
	_, err = server.HandleMessage(ctx, msg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestSendMultipleNotifications(t *testing.T) {
	server := createTestServer()

	// Send multiple notifications of different types
	notifications := []models.Notification{
		models.NewResourceUpdatedNotification("file:///test1.txt"),
		models.NewPromptListChangedNotification(),
		models.NewToolListChangedNotification(),
		models.NewLoggingMessageNotification(models.LogLevelInfo, "Test log message", "test-logger"),
	}

	for _, notification := range notifications {
		server.sendNotification(notification)
	}

	// Check all notifications were sent
	for i := 0; i < len(notifications); i++ {
		select {
		case msg := <-server.Notifications():
			// Just verify we received a notification with the expected method
			assert.Contains(t, msg.Method, "notifications/")
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("Expected notification %d in channel but found none", i)
		}
	}

	// Verify no more notifications
	select {
	case msg := <-server.Notifications():
		t.Fatalf("Unexpected extra notification: %s", msg.Method)
	case <-time.After(100 * time.Millisecond):
		// Expected, no more notifications
	}
}

func TestChannelFullNotification(t *testing.T) {
	// Create server with tiny notification channel
	server := &Server{
		info:             models.Implementation{Name: "TestServer", Version: "1.0"},
		version:          "1.0",
		resourceManager:  resource.NewManager(),
		toolsManager:     tools.NewToolsManager(),
		promptManager:    prompt.NewManager(),
		logManager:       logging.NewManager(),
		notificationChan: make(chan protocol.Message, 2), // Only 2 capacity
	}

	// Fill the channel
	server.notificationChan <- protocol.Message{Method: "test1"}
	server.notificationChan <- protocol.Message{Method: "test2"}

	// This should not block or panic
	notification := models.NewResourceUpdatedNotification("file:///test.txt")
	server.sendNotification(notification)

	// Verify the channel still has exactly 2 messages
	assert.Equal(t, 2, len(server.notificationChan))
}

func createTestServer() *Server {
	server := NewServer(models.Implementation{
		Name:    "test-server",
		Version: "1.0.0",
	}, "1.0")
	return server
}

func initializeServer(t *testing.T, server *Server) models.InitializeResult {
	ctx := context.Background()
	initParams := struct {
		Capabilities    protocol.ClientCapabilities `json:"capabilities"`
		ClientInfo      models.Implementation       `json:"clientInfo"`
		ProtocolVersion string                      `json:"protocolVersion"`
	}{
		Capabilities: protocol.ClientCapabilities{},
		ClientInfo: models.Implementation{
			Name:    "test-client",
			Version: "1.0",
		},
		ProtocolVersion: "1.0",
	}

	paramsBytes, err := json.Marshal(initParams)
	require.NoError(t, err)

	initMsg := &protocol.Message{
		JSONRPC: "2.0",
		ID:      createRequestID(1),
		Method:  "initialize",
		Params:  json.RawMessage(paramsBytes),
	}

	resp, err := server.HandleMessage(ctx, initMsg)
	require.NoError(t, err)

	var result models.InitializeResult
	err = json.Unmarshal(resp.Result.(json.RawMessage), &result)
	require.NoError(t, err)

	return result
}

func setupTestResources(t *testing.T) (string, func()) {
	tempDir, err := os.MkdirTemp("", "mcp-server-test-*")
	require.NoError(t, err)

	// files := map[string]string{
	// 	"test.txt":  "Hello, World!",
	// 	"test.json": `{"message": "Hello, JSON!"}`,
	// 	"test.md":   "# Hello Markdown!",
	// }
	//
	// for name, content := range files {
	// 	path := filepath.Join(tempDir, name)
	// 	err = os.WriteFile(path, []byte(content), 0644)
	// 	require.NoError(t, err)
	// }
	err = os.WriteFile(filepath.Join(tempDir, "test1.txt"), []byte("test content 1"), 0644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(tempDir, "test2.json"), []byte(`{"key": "value"}`), 0644)
	require.NoError(t, err)

	return tempDir, func() { os.RemoveAll(tempDir) }
}
