package tools

import (
	"testing"

	"github.com/XiaoConstantine/mcp-go/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockToolHandler implements the ToolHandler interface for testing.
type MockToolHandler struct {
	mock.Mock
}

func (m *MockToolHandler) ListTools() ([]models.Tool, error) {
	args := m.Called()
	return args.Get(0).([]models.Tool), args.Error(1)
}

func (m *MockToolHandler) CallTool(name string, arguments map[string]interface{}) (*models.CallToolResult, error) {
	args := m.Called(name, arguments)
	if res := args.Get(0); res != nil {
		return res.(*models.CallToolResult), args.Error(1)
	}
	return nil, args.Error(1)
}

func TestNewToolsManager(t *testing.T) {
	manager := NewToolsManager()
	assert.NotNil(t, manager)
	assert.Empty(t, manager.handlers)
	assert.Empty(t, manager.tools)
}

func TestRegisterHandler(t *testing.T) {
	manager := NewToolsManager()

	// Create mock handler with tools
	handler := new(MockToolHandler)
	tools := []models.Tool{
		{
			Name:        "testTool",
			Description: "A test tool",
			InputSchema: models.InputSchema{
				Type: "object",
			},
		},
	}
	handler.On("ListTools").Return(tools, nil)

	// Register handler
	err := manager.RegisterHandler("test", handler)
	require.NoError(t, err)

	// Verify handler was registered
	assert.Contains(t, manager.handlers, "test")

	// Verify tools were added with namespace prefix
	assert.Len(t, manager.tools, 1)
	assert.Equal(t, "test_testTool", manager.tools[0].Name)

	// Verify ListTools returns the registered tools
	listedTools := manager.ListTools()
	assert.Len(t, listedTools, 1)
	assert.Equal(t, "test_testTool", listedTools[0].Name)

	handler.AssertExpectations(t)
}

func TestRegisterHandlerAlreadyExists(t *testing.T) {
	manager := NewToolsManager()

	// Create mock handlers
	handler1 := new(MockToolHandler)
	handler2 := new(MockToolHandler)

	tools := []models.Tool{{Name: "tool1"}}
	handler1.On("ListTools").Return(tools, nil)

	// Register first handler
	err := manager.RegisterHandler("test", handler1)
	require.NoError(t, err)

	// Try to register second handler with same namespace
	err = manager.RegisterHandler("test", handler2)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")

	handler1.AssertExpectations(t)
}

func TestCallTool(t *testing.T) {
	manager := NewToolsManager()

	// Create mock handler
	handler := new(MockToolHandler)
	tools := []models.Tool{
		{
			Name:        "testTool",
			Description: "A test tool",
			InputSchema: models.InputSchema{
				Type: "object",
			},
		},
	}
	handler.On("ListTools").Return(tools, nil)

	// Expected result for tool call
	textContent := models.TextContent{
		Type: "text",
		Text: "Tool result",
	}
	expectedResult := &models.CallToolResult{
		Content: []models.Content{textContent},
		IsError: false,
	}

	// Setup expectations for CallTool
	handler.On("CallTool", "testTool", mock.Anything).Return(expectedResult, nil)

	// Register handler
	err := manager.RegisterHandler("test", handler)
	require.NoError(t, err)

	// Call tool with arguments
	args := map[string]interface{}{"key": "value"}
	result, err := manager.CallTool("test_testTool", args)
	require.NoError(t, err)

	// Verify result
	assert.Equal(t, expectedResult, result)

	handler.AssertExpectations(t)
}

func TestCallToolInvalidFormat(t *testing.T) {
	manager := NewToolsManager()

	// Try to call tool with invalid name format (no namespace separator)
	_, err := manager.CallTool("invalid", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid tool name format")
}

func TestCallToolNamespaceNotFound(t *testing.T) {
	manager := NewToolsManager()

	// Try to call tool with nonexistent namespace
	_, err := manager.CallTool("nonexistent_tool", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no handler registered for namespace")
}

func TestUnregisterHandler(t *testing.T) {
	manager := NewToolsManager()

	// Create mock handler with tools
	handler := new(MockToolHandler)
	tools := []models.Tool{
		{Name: "tool1"},
		{Name: "tool2"},
	}
	handler.On("ListTools").Return(tools, nil)

	// Register handler
	err := manager.RegisterHandler("test", handler)
	require.NoError(t, err)

	// Verify tools were added
	assert.Len(t, manager.tools, 2)

	// Unregister handler
	err = manager.UnregisterHandler("test")
	require.NoError(t, err)

	// Verify handler and tools were removed
	assert.NotContains(t, manager.handlers, "test")
	assert.Empty(t, manager.tools)

	handler.AssertExpectations(t)
}

func TestUnregisterHandlerNotFound(t *testing.T) {
	manager := NewToolsManager()

	// Try to unregister nonexistent handler
	err := manager.UnregisterHandler("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no handler registered")
}
