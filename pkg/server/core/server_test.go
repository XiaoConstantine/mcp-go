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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createRequestID(id interface{}) *protocol.RequestID {
	reqID := protocol.RequestID(id)
	return &reqID
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
	server := createInitializedServer(t)
	ctx := context.Background()

	tempDir := setupTestResources(t)
	defer os.RemoveAll(tempDir)

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
	server := createInitializedServer(t)
	ctx := context.Background()

	tempDir := setupTestResources(t)
	defer os.RemoveAll(tempDir)

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
		err = json.Unmarshal(notification.Params.(json.RawMessage), &notif)
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
	server := createInitializedServer(t)
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

func createInitializedServer(t *testing.T) *Server {
	server := NewServer(models.Implementation{
		Name:    "test-server",
		Version: "1.0.0",
	}, "1.0")
	initializeServer(t, server)
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

func setupTestResources(t *testing.T) string {
	tempDir, err := os.MkdirTemp("", "mcp-server-test-*")
	require.NoError(t, err)

	files := map[string]string{
		"test.txt":  "Hello, World!",
		"test.json": `{"message": "Hello, JSON!"}`,
		"test.md":   "# Hello Markdown!",
	}

	for name, content := range files {
		path := filepath.Join(tempDir, name)
		err = os.WriteFile(path, []byte(content), 0644)
		require.NoError(t, err)
	}

	return tempDir
}
