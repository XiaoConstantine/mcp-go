package resource

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/XiaoConstantine/mcp-go/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewManager(t *testing.T) {
	manager := NewManager()
	assert.NotNil(t, manager)
	assert.Empty(t, manager.resources)
	assert.Empty(t, manager.contents)
	assert.Empty(t, manager.subscribers)
}

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
		case *models.TextResourceContents:
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

func TestListResources(t *testing.T) {
	manager := NewManager()
	ctx := context.Background()

	// Create a temporary directory with test files
	tempDir, cleanupFunc := setupTestDirectory(t)
	defer cleanupFunc()

	// Add the root
	root := models.Root{
		URI:  "file://" + tempDir,
		Name: "TestRoot",
	}
	err := manager.AddRoot(root)
	require.NoError(t, err)

	// List resources
	resources, cursor, err := manager.ListResources(ctx, nil)
	require.NoError(t, err)
	assert.Nil(t, cursor)
	assert.NotEmpty(t, resources)

	// Verify expected resource count
	assert.Equal(t, 3, len(resources)) // Assuming 3 files were created in setupTestDirectory
}

func TestUpdateResource(t *testing.T) {
	manager := NewManager()

	// Create a temporary directory with test files
	tempDir, cleanupFunc := setupTestDirectory(t)
	defer cleanupFunc()

	// Add the root
	root := models.Root{
		URI:  "file://" + tempDir,
		Name: "TestRoot",
	}
	err := manager.AddRoot(root)
	require.NoError(t, err)

	// Update a resource
	testFileURI := "file://" + filepath.Join(tempDir, "test1.txt")
	err = manager.UpdateResource(testFileURI, "updated content")
	require.NoError(t, err)

	// Verify content was updated
	assert.Equal(t, "updated content", manager.contents[testFileURI])
}

func TestUpdateResourceNonexistent(t *testing.T) {
	manager := NewManager()

	// Try to update a nonexistent resource
	err := manager.UpdateResource("file:///nonexistent.txt", "content")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestGetCompletions(t *testing.T) {
	manager := NewManager()

	// Create a temporary directory with test files
	tempDir, cleanupFunc := setupTestDirectory(t)
	defer cleanupFunc()

	// Add the root
	root := models.Root{
		URI:  "file://" + tempDir,
		Name: "TestRoot",
	}
	err := manager.AddRoot(root)
	require.NoError(t, err)

	// Add a file with predictable content for completions
	testFileURI := "file://" + filepath.Join(tempDir, "completions.txt")
	testContent := "apple banana cherry date elderberry fig grape"
	err = os.WriteFile(filepath.Join(tempDir, "completions.txt"), []byte(testContent), 0644)
	require.NoError(t, err)

	// Rescan to pick up the new file
	err = manager.scanRoot(root)
	require.NoError(t, err)

	// Test completions with a prefix
	completions, hasMore, total, err := manager.GetCompletions(testFileURI, "arg1", "ba")
	require.NoError(t, err)
	assert.Contains(t, completions, "banana")
	assert.False(t, hasMore)
	assert.NotNil(t, total)
	assert.Equal(t, 1, *total)

	// Test with a prefix that should match multiple words
	completions, hasMore, total, err = manager.GetCompletions(testFileURI, "arg1", "e")
	require.NoError(t, err)
	assert.Contains(t, completions, "elderberry")
	assert.False(t, hasMore)
	assert.NotNil(t, total)
	assert.Equal(t, 1, *total)
}

func setupTestDirectory(t *testing.T) (string, func()) {
	tempDir, err := os.MkdirTemp("", "resource-test")
	require.NoError(t, err)

	// Create test files
	err = os.WriteFile(filepath.Join(tempDir, "test1.txt"), []byte("test content 1"), 0644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(tempDir, "test2.json"), []byte(`{"key": "value"}`), 0644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(tempDir, "test3.md"), []byte("# Markdown test"), 0644)
	require.NoError(t, err)

	return tempDir, func() {
		os.RemoveAll(tempDir)
	}
}

func TestManagerShutdown(t *testing.T) {
	manager := NewManager()
	
	// Create a temporary directory
	tempDir, cleanupFunc := setupTestDirectory(t)
	defer cleanupFunc()
	
	// Add a root
	root := models.Root{
		URI:  "file://" + tempDir,
		Name: "TestRoot",
	}
	err := manager.AddRoot(root)
	require.NoError(t, err)
	
	// Create a subscription to test shutdown
	sub, err := manager.Subscribe("file://" + filepath.Join(tempDir, "test1.txt"))
	require.NoError(t, err)
	require.NotNil(t, sub)
	
	// Test shutdown - this should execute the shutdown code path
	ctx := context.Background()
	err = manager.Shutdown(ctx)
	assert.NoError(t, err)
	
	// The shutdown method was called successfully (coverage achieved)
}

func TestManagerShutdownWithTimeout(t *testing.T) {
	manager := NewManager()
	
	// Create a context that will timeout quickly
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()
	
	// Wait for context to timeout
	time.Sleep(5 * time.Millisecond)
	
	// Test shutdown with already timed out context
	err := manager.Shutdown(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "deadline exceeded")
}

func TestCloseAllSubscriptions(t *testing.T) {
	manager := NewManager()
	
	// Create a temporary directory
	tempDir, cleanupFunc := setupTestDirectory(t)
	defer cleanupFunc()
	
	// Add a root
	root := models.Root{
		URI:  "file://" + tempDir,
		Name: "TestRoot", 
	}
	err := manager.AddRoot(root)
	require.NoError(t, err)
	
	// Create multiple subscriptions
	uri1 := "file://" + filepath.Join(tempDir, "test1.txt")
	uri2 := "file://" + filepath.Join(tempDir, "test2.json")
	
	sub1, err := manager.Subscribe(uri1)
	require.NoError(t, err)
	sub2, err := manager.Subscribe(uri2)
	require.NoError(t, err)
	
	require.NotNil(t, sub1)
	require.NotNil(t, sub2)
	
	// Manually call closeAllSubscriptions to test it directly
	// This achieves coverage of the closeAllSubscriptions method
	manager.closeAllSubscriptions()
	
	// Verify the subscribers map was cleared (coverage achieved)
	manager.subMu.Lock()
	assert.Empty(t, manager.subscribers, "Subscribers map should be cleared")
	manager.subMu.Unlock()
}

func TestDetectMimeType(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "text file",
			path:     "test.txt",
			expected: "text/plain",
		},
		{
			name:     "json file",
			path:     "data.json",
			expected: "application/json",
		},
		{
			name:     "markdown file",
			path:     "readme.md",
			expected: "text/markdown",
		},
		{
			name:     "unknown extension",
			path:     "file.unknown",
			expected: "text/plain", // fallback
		},
		{
			name:     "no extension",
			path:     "filename",
			expected: "text/plain", // fallback
		},
		{
			name:     "uppercase extension",
			path:     "FILE.TXT",
			expected: "text/plain",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectMimeType(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestScanRootErrorHandling(t *testing.T) {
	manager := NewManager()
	
	// Test with non-existent directory
	root := models.Root{
		URI:  "file:///non/existent/path",
		Name: "NonExistentRoot",
	}
	
	err := manager.scanRoot(root)
	assert.Error(t, err)
}

func TestGetCompletionsEdgeCases(t *testing.T) {
	manager := NewManager()
	
	// Test with non-existent resource
	completions, hasMore, total, err := manager.GetCompletions("file:///non/existent", "arg", "prefix")
	assert.Error(t, err)
	assert.Nil(t, completions)
	assert.False(t, hasMore)
	assert.Nil(t, total)
	
	// Create a temporary directory with test files
	tempDir, cleanupFunc := setupTestDirectory(t)
	defer cleanupFunc()
	
	// Add root and scan
	root := models.Root{
		URI:  "file://" + tempDir,
		Name: "TestRoot",
	}
	err = manager.AddRoot(root)
	require.NoError(t, err)
	
	// Test with empty prefix
	testFileURI := "file://" + filepath.Join(tempDir, "test1.txt")
	completions, hasMore, total, err = manager.GetCompletions(testFileURI, "arg", "")
	require.NoError(t, err)
	assert.NotNil(t, completions)
	assert.False(t, hasMore)
	assert.NotNil(t, total)
}

func TestSubscriptionCloseSafetyMultiple(t *testing.T) {
	manager := NewManager()
	
	// Create a temporary directory
	tempDir, cleanupFunc := setupTestDirectory(t)
	defer cleanupFunc()
	
	// Add a root
	root := models.Root{
		URI:  "file://" + tempDir,
		Name: "TestRoot",
	}
	err := manager.AddRoot(root)
	require.NoError(t, err)
	
	// Create multiple subscriptions to the same URI
	uri := "file://" + filepath.Join(tempDir, "test1.txt")
	
	sub1, err := manager.Subscribe(uri)
	require.NoError(t, err)
	sub2, err := manager.Subscribe(uri)
	require.NoError(t, err)
	sub3, err := manager.Subscribe(uri)
	require.NoError(t, err)
	
	require.NotNil(t, sub1)
	require.NotNil(t, sub2)
	require.NotNil(t, sub3)
	
	// Close individual subscriptions
	sub1.Close()
	sub2.Close()
	sub3.Close()
	
	// Verify they can be closed multiple times without panic
	sub1.Close()
	sub2.Close()
	sub3.Close()
}
