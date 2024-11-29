package tools

import (
	"testing"

	"github.com/XiaoConstantine/mcp-go/pkg/models"
)

// MockToolHandler implements ToolHandler for testing.
type MockToolHandler struct {
	tools []models.Tool
}

func (h *MockToolHandler) ListTools() ([]models.Tool, error) {
	return h.tools, nil
}

func (h *MockToolHandler) CallTool(name string, arguments map[string]interface{}) (*models.CallToolResult, error) {
	return &models.CallToolResult{
		Content: []models.Content{&models.TextContent{
			Type: "text",
			Text: "mock result",
		}},
	}, nil
}

func TestToolsManager_RegisterHandler(t *testing.T) {
	tm := NewToolsManager()
	mockHandler := &MockToolHandler{
		tools: []models.Tool{
			{
				Name:        "test",
				Description: "Test tool",
			},
		},
	}

	// Test registering a handler
	err := tm.RegisterHandler("mock", mockHandler)
	if err != nil {
		t.Errorf("Failed to register handler: %v", err)
	}

	// Test duplicate registration
	err = tm.RegisterHandler("mock", mockHandler)
	if err == nil {
		t.Error("Expected error on duplicate registration")
	}

	// Verify tool name prefixing
	tools := tm.ListTools()
	if len(tools) != 1 {
		t.Errorf("Expected 1 tool, got %d", len(tools))
	}
	if tools[0].Name != "mock_test" {
		t.Errorf("Expected tool name to be prefixed, got %s", tools[0].Name)
	}
}

func TestToolsManager_ListTools(t *testing.T) {
	tm := NewToolsManager()
	mockHandler := &MockToolHandler{
		tools: []models.Tool{
			{Name: "tool1"},
			{Name: "tool2"},
		},
	}

	err := tm.RegisterHandler("mock", mockHandler)
	if err != nil {
		t.Errorf("Failed to register handler: %v", err)
	}

	tools := tm.ListTools()
	if len(tools) != 2 {
		t.Errorf("Expected 2 tools, got %d", len(tools))
	}

	// Verify tool list is a copy
	tools[0].Name = "modified"
	originalTools := tm.ListTools()
	if originalTools[0].Name == "modified" {
		t.Error("Tool list should be a copy")
	}
}

func TestToolsManager_CallTool(t *testing.T) {
	tm := NewToolsManager()
	mockHandler := &MockToolHandler{
		tools: []models.Tool{
			{Name: "test"},
		},
	}

	err := tm.RegisterHandler("mock", mockHandler)
	if err != nil {
		t.Errorf("Failed to register handler: %v", err)
	}

	// Test valid call
	result, err := tm.CallTool("mock_test", map[string]interface{}{"arg": "value"})
	if err != nil {
		t.Errorf("Failed to call tool: %v", err)
	}
	if result == nil {
		t.Error("Expected result, got nil")
	}

	// Test invalid tool name format
	_, err = tm.CallTool("invalidname", nil)
	if err == nil {
		t.Error("Expected error for invalid tool name format")
	}

	// Test non-existent namespace
	_, err = tm.CallTool("nonexistent_test", nil)
	if err == nil {
		t.Error("Expected error for non-existent namespace")
	}
}

func TestToolsManager_UnregisterHandler(t *testing.T) {
	tm := NewToolsManager()
	mockHandler := &MockToolHandler{
		tools: []models.Tool{
			{Name: "test1"},
			{Name: "test2"},
		},
	}

	// Register and then unregister handler
	err := tm.RegisterHandler("mock", mockHandler)
	if err != nil {
		t.Errorf("Failed to register handler: %v", err)
	}

	err = tm.UnregisterHandler("mock")
	if err != nil {
		t.Errorf("Failed to unregister handler: %v", err)
	}

	// Verify tools were removed
	tools := tm.ListTools()
	if len(tools) != 0 {
		t.Errorf("Expected 0 tools after unregistering, got %d", len(tools))
	}

	// Test unregistering non-existent handler
	err = tm.UnregisterHandler("nonexistent")
	if err == nil {
		t.Error("Expected error when unregistering non-existent handler")
	}
}
