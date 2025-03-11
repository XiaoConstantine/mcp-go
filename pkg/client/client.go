package client

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/XiaoConstantine/mcp-go/pkg/errors"
	"github.com/XiaoConstantine/mcp-go/pkg/handler"
	"github.com/XiaoConstantine/mcp-go/pkg/logging"
	"github.com/XiaoConstantine/mcp-go/pkg/model"
	"github.com/XiaoConstantine/mcp-go/pkg/protocol"
	"github.com/XiaoConstantine/mcp-go/pkg/transport"
)

// Client represents an MCP client.
type Client struct {
	transport      transport.Transport
	logger         logging.Logger
	requestTracker *handler.RequestTracker
	notifChan      chan *protocol.Message

	capabilities    *protocol.ServerCapabilities
	serverInfo      *models.Implementation
	instructions    string
	protocolVersion string

	initialized   bool
	initializedMu sync.RWMutex

	clientInfo models.Implementation

	shutdown   bool
	shutdownMu sync.RWMutex

	msgHandlerWg sync.WaitGroup

	// Map to track requests that are currently being set up
	pendingSetup sync.Map
}

// Option is a function that configures a Client.
type Option func(*Client)

// WithLogger sets the logger for the client.
func WithLogger(logger logging.Logger) Option {
	return func(c *Client) {
		c.logger = logger
	}
}

// WithClientInfo sets the client info for the client.
func WithClientInfo(name, version string) Option {
	return func(c *Client) {
		c.clientInfo = models.Implementation{
			Name:    name,
			Version: version,
		}
	}
}

// NewClient creates a new MCP client.
func NewClient(t transport.Transport, options ...Option) *Client {
	c := &Client{
		transport:       t,
		logger:          logging.NewNoopLogger(),
		notifChan:       make(chan *protocol.Message, 100),
		clientInfo:      models.Implementation{Name: "mcp-go-client", Version: "2024-11-05"},
		protocolVersion: "2024-11-05",
	}

	// Apply options
	for _, option := range options {
		option(c)
	}

	// Initialize request tracker
	c.requestTracker = handler.NewRequestTracker(c.logger)

	// Start message handling goroutine
	c.msgHandlerWg.Add(1)
	go c.handleMessages()

	return c
}

// handleMessages processes incoming messages from the transport.
func (c *Client) handleMessages() {
	defer c.msgHandlerWg.Done()

	for {
		if c.isShutdown() {
			return
		}

		// Use a short timeout to allow checking shutdown status
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		msg, err := c.transport.Receive(ctx)
		cancel()

		if err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				// This is just the polling timeout, continue
				continue
			}

			c.logger.Error("Error receiving message", "error", err)
			if c.isShutdown() {
				return
			}

			// Sleep briefly to avoid spinning in case of persistent errors
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// Process the message based on type
		if msg.ID == nil && msg.Method != "" {
			// This is a notification
			select {
			case c.notifChan <- msg:
				// Notification sent successfully
			default:
				c.logger.Warn("Notification channel full, dropping notification", "method", msg.Method)
			}
		} else if msg.ID != nil {
			// This is a response to a request
			id := *msg.ID

			// First check if this response is for a request that's currently being set up
			if _, isPending := c.pendingSetup.Load(id); isPending {
				c.logger.Debug("Received response for request that's being set up, waiting briefly: %v", id)

				// Try with short retries to see if the request gets tracked
				for retries := 0; retries < 5; retries++ {
					// Check if request is now tracked
					if c.requestTracker.IsTracked(id) {
						c.logger.Debug("Request now tracked, processing response: %v", id)
						c.requestTracker.HandleResponse(msg)
						break
					}

					// Wait briefly before retrying
					time.Sleep(10 * time.Millisecond)
				}
			} else {
				// Normal processing
				c.requestTracker.HandleResponse(msg)
			}
		}
	}
}

// isShutdown returns whether the client is shut down.
func (c *Client) isShutdown() bool {
	c.shutdownMu.RLock()
	defer c.shutdownMu.RUnlock()
	return c.shutdown
}

// setShutdown sets the shutdown state of the client.
func (c *Client) setShutdown(shutdown bool) {
	c.shutdownMu.Lock()
	defer c.shutdownMu.Unlock()
	c.shutdown = shutdown
}

// isInitialized returns whether the client is initialized.
func (c *Client) isInitialized() bool {
	c.initializedMu.RLock()
	defer c.initializedMu.RUnlock()
	return c.initialized
}

// setInitialized sets the initialized state of the client.
func (c *Client) setInitialized(initialized bool) {
	c.initializedMu.Lock()
	defer c.initializedMu.Unlock()
	c.initialized = initialized
}

// sendRequest sends a request and waits for a response.
func (c *Client) sendRequest(ctx context.Context, method string, params interface{}) (*protocol.Message, error) {
	if c.isShutdown() {
		return nil, errors.ErrConnClosed
	}

	// Generate request ID
	id := c.requestTracker.NextID()

	// Mark this request as being set up to handle early responses
	c.pendingSetup.Store(id, true)
	c.logger.Debug("Setting up request ID: %v for method: %s", id, method)

	// Create request context
	reqCtx := c.requestTracker.TrackRequest(id, method)

	// Small delay to ensure tracking is fully set up
	time.Sleep(5 * time.Millisecond)
	c.logger.Debug("Request tracking setup complete for ID: %v, ready to send", id)

	// Create request message
	request := &protocol.Message{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  method,
	}

	// Add parameters if provided
	if params != nil {
		paramsBytes, err := json.Marshal(params)
		if err != nil {
			c.requestTracker.UntrackRequest(id)
			c.pendingSetup.Delete(id) // Clean up pending setup
			return nil, fmt.Errorf("failed to marshal parameters: %w", err)
		}
		request.Params = paramsBytes
	}

	// Send the request
	if err := c.transport.Send(ctx, request); err != nil {
		c.requestTracker.UntrackRequest(id)
		c.pendingSetup.Delete(id) // Clean up pending setup
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	// Wait for response
	resp, err := c.requestTracker.WaitForResponse(ctx, reqCtx)

	// Clean up pending setup tracking
	c.pendingSetup.Delete(id)

	return resp, err
}

// sendNotification sends a notification.
func (c *Client) sendNotification(ctx context.Context, method string, params interface{}) error {
	if c.isShutdown() {
		return errors.ErrConnClosed
	}

	// Create notification message
	notification := &protocol.Message{
		JSONRPC: "2.0",
		Method:  method,
	}

	// Add parameters if provided
	if params != nil {
		paramsBytes, err := json.Marshal(params)
		if err != nil {
			return fmt.Errorf("failed to marshal parameters: %w", err)
		}
		notification.Params = paramsBytes
	}

	// Send the notification
	return c.transport.Send(ctx, notification)
}

// Initialize initializes the client.
func (c *Client) Initialize(ctx context.Context) (*models.InitializeResult, error) {
	if c.isInitialized() {
		return nil, fmt.Errorf("client already initialized")
	}

	// Create initialize parameters
	params := struct {
		Capabilities    protocol.ClientCapabilities `json:"capabilities"`
		ClientInfo      models.Implementation       `json:"clientInfo"`
		ProtocolVersion string                      `json:"protocolVersion"`
	}{
		Capabilities: protocol.ClientCapabilities{
			Sampling: map[string]interface{}{},
		},
		ClientInfo:      c.clientInfo,
		ProtocolVersion: "0.1.0",
	}

	// Send initialize request
	response, err := c.sendRequest(ctx, "initialize", params)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize: %w", err)
	}

	resultBytes, err := json.Marshal(response.Result)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal result: %w", err)
	}
	// Parse response
	var result models.InitializeResult
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal initialize result: %w", err)
	}

	// Store server information
	c.capabilities = &result.Capabilities
	c.serverInfo = &result.ServerInfo
	c.instructions = result.Instructions
	c.protocolVersion = result.ProtocolVersion

	// Mark as initialized
	c.setInitialized(true)

	// Send initialized notification
	if err := c.sendNotification(ctx, "notifications/initialized", struct{}{}); err != nil {
		c.logger.Warn("Failed to send initialized notification", "error", err)
	}

	return &result, nil
}

// Shutdown shuts down the client.
func (c *Client) Shutdown() error {
	if c.isShutdown() {
		return nil
	}

	c.setShutdown(true)

	// Wait for message handler to finish
	done := make(chan struct{})
	go func() {
		c.msgHandlerWg.Wait()
		close(done)
	}()

	// Wait with timeout
	select {
	case <-done:
		// Handler finished
	case <-time.After(5 * time.Second):
		c.logger.Warn("Timed out waiting for message handler to finish")
	}

	// Close transport
	return c.transport.Close()
}

// Notifications returns a channel for receiving notifications.
func (c *Client) Notifications() <-chan *protocol.Message {
	return c.notifChan
}

// Ping sends a ping request to the server.
func (c *Client) Ping(ctx context.Context) error {
	if !c.isInitialized() {
		return errors.ErrNotInitialized
	}

	_, err := c.sendRequest(ctx, "ping", nil)
	return err
}

// ListResources requests a list of resources from the server.
func (c *Client) ListResources(ctx context.Context, cursor *models.Cursor) (*models.ListResourcesResult, error) {
	if !c.isInitialized() {
		return nil, errors.ErrNotInitialized
	}

	params := struct {
		Cursor *models.Cursor `json:"cursor,omitempty"`
	}{
		Cursor: cursor,
	}

	response, err := c.sendRequest(ctx, "resources/list", params)
	if err != nil {
		return nil, fmt.Errorf("failed to list resources: %w", err)
	}

	resultBytes, err := json.Marshal(response.Result)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal result: %w", err)
	}
	var result models.ListResourcesResult
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal list resources result: %w", err)
	}

	return &result, nil
}

// ReadResource requests the content of a resource from the server.
func (c *Client) ReadResource(ctx context.Context, uri string) (*models.ReadResourceResult, error) {
	if !c.isInitialized() {
		return nil, errors.ErrNotInitialized
	}

	params := struct {
		URI string `json:"uri"`
	}{
		URI: uri,
	}

	response, err := c.sendRequest(ctx, "resources/read", params)
	if err != nil {
		return nil, fmt.Errorf("failed to read resource: %w", err)
	}

	resultBytes, err := json.Marshal(response.Result)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal result: %w", err)
	}

	var result models.ReadResourceResult
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal read resource result: %w", err)
	}

	return &result, nil
}

// Subscribe subscribes to updates for a resource.
func (c *Client) Subscribe(ctx context.Context, uri string) error {
	if !c.isInitialized() {
		return errors.ErrNotInitialized
	}

	params := struct {
		URI string `json:"uri"`
	}{
		URI: uri,
	}

	_, err := c.sendRequest(ctx, "resources/subscribe", params)
	if err != nil {
		return fmt.Errorf("failed to subscribe to resource: %w", err)
	}

	return nil
}

// Unsubscribe unsubscribes from updates for a resource.
func (c *Client) Unsubscribe(ctx context.Context, uri string) error {
	if !c.isInitialized() {
		return errors.ErrNotInitialized
	}

	params := struct {
		URI string `json:"uri"`
	}{
		URI: uri,
	}

	_, err := c.sendRequest(ctx, "resources/unsubscribe", params)
	if err != nil {
		return fmt.Errorf("failed to unsubscribe from resource: %w", err)
	}

	return nil
}

// ListTools requests a list of tools from the server.
func (c *Client) ListTools(ctx context.Context, cursor *models.Cursor) (*models.ListToolsResult, error) {
	if !c.isInitialized() {
		return nil, errors.ErrNotInitialized
	}

	params := struct {
		Cursor *models.Cursor `json:"cursor,omitempty"`
	}{
		Cursor: cursor,
	}

	response, err := c.sendRequest(ctx, "tools/list", params)
	if err != nil {
		return nil, fmt.Errorf("failed to list tools: %w", err)
	}

	resultBytes, err := json.Marshal(response.Result)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal result: %w", err)
	}
	var result models.ListToolsResult
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal list tools result: %w", err)
	}

	return &result, nil
}

// CallTool calls a tool on the server.
func (c *Client) CallTool(ctx context.Context, name string, arguments map[string]interface{}) (*models.CallToolResult, error) {
	if !c.isInitialized() {
		return nil, errors.ErrNotInitialized
	}

	params := struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments,omitempty"`
	}{
		Name:      name,
		Arguments: arguments,
	}

	response, err := c.sendRequest(ctx, "tools/call", params)
	if err != nil {
		return nil, fmt.Errorf("failed to call tool: %w", err)
	}

	resultBytes, err := json.Marshal(response.Result)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal result: %w", err)
	}
	var result models.CallToolResult
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal call tool result: %w", err)
	}

	return &result, nil
}

// ListPrompts requests a list of prompts from the server.
func (c *Client) ListPrompts(ctx context.Context, cursor *models.Cursor) (*models.ListPromptsResult, error) {
	if !c.isInitialized() {
		return nil, errors.ErrNotInitialized
	}

	params := struct {
		Cursor *models.Cursor `json:"cursor,omitempty"`
	}{
		Cursor: cursor,
	}

	response, err := c.sendRequest(ctx, "prompts/list", params)
	if err != nil {
		return nil, fmt.Errorf("failed to list prompts: %w", err)
	}

	resultBytes, err := json.Marshal(response.Result)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal result: %w", err)
	}
	var result models.ListPromptsResult
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal list prompts result: %w", err)
	}

	return &result, nil
}

// GetPrompt requests a prompt from the server.
func (c *Client) GetPrompt(ctx context.Context, name string, arguments map[string]string) (*models.GetPromptResult, error) {
	if !c.isInitialized() {
		return nil, errors.ErrNotInitialized
	}

	params := struct {
		Name      string            `json:"name"`
		Arguments map[string]string `json:"arguments,omitempty"`
	}{
		Name:      name,
		Arguments: arguments,
	}

	response, err := c.sendRequest(ctx, "prompts/get", params)
	if err != nil {
		return nil, fmt.Errorf("failed to get prompt: %w", err)
	}

	var result models.GetPromptResult
	resultBytes, err := json.Marshal(response.Result)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal result: %w", err)
	}

	// Then unmarshal to desired type
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal get prompt result: %w", err)
	}

	return &result, nil
}

// SetLogLevel sets the logging level on the server.
func (c *Client) SetLogLevel(ctx context.Context, level models.LogLevel) error {
	if !c.isInitialized() {
		return errors.ErrNotInitialized
	}

	params := struct {
		Level models.LogLevel `json:"level"`
	}{
		Level: level,
	}

	_, err := c.sendRequest(ctx, "logging/setLevel", params)
	if err != nil {
		return fmt.Errorf("failed to set log level: %w", err)
	}

	return nil
}

// GetCapabilities returns the server capabilities.
func (c *Client) GetCapabilities() *protocol.ServerCapabilities {
	return c.capabilities
}

// GetServerInfo returns information about the server.
func (c *Client) GetServerInfo() *models.Implementation {
	return c.serverInfo
}

// GetInstructions returns the server instructions.
func (c *Client) GetInstructions() string {
	return c.instructions
}

// GetProtocolVersion returns the protocol version.
func (c *Client) GetProtocolVersion() string {
	return c.protocolVersion
}
