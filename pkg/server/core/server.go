package core

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/XiaoConstantine/mcp-go/pkg/models"
	"github.com/XiaoConstantine/mcp-go/pkg/protocol"
	"github.com/XiaoConstantine/mcp-go/pkg/server/resource"
)

// Server represents the core MCP server, integrating various capabilities
type Server struct {
	info             models.Implementation
	version          string
	initOnce         sync.Once
	initialized      bool
	mu               sync.RWMutex
	resourceManager  *resource.Manager
	capabilities     protocol.ServerCapabilities
	notificationChan chan protocol.Message
}

// NewServer creates a new MCP server instance with the specified implementation details
func NewServer(info models.Implementation, version string) *Server {
	return &Server{
		info:             info,
		version:          version,
		resourceManager:  resource.NewManager(),
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
}

// IsInitialized returns whether the server has been initialized
func (s *Server) IsInitialized() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.initialized
}

// HandleMessage processes incoming MCP messages and returns appropriate responses
func (s *Server) HandleMessage(ctx context.Context, msg *protocol.Message) (*protocol.Message, error) {
	if msg.Method == "initialize" {
		return s.handleInitialize(ctx, msg)
	}

	if !s.IsInitialized() {
		return nil, fmt.Errorf("server not initialized")
	}

	switch msg.Method {
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

// AddRoot adds a new root path for resource management
func (s *Server) AddRoot(root models.Root) error {
	return s.resourceManager.AddRoot(root)
}

// Notifications returns the channel for receiving server notifications
func (s *Server) Notifications() <-chan protocol.Message {
	return s.notificationChan
}
