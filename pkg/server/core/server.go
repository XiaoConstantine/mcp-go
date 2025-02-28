package core

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/XiaoConstantine/mcp-go/pkg/models"
	"github.com/XiaoConstantine/mcp-go/pkg/protocol"
	"github.com/XiaoConstantine/mcp-go/pkg/server/logging"
	"github.com/XiaoConstantine/mcp-go/pkg/server/prompt"
	"github.com/XiaoConstantine/mcp-go/pkg/server/resource"
	"github.com/XiaoConstantine/mcp-go/pkg/server/tools"
)

// Server represents the core MCP server, integrating various capabilities.
type Server struct {
	info             models.Implementation
	version          string
	initOnce         sync.Once
	initialized      bool
	mu               sync.RWMutex
	resourceManager  *resource.Manager
	capabilities     protocol.ServerCapabilities
	notificationChan chan protocol.Message

	toolsManager  *tools.ToolsManager
	promptManager *prompt.Manager
	logManager    *logging.Manager
}

// NewServer creates a new MCP server instance with the specified implementation details.
func NewServer(info models.Implementation, version string) *Server {
	server := &Server{
		info:             info,
		version:          version,
		resourceManager:  resource.NewManager(),
		toolsManager:     tools.NewToolsManager(),
		promptManager:    prompt.NewManager(),
		logManager:       logging.NewManager(),
		notificationChan: make(chan protocol.Message, 100),
		capabilities: protocol.ServerCapabilities{
			Resources: &protocol.ResourcesCapability{
				ListChanged: true,
				Subscribe:   true,
			},
			Prompts: &protocol.PromptsCapability{
				ListChanged: true,
			},
			Tools: &protocol.ToolsCapability{
				ListChanged: true,
			},
			Logging: map[string]interface{}{},
		},
	}
	server.logManager.SetSink(func(level models.LogLevel, data interface{}, logger string) {
		notification := models.NewLoggingMessageNotification(level, data, logger)
		server.sendNotification(notification)
	})

	return server
}

// IsInitialized returns whether the server has been initialized.
func (s *Server) IsInitialized() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.initialized
}

// HandleMessage processes incoming MCP messages and returns appropriate responses.
func (s *Server) HandleMessage(ctx context.Context, msg *protocol.Message) (*protocol.Message, error) {
	if msg.Method == "initialize" {
		return s.handleInitialize(ctx, msg)
	}

	if !s.IsInitialized() {
		return nil, fmt.Errorf("server not initialized")
	}

	switch msg.Method {
	case "completion/complete":
		return s.handleComplete(ctx, msg)
	case "ping":
		return s.handlePing(ctx, msg)
	case "resources/list":
		return s.handleListResources(ctx, msg)
	case "resources/read":
		return s.handleReadResource(ctx, msg)
	case "resources/update":
		return s.handleUpdateResource(ctx, msg)
	case "resources/subscribe":
		return s.handleResourceSubscribe(ctx, msg)
	case "resources/unsubscribe":
		return s.handleResourceUnsubscribe(ctx, msg)
	case "tools/list":
		return s.handleListTools(ctx, msg)
	case "tools/call":
		return s.handleCallTool(ctx, msg)
	case "prompts/list":
		return s.handleListPrompts(ctx, msg)
	case "prompts/get":
		return s.handleGetPrompt(ctx, msg)
	case "logging/setLevel":
		return s.handleSetLevel(ctx, msg)
	default:
		return nil, fmt.Errorf("unsupported method: %s", msg.Method)
	}
}

func (s *Server) createResponse(id *protocol.RequestID, result interface{}) (*protocol.Message, error) {
	resultBytes, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal response: %w", err)
	}

	return &protocol.Message{
		JSONRPC: "2.0",
		ID:      id,
		Result:  json.RawMessage(resultBytes),
	}, nil
}

func (s *Server) handleInitialize(ctx context.Context, msg *protocol.Message) (*protocol.Message, error) {
	var initialized bool
	var initErr error

	s.initOnce.Do(func() {
		var params struct {
			Capabilities    protocol.ClientCapabilities `json:"capabilities"`
			ClientInfo      models.Implementation       `json:"clientInfo"`
			ProtocolVersion string                      `json:"protocolVersion"`
		}

		if err := json.Unmarshal(msg.Params.(json.RawMessage), &params); err != nil {
			initErr = fmt.Errorf("invalid initialize params: %w", err)
			return
		}

		s.mu.Lock()
		s.initialized = true
		s.mu.Unlock()

		initialized = true
	})

	if initErr != nil {
		return nil, initErr
	}

	if !initialized {
		return nil, fmt.Errorf("server already initialized")
	}

	result := models.InitializeResult{
		Capabilities:    s.capabilities,
		ProtocolVersion: s.version,
		ServerInfo:      s.info,
		Instructions:    fmt.Sprintf("MCP Server %s - Ready for requests", s.version),
	}

	return s.createResponse(msg.ID, result)
}

func (s *Server) handlePing(ctx context.Context, msg *protocol.Message) (*protocol.Message, error) {
	return s.createResponse(msg.ID, struct{}{})
}

func (s *Server) handleListResources(ctx context.Context, msg *protocol.Message) (*protocol.Message, error) {
	var params struct {
		Cursor *models.Cursor `json:"cursor,omitempty"`
	}

	if err := json.Unmarshal(msg.Params.(json.RawMessage), &params); err != nil {
		return nil, fmt.Errorf("invalid list resources params: %w", err)
	}

	resources, nextCursor, err := s.resourceManager.ListResources(ctx, params.Cursor)
	if err != nil {
		return nil, fmt.Errorf("failed to list resources: %w", err)
	}

	result := models.ListResourcesResult{
		Resources:  resources,
		NextCursor: nextCursor,
	}

	return s.createResponse(msg.ID, result)
}

func (s *Server) handleReadResource(ctx context.Context, msg *protocol.Message) (*protocol.Message, error) {
	var params struct {
		URI string `json:"uri"`
	}

	if err := json.Unmarshal(msg.Params.(json.RawMessage), &params); err != nil {
		return nil, fmt.Errorf("invalid read resource params: %w", err)
	}

	contents, err := s.resourceManager.ReadResource(ctx, params.URI)
	if err != nil {
		return nil, fmt.Errorf("failed to read resource: %w", err)
	}

	result := models.ReadResourceResult{
		Contents: contents,
	}

	return s.createResponse(msg.ID, result)
}

func (s *Server) handleUpdateResource(ctx context.Context, msg *protocol.Message) (*protocol.Message, error) {
	var params struct {
		URI     string `json:"uri"`
		Content string `json:"content"`
	}

	if err := json.Unmarshal(msg.Params.(json.RawMessage), &params); err != nil {
		return nil, fmt.Errorf("invalid update resource params: %w", err)
	}

	if err := s.resourceManager.UpdateResource(params.URI, params.Content); err != nil {
		return nil, fmt.Errorf("failed to update resource: %w", err)
	}

	return s.createResponse(msg.ID, struct{}{})
}

func (s *Server) handleComplete(ctx context.Context, msg *protocol.Message) (*protocol.Message, error) {
	var params struct {
		Argument struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"argument"`
		Ref json.RawMessage `json:"ref"`
	}

	if err := json.Unmarshal(msg.Params.(json.RawMessage), &params); err != nil {
		return nil, fmt.Errorf("invalid complete params: %w", err)
	}

	// Determine if ref is a PromptReference or ResourceReference
	var refMap map[string]interface{}
	if err := json.Unmarshal(params.Ref, &refMap); err != nil {
		return nil, fmt.Errorf("invalid reference format: %w", err)
	}

	refType, ok := refMap["type"].(string)
	if !ok {
		return nil, fmt.Errorf("reference type not found")
	}

	var completionValues []string
	var hasMore bool
	var total int

	switch refType {
	case "ref/prompt":
		// Handle prompt completion
		name, ok := refMap["name"].(string)
		if !ok {
			return nil, fmt.Errorf("prompt name not found")
		}

		// If we have a prompt manager, use it to get completions
		if s.promptManager != nil {
			values, more, totalPtr, err := s.promptManager.GetCompletions(name, params.Argument.Name, params.Argument.Value)
			if err != nil {
				return nil, fmt.Errorf("failed to get prompt completions: %w", err)
			}
			completionValues = values
			hasMore = more
			if totalPtr != nil {
				total = *totalPtr
			} else {
				total = len(completionValues)
			}
		} else {
			// Fallback behavior when promptManager is not available
			completionValues = []string{}
			total = 0
		}

	case "ref/resource":
		// Handle resource completion
		uri, ok := refMap["uri"].(string)
		if !ok {
			return nil, fmt.Errorf("resource URI not found")
		}

		// Get potential completions from resource content
		resourceContent, err := s.resourceManager.ReadResource(ctx, uri)
		if err != nil {
			return nil, fmt.Errorf("failed to read resource for completions: %w", err)
		}

		// Extract text from resource
		if len(resourceContent) > 0 {
			if textContent, ok := resourceContent[0].(*models.TextResourceContents); ok {
				// Simple implementation: find words that start with the prefix
				words := strings.Fields(textContent.Text)
				prefix := strings.ToLower(params.Argument.Value)
				uniqueMatches := make(map[string]struct{})

				for _, word := range words {
					if strings.HasPrefix(strings.ToLower(word), prefix) {
						uniqueMatches[word] = struct{}{}
					}
				}

				// Convert matches to slice
				for word := range uniqueMatches {
					completionValues = append(completionValues, word)
				}

				total = len(completionValues)
			}
		}
	default:
		return nil, fmt.Errorf("unsupported reference type: %s", refType)
	}

	// Create result
	totalPtr := &total
	result := models.CompleteResult{}
	result.Completion.Values = completionValues
	result.Completion.HasMore = hasMore
	result.Completion.Total = totalPtr

	return s.createResponse(msg.ID, result)
}

func (s *Server) handleResourceSubscribe(ctx context.Context, msg *protocol.Message) (*protocol.Message, error) {
	var params struct {
		URI string `json:"uri"`
	}

	if err := json.Unmarshal(msg.Params.(json.RawMessage), &params); err != nil {
		return nil, fmt.Errorf("invalid subscribe params: %w", err)
	}

	sub, err := s.resourceManager.Subscribe(params.URI)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to resource: %w", err)
	}

	go s.forwardResourceNotifications(sub)

	return s.createResponse(msg.ID, struct{}{})
}

func (s *Server) handleResourceUnsubscribe(ctx context.Context, msg *protocol.Message) (*protocol.Message, error) {
	var params struct {
		URI string `json:"uri"`
	}

	if err := json.Unmarshal(msg.Params.(json.RawMessage), &params); err != nil {
		return nil, fmt.Errorf("invalid unsubscribe params: %w", err)
	}

	return s.createResponse(msg.ID, struct{}{})
}

func (s *Server) forwardResourceNotifications(sub *resource.Subscription) {
	for notification := range sub.Channel() {
		// Create the full notification message structure
		notifParams := struct {
			Params struct {
				URI string `json:"uri"`
			} `json:"params"`
		}{
			Params: struct {
				URI string `json:"uri"`
			}{
				URI: notification.Params.URI,
			},
		}

		notificationBytes, err := json.Marshal(notifParams)
		if err != nil {
			continue
		}

		msg := protocol.Message{
			JSONRPC: "2.0",
			Method:  "notifications/resources/updated",
			Params:  json.RawMessage(notificationBytes),
		}

		select {
		case s.notificationChan <- msg:
		default:
			// Channel is full, skip notification
		}
	}
}

// AddRoot adds a new root path for resource management.
func (s *Server) AddRoot(root models.Root) error {
	return s.resourceManager.AddRoot(root)
}

// Notifications returns the channel for receiving server notifications.
func (s *Server) Notifications() <-chan protocol.Message {
	return s.notificationChan
}

func (s *Server) handleListTools(ctx context.Context, msg *protocol.Message) (*protocol.Message, error) {
	var params struct {
		Cursor *models.Cursor `json:"cursor,omitempty"`
	}

	if err := json.Unmarshal(msg.Params.(json.RawMessage), &params); err != nil {
		return nil, fmt.Errorf("invalid list tools params: %w", err)
	}

	tools := s.toolsManager.ListTools()

	// Simple pagination - in a real implementation, this would be more sophisticated
	result := models.ListToolsResult{
		Tools: tools,
		// Add pagination logic as needed
	}

	return s.createResponse(msg.ID, result)
}

func (s *Server) handleCallTool(ctx context.Context, msg *protocol.Message) (*protocol.Message, error) {
	var params struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments,omitempty"`
	}

	if err := json.Unmarshal(msg.Params.(json.RawMessage), &params); err != nil {
		return nil, fmt.Errorf("invalid call tool params: %w", err)
	}

	toolResult, err := s.toolsManager.CallTool(params.Name, params.Arguments)
	if err != nil {
		// Return tool error as part of the result, not as a protocol error
		errorContent := models.TextContent{
			Type: "text",
			Text: fmt.Sprintf("Error calling tool: %v", err),
		}
		toolResult = &models.CallToolResult{
			Content: []models.Content{errorContent},
			IsError: true,
		}
	}

	return s.createResponse(msg.ID, toolResult)
}

func (s *Server) handleListPrompts(ctx context.Context, msg *protocol.Message) (*protocol.Message, error) {
	var params struct {
		Cursor *models.Cursor `json:"cursor,omitempty"`
	}

	if err := json.Unmarshal(msg.Params.(json.RawMessage), &params); err != nil {
		return nil, fmt.Errorf("invalid list prompts params: %w", err)
	}

	prompts, nextCursor, err := s.promptManager.ListPrompts(ctx, params.Cursor)
	if err != nil {
		return nil, fmt.Errorf("failed to list prompts: %w", err)
	}

	result := models.ListPromptsResult{
		Prompts:    prompts,
		NextCursor: nextCursor,
	}

	return s.createResponse(msg.ID, result)
}

func (s *Server) handleGetPrompt(ctx context.Context, msg *protocol.Message) (*protocol.Message, error) {
	var params struct {
		Name      string            `json:"name"`
		Arguments map[string]string `json:"arguments,omitempty"`
	}

	if err := json.Unmarshal(msg.Params.(json.RawMessage), &params); err != nil {
		return nil, fmt.Errorf("invalid get prompt params: %w", err)
	}

	messages, description, err := s.promptManager.GetPrompt(ctx, params.Name, params.Arguments)
	if err != nil {
		return nil, fmt.Errorf("failed to get prompt: %w", err)
	}

	result := models.GetPromptResult{
		Messages:    messages,
		Description: description,
	}

	return s.createResponse(msg.ID, result)
}

func (s *Server) handleSetLevel(ctx context.Context, msg *protocol.Message) (*protocol.Message, error) {
	var params struct {
		Level models.LogLevel `json:"level"`
	}

	if err := json.Unmarshal(msg.Params.(json.RawMessage), &params); err != nil {
		return nil, fmt.Errorf("invalid set level params: %w", err)
	}

	if !params.Level.IsValid() {
		return nil, fmt.Errorf("invalid log level: %s", params.Level)
	}

	s.logManager.SetLevel(params.Level)
	return s.createResponse(msg.ID, struct{}{})
}

// Add a SendLog method to send logging notifications.
func (s *Server) SendLog(level models.LogLevel, data interface{}, logger string) {
	notification := models.NewLoggingMessageNotification(level, data, logger)
	s.sendNotification(notification)
}

// sendNotification sends a notification through the notification channel.
func (s *Server) sendNotification(notification models.Notification) {
	notifMethod := notification.Method()

	// Convert notification to appropriate structure based on type
	var params interface{}

	switch n := notification.(type) {
	case *models.ResourceUpdatedNotification:
		params = struct {
			URI string `json:"uri"`
		}{
			URI: n.Params.URI,
		}
	case *models.LoggingMessageNotification:
		params = struct {
			Level  models.LogLevel `json:"level"`
			Data   interface{}     `json:"data"`
			Logger string          `json:"logger,omitempty"`
		}{
			Level:  n.Params.Level,
			Data:   n.Params.Data,
			Logger: n.Params.Logger,
		}
	case *models.PromptListChangedNotification:
		params = struct{}{}
	case *models.ToolListChangedNotification:
		params = struct{}{}
	case *models.ResourceListChangedNotification:
		params = struct{}{}
	default:
		// Handle unknown notification types
		s.logManager.Log(models.LogLevelWarning,
			fmt.Sprintf("Unknown notification type: %T", notification),
			"server")
		return
	}

	paramsBytes, err := json.Marshal(params)
	if err != nil {
		s.logManager.Log(models.LogLevelError,
			fmt.Sprintf("Failed to marshal notification params: %v", err),
			"server")
		return
	}

	msg := protocol.Message{
		JSONRPC: "2.0",
		Method:  notifMethod,
		Params:  json.RawMessage(paramsBytes),
	}

	select {
	case s.notificationChan <- msg:
		// Successfully sent notification
	default:
		// Channel is full, log warning and skip notification
		s.logManager.Log(models.LogLevelWarning,
			"Notification channel full, dropping notification",
			"server")
	}
}
