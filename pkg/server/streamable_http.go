package server

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/XiaoConstantine/mcp-go/pkg/logging"
	"github.com/XiaoConstantine/mcp-go/pkg/protocol"
	"github.com/XiaoConstantine/mcp-go/pkg/server/core"
	"github.com/XiaoConstantine/mcp-go/pkg/transport"
)

// StreamableHTTPServer implements an HTTP server that supports the Model Context Protocol
// using the Streamable HTTP transport specification.
type StreamableHTTPServer struct {
	// The core MCP server that handles messages
	mcpServer core.MCPServer

	// Transport for handling HTTP requests/responses and SSE
	transport *transport.StreamableHTTPTransport

	// Configuration options
	config StreamableHTTPServerConfig

	// Logger for server operations
	logger logging.Logger

	// Server state
	mutex      sync.RWMutex
	isRunning  bool
	httpServer *http.Server

	// Context and cancellation for shutdown coordination
	ctx        context.Context
	cancelFunc context.CancelFunc

	// Worker pool for handling messages
	messageQueue chan *protocol.Message
	workerPool   sync.WaitGroup
	maxWorkers   int
}

// StreamableHTTPServerConfig contains configuration options for the StreamableHTTP server.
type StreamableHTTPServerConfig struct {
	// Address to listen on, e.g., ":8080"
	ListenAddr string

	// Path to serve the MCP endpoint at, e.g., "/mcp"
	MCPPath string

	// Whether to enable CORS
	EnableCORS bool

	// Logger for server operations
	Logger logging.Logger

	// Session configuration
	RequireSession bool
	
	// Timeout values
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration

	// How often to send keepalive messages on SSE connections
	KeepAliveInterval time.Duration

	// Maximum number of worker goroutines for processing messages
	MaxWorkers int
}

// DefaultStreamableHTTPServerConfig returns a default configuration for the server.
func DefaultStreamableHTTPServerConfig() StreamableHTTPServerConfig {
	return StreamableHTTPServerConfig{
		ListenAddr:        ":8080",
		MCPPath:           "/mcp",
		EnableCORS:        true,
		Logger:            &logging.NoopLogger{},
		RequireSession:    false,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		KeepAliveInterval: 30 * time.Second,
		MaxWorkers:        10, // Default worker pool size
	}
}

// NewStreamableHTTPServer creates a new server instance with the given configuration.
func NewStreamableHTTPServer(mcpServer core.MCPServer, config StreamableHTTPServerConfig) *StreamableHTTPServer {
	if config.Logger == nil {
		config.Logger = &logging.NoopLogger{}
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Create the transport with appropriate configuration
	transportConfig := &transport.StreamableHTTPTransportConfig{
		Logger:            config.Logger,
		AllowUpgrade:      true,
		RequireSession:    config.RequireSession,
		KeepAliveInterval: config.KeepAliveInterval,
	}
	
	// Set default MaxWorkers if not specified
	if config.MaxWorkers <= 0 {
		config.MaxWorkers = 10
	}

	// Initialize server
	server := &StreamableHTTPServer{
		mcpServer:    mcpServer,
		transport:    transport.NewStreamableHTTPTransport(transportConfig),
		config:       config,
		logger:       config.Logger,
		ctx:          ctx,
		cancelFunc:   cancel,
		messageQueue: make(chan *protocol.Message, config.MaxWorkers*2), // Buffer size: 2x workers
		maxWorkers:   config.MaxWorkers,
	}

	return server
}

// Start begins serving HTTP requests.
func (s *StreamableHTTPServer) Start() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.isRunning {
		return fmt.Errorf("server is already running")
	}

	// Create router/mux for handling requests
	mux := http.NewServeMux()
	
	// Set up the main MCP endpoint handler
	mux.HandleFunc(s.config.MCPPath, s.handleMCPRequest)

	// Configure HTTP server
	s.httpServer = &http.Server{
		Addr:         s.config.ListenAddr,
		Handler:      mux,
		ReadTimeout:  s.config.ReadTimeout,
		WriteTimeout: s.config.WriteTimeout,
		IdleTimeout:  s.config.IdleTimeout,
	}

	// Start processing notifications from the MCP server
	go s.processNotifications()

	// Start worker pool for handling messages
	s.startWorkerPool()

	// Start processing messages from the transport
	go s.processMessages()

	// Start HTTP server in a goroutine
	go func() {
		s.logger.Info("Starting Streamable HTTP server on %s", s.config.ListenAddr)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("HTTP server error: %v", err)
		}
	}()

	s.isRunning = true
	return nil
}

// Stop gracefully shuts down the server.
func (s *StreamableHTTPServer) Stop() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if !s.isRunning {
		return nil
	}

	s.logger.Info("Shutting down Streamable HTTP server...")

	// Cancel the context to signal shutdown
	s.cancelFunc()

	// Create a timeout context for graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Shutdown the HTTP server
	if err := s.httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("error shutting down HTTP server: %w", err)
	}

	// Close the transport
	if err := s.transport.Close(); err != nil {
		return fmt.Errorf("error closing transport: %w", err)
	}

	// Close message queue and wait for workers to finish
	close(s.messageQueue)
	s.workerPool.Wait()
	s.logger.Debug("All worker goroutines have stopped")

	// Signal the MCP server to shut down
	if err := s.mcpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("error shutting down MCP server: %w", err)
	}

	s.isRunning = false
	s.logger.Info("Streamable HTTP server has been shut down")
	return nil
}

// handleMCPRequest processes all MCP requests (both POST and GET).
func (s *StreamableHTTPServer) handleMCPRequest(w http.ResponseWriter, r *http.Request) {
	// Use the StreamableHTTPTransport to handle the request
	s.transport.HandleRequest(w, r)
}

// processNotifications forwards notifications from the MCP server to connected clients.
func (s *StreamableHTTPServer) processNotifications() {
	notificationCh := s.mcpServer.Notifications()

	for {
		select {
		case <-s.ctx.Done():
			// Server is shutting down
			return
			
		case notification, ok := <-notificationCh:
			if !ok {
				// Channel closed
				return
			}
			
			// Forward the notification via the transport
			err := s.transport.Send(s.ctx, &notification)
			if err != nil {
				s.logger.Error("Failed to send notification: %v", err)
			}
		}
	}
}

// startWorkerPool initializes and starts the worker pool for handling messages.
func (s *StreamableHTTPServer) startWorkerPool() {
	for i := 0; i < s.maxWorkers; i++ {
		s.workerPool.Add(1)
		go func(workerID int) {
			defer s.workerPool.Done()
			s.logger.Debug("Worker %d started", workerID)
			
			for {
				select {
				case <-s.ctx.Done():
					s.logger.Debug("Worker %d stopping due to context cancellation", workerID)
					return
				case msg := <-s.messageQueue:
					if msg != nil {
						s.handleMessage(msg)
					}
				}
			}
		}(i)
	}
}

// processMessages continuously receives messages from the transport and processes them.
func (s *StreamableHTTPServer) processMessages() {
	for {
		// Receive a message from the transport - this will block until a message arrives
		// or the context is cancelled, eliminating the race condition from the default case
		msg, err := s.transport.Receive(s.ctx)
		if err != nil {
			// Check if context was canceled (server shutdown)
			if s.ctx.Err() != nil {
				s.logger.Debug("Message processing stopped due to context cancellation")
				return
			}
			
			s.logger.Error("Error receiving message: %v", err)
			continue
		}
		
		// Queue the message for worker pool processing
		select {
		case s.messageQueue <- msg:
			// Message queued successfully
		case <-s.ctx.Done():
			// Server is shutting down
			s.logger.Debug("Message processing stopped due to context cancellation")
			return
		default:
			// Queue is full, drop the message with warning
			s.logger.Warn("Message queue is full, dropping message")
		}
	}
}

// handleMessage processes a single MCP message and sends the response.
func (s *StreamableHTTPServer) handleMessage(msg *protocol.Message) {
	// Create a context with timeout for processing
	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()
	
	// Process the message with the MCP server
	response, err := s.mcpServer.HandleMessage(ctx, msg)
	
	// If this is a notification (no ID), we don't need to send a response
	if msg.ID == nil {
		if err != nil {
			s.logger.Error("Error processing notification: %v", err)
		}
		return
	}
	
	// For messages with IDs, create a response
	if err != nil {
		// Create an error response
		errorResponse := &protocol.Message{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Error: &protocol.ErrorObject{
				Code:    -32000, // Server error
				Message: fmt.Sprintf("Error processing request: %v", err),
			},
		}
		
		// Send error response
		if sendErr := s.transport.Send(ctx, errorResponse); sendErr != nil {
			s.logger.Error("Error sending error response: %v", sendErr)
		}
		return
	}
	
	// Send the successful response
	if err := s.transport.Send(ctx, response); err != nil {
		s.logger.Error("Error sending response: %v", err)
	}
}

// ServeHTTPWithContext starts an MCP server with the Streamable HTTP transport and respects the provided context.
func ServeHTTPWithContext(ctx context.Context, mcpServer core.MCPServer, addr string) error {
	config := DefaultStreamableHTTPServerConfig()
	config.ListenAddr = addr
	
	server := NewStreamableHTTPServer(mcpServer, config)
	
	// Start the server
	if err := server.Start(); err != nil {
		return err
	}
	
	// Wait for either context cancellation or server context cancellation
	select {
	case <-ctx.Done():
		// External context cancelled
	case <-server.ctx.Done():
		// Server's internal context cancelled
	}
	
	// Shutdown the server gracefully
	return server.Stop()
}

// ServeHTTP starts an MCP server with the Streamable HTTP transport.
func ServeHTTP(mcpServer core.MCPServer, addr string) error {
	config := DefaultStreamableHTTPServerConfig()
	config.ListenAddr = addr
	
	server := NewStreamableHTTPServer(mcpServer, config)
	
	// Start the server
	if err := server.Start(); err != nil {
		return err
	}
	
	// Wait for context cancellation (allows graceful shutdown)
	<-server.ctx.Done()
	
	// Shutdown the server gracefully
	return server.Stop()
}
