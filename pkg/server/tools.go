package server

import (
	"fmt"
	"strings"
	"sync"

	"github.com/XiaoConstantine/mcp-go/pkg/models"
)

// ToolHandler represents a handler for tool-related operations
type ToolHandler interface {
	ListTools() ([]models.Tool, error)
	CallTool(name string, arguments map[string]interface{}) (*models.CallToolResult, error)
}

// ToolsManager manages tool registration and execution
type ToolsManager struct {
	mu       sync.RWMutex
	handlers map[string]ToolHandler
	tools    []models.Tool
}

// NewToolsManager creates a new tools manager
func NewToolsManager() *ToolsManager {
	return &ToolsManager{
		handlers: make(map[string]ToolHandler),
		tools:    make([]models.Tool, 0),
	}
}

// RegisterHandler registers a tool handler with a specific namespace
func (tm *ToolsManager) RegisterHandler(namespace string, handler ToolHandler) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if _, exists := tm.handlers[namespace]; exists {
		return fmt.Errorf("handler already registered for namespace: %s", namespace)
	}

	// Get tools from the handler
	tools, err := handler.ListTools()
	if err != nil {
		return fmt.Errorf("failed to list tools for namespace %s: %v", namespace, err)
	}

	// Add namespace prefix to tool names if not already present
	for i := range tools {
		if !strings.HasPrefix(tools[i].Name, namespace+"_") {
			tools[i].Name = namespace + "_" + tools[i].Name
		}
	}

	tm.handlers[namespace] = handler
	tm.tools = append(tm.tools, tools...)
	return nil
}

// ListTools returns all registered tools
func (tm *ToolsManager) ListTools() []models.Tool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	toolsCopy := make([]models.Tool, len(tm.tools))
	copy(toolsCopy, tm.tools)
	return toolsCopy
}

// CallTool calls a specific tool by name with the provided arguments
func (tm *ToolsManager) CallTool(name string, arguments map[string]interface{}) (*models.CallToolResult, error) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	// Find the namespace and tool name
	parts := strings.SplitN(name, "_", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid tool name format: %s", name)
	}

	namespace, toolName := parts[0], parts[1]
	handler, exists := tm.handlers[namespace]
	if !exists {
		return nil, fmt.Errorf("no handler registered for namespace: %s", namespace)
	}

	// Call the tool through the handler
	return handler.CallTool(toolName, arguments)
}

// UnregisterHandler removes a tool handler and its tools
func (tm *ToolsManager) UnregisterHandler(namespace string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if _, exists := tm.handlers[namespace]; !exists {
		return fmt.Errorf("no handler registered for namespace: %s", namespace)
	}

	delete(tm.handlers, namespace)

	// Remove tools associated with this namespace
	newTools := make([]models.Tool, 0)
	prefix := namespace + "_"
	for _, tool := range tm.tools {
		if !strings.HasPrefix(tool.Name, prefix) {
			newTools = append(newTools, tool)
		}
	}
	tm.tools = newTools

	return nil
}
