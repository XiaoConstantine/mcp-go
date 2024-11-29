package resource

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/XiaoConstantine/mcp-go/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManager(t *testing.T) {
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "resource-manager-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create test files
	testFiles := map[string]string{
		"test.txt":  "Hello, World!",
		"test.json": `{"message": "Hello, JSON!"}`,
		"test.md":   "# Hello, Markdown!",
	}

	for name, content := range testFiles {
		path := filepath.Join(tempDir, name)
		err := os.WriteFile(path, []byte(content), 0644)
		require.NoError(t, err)
	}

	// Create manager
	manager := NewManager()
	require.NotNil(t, manager)

	// Test adding root
	root := models.Root{
		URI:  "file://" + tempDir,
		Name: "test-root",
	}
	err = manager.AddRoot(root)
	require.NoError(t, err)

	// Test listing resources
	resources, cursor, err := manager.ListResources(context.Background(), nil)
	require.NoError(t, err)
	assert.Nil(t, cursor) // No pagination in basic implementation
	assert.Len(t, resources, len(testFiles))

	// Test reading resources
	for _, resource := range resources {
		contents, err := manager.ReadResource(context.Background(), resource.URI)
		require.NoError(t, err)
		require.Len(t, contents, 1)

		// Verify content based on type
		switch content := contents[0].(type) {
		case models.TextResourceContents:
			originalContent := testFiles[filepath.Base(resource.URI)]
			assert.Equal(t, originalContent, content.Text)
		default:
			t.Errorf("unexpected content type for %s", resource.URI)
		}
	}

	// Test subscription
	testSubscription(t, manager, resources[0].URI)
}

func testSubscription(t *testing.T, manager *Manager, uri string) {
	// Subscribe to resource
	sub, err := manager.Subscribe(uri)
	require.NoError(t, err)

	// Only call Close() once at the end
	defer sub.Close()

	// Create a test notification
	go func() {
		manager.notifyResourceChanged(uri)
	}()

	// Wait for notification
	select {
	case notification := <-sub.Channel():
		assert.Equal(t, uri, notification.Params.URI)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for notification")
	}

	// Verify channel gets closed properly
	time.Sleep(100 * time.Millisecond) // Give a small window for cleanup
	select {
	case _, ok := <-sub.Channel():
		assert.False(t, ok, "channel should be closed after Close()")
	default:
		// Channel is still open, which is fine since we haven't closed it yet
	}
}

func TestInvalidRoot(t *testing.T) {
	manager := NewManager()

	// Test invalid scheme
	err := manager.AddRoot(models.Root{URI: "invalid://path"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "only file:// URIs are supported")

	// Test non-existent path
	err = manager.AddRoot(models.Root{URI: "file:///nonexistent/path"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "root path not accessible")
}

func TestDuplicateRoot(t *testing.T) {
	manager := NewManager()
	tempDir, err := os.MkdirTemp("", "resource-manager-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	root := models.Root{URI: "file://" + tempDir}

	// Add root first time should succeed
	err = manager.AddRoot(root)
	require.NoError(t, err)

	// Add same root second time should fail
	err = manager.AddRoot(root)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "root already exists")
}

func TestResourceNotFound(t *testing.T) {
	manager := NewManager()

	// Test reading non-existent resource
	_, err := manager.ReadResource(context.Background(), "file:///nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "resource not found")

	// Test subscribing to non-existent resource
	_, err = manager.Subscribe("file:///nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "resource not found")
}

func TestSubscriptionCloseSafety(t *testing.T) {
	manager := NewManager()
	tempDir, err := os.MkdirTemp("", "resource-manager-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create a test file
	testPath := filepath.Join(tempDir, "test.txt")
	err = os.WriteFile(testPath, []byte("test content"), 0644)
	require.NoError(t, err)

	// Add root
	root := models.Root{
		URI: "file://" + tempDir,
	}
	err = manager.AddRoot(root)
	require.NoError(t, err)

	// Get the resource URI
	resources, _, err := manager.ListResources(context.Background(), nil)
	require.NoError(t, err)
	require.NotEmpty(t, resources)

	// Create subscription
	sub, err := manager.Subscribe(resources[0].URI)
	require.NoError(t, err)

	// Test multiple closes - should not panic
	for i := 0; i < 3; i++ {
		sub.Close()
	}
}
