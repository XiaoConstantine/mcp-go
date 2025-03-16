package server

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/XiaoConstantine/mcp-go/pkg/logging"
	"github.com/XiaoConstantine/mcp-go/pkg/protocol"
	"github.com/XiaoConstantine/mcp-go/pkg/server/core"
	"github.com/XiaoConstantine/mcp-go/pkg/transport"
)

// SSEServer represents an MCP server that communicates over Server-Sent Events.
type SSEServer struct {
	mcpServer      core.MCPServer
	transport      *transport.SSETransport
	httpServer     *http.Server
	mu             sync.Mutex
	wg             sync.WaitGroup
	ctx            context.Context
	cancel         context.CancelFunc
	defaultTimeout time.Duration

	shutdownMu   sync.RWMutex
	shuttingDown bool
	messageQueue chan *protocol.Message

	logger logging.Logger
}

// SSEServerConfig holds configuration options for the SSE server.
type SSEServerConfig struct {
	DefaultTimeout time.Duration  // Default timeout for request processing
	ListenAddr     string         // Address to listen on, e.g., ":8080"
	Logger         logging.Logger // Logger for server messages
}

// DefaultSSEServerConfig provides reasonable default configuration values.
func DefaultSSEServerConfig() *SSEServerConfig {
	return &SSEServerConfig{
		DefaultTimeout: 30 * time.Second,
		ListenAddr:     ":8080",
		Logger:         logging.NewStdLogger(logging.InfoLevel),
	}
}

// NewSSEServer creates a new SSE-based MCP server.
func NewSSEServer(mcpServer core.MCPServer, config *SSEServerConfig) *SSEServer {
	if config == nil {
		config = DefaultSSEServerConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	sseTransport := transport.NewSSETransport(config.Logger)
	return &SSEServer{
		mcpServer:      mcpServer,
		transport:      sseTransport,
		ctx:            ctx,
		cancel:         cancel,
		defaultTimeout: config.DefaultTimeout,
		messageQueue:   make(chan *protocol.Message, 100),
		logger:         config.Logger,
		httpServer: &http.Server{
			Addr:         config.ListenAddr,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 60 * time.Second,
			IdleTimeout:  120 * time.Second,
		},
	}
}

// isShuttingDown checks whether the server is in shutdown state.
func (s *SSEServer) isShuttingDown() bool {
	s.shutdownMu.RLock()
	defer s.shutdownMu.RUnlock()
	return s.shuttingDown
}

// handleOutgoingMessages processes messages from the queue and sends them via the transport.
func (s *SSEServer) handleOutgoingMessages() {
	defer s.wg.Done()
	for {
		select {
		case <-s.ctx.Done():
			return
		case msg := <-s.messageQueue:
			ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
			if err := s.transport.Send(ctx, msg); err != nil {
				s.logger.Error("Failed to send message",
					"error", err,
					"message_id", msg.ID)
			}
			cancel()
		}
	}
}

// Start begins the server's HTTP server and message processing.
func (s *SSEServer) Start() error {
	s.shutdownMu.RLock()
	if s.shuttingDown {
		s.shutdownMu.RUnlock()
		return errors.New("server is shutting down")
	}
	s.shutdownMu.RUnlock()

	// Set up the HTTP routes
	mux := http.NewServeMux()
	mux.HandleFunc("/events", s.transport.HandleSSE)
	mux.HandleFunc("/message", s.transport.HandleClientMessage)

	// Set the handler
	s.httpServer.Handler = mux

	// Start the HTTP server in a goroutine
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.logger.Info("Starting HTTP server on %s", s.httpServer.Addr)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("HTTP server error: %v", err)
		}
	}()

	// Start notification forwarding goroutine
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.forwardNotifications()
	}()

	// Start message queue processing
	s.wg.Add(1)
	go s.handleOutgoingMessages()

	// Start message receiving loop
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.receiveAndProcessMessages()
	}()

	return nil
}

func (s *SSEServer) forwardNotifications() {
	notifChan := s.mcpServer.Notifications()

	for {
		select {
		case <-s.ctx.Done():
			return

		case notif, ok := <-notifChan:
			if !ok {
				return
			}
			select {
			case s.messageQueue <- &notif:
				// Successfully queued
			default:
				// Queue is full, log and discard
				s.logger.Warn("Notification queue full, dropping notification")
			}
		}
	}
}

// receiveAndProcessMessages receives messages from the transport and processes them.
func (s *SSEServer) receiveAndProcessMessages() {
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		ctx, cancel := context.WithTimeout(s.ctx, 100*time.Millisecond)
		msg, err := s.transport.Receive(ctx)
		cancel()

		if err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				// This is just a timeout for polling, continue
				continue
			}

			// Log other errors but don't exit
			s.logger.Debug("Error receiving message: %v", err)
			continue
		}

		// Process the message in a goroutine with timeout
		s.wg.Add(1)
		go func(msg *protocol.Message) {
			defer s.wg.Done()
			s.processMessage(msg)
		}(msg)
	}
}

// processMessage handles an incoming message with proper timeout and error handling.
func (s *SSEServer) processMessage(msg *protocol.Message) {
	// Check if we're shutting down first
	if s.isShuttingDown() {
		// If server is in shutdown state, reject all requests with IDs
		if msg.ID != nil {
			s.enqueueError(msg.ID, protocol.ErrCodeShuttingDown,
				"Server is shutting down", nil)
		}
		return
	}
	// Determine the appropriate timeout
	timeout := s.defaultTimeout

	// Create a context with timeout for this request
	ctx, cancel := context.WithTimeout(s.ctx, timeout)
	defer cancel()

	// Process the message with the timeout context
	responseCh := make(chan *protocol.Message, 1)
	errCh := make(chan error, 1)

	go func() {
		response, err := s.mcpServer.HandleMessage(ctx, msg)
		if err != nil {
			errCh <- err
			return
		}
		responseCh <- response
	}()
	select {
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			s.enqueueError(msg.ID, protocol.ErrCodeServerTimeout,
				"Request processing timed out", nil)
		} else if errors.Is(ctx.Err(), context.Canceled) {
			s.enqueueError(msg.ID, protocol.ErrCodeShuttingDown,
				"Server is shutting down", nil)
		}

		// Drain channels to prevent goroutine leaks
		select {
		case <-responseCh:
		case <-errCh:
		default:
		}

	case err := <-errCh:
		s.handleSpecificError(msg.ID, err)

	case response := <-responseCh:
		// Send response if not a notification
		if msg.ID != nil && response != nil {
			select {
			case s.messageQueue <- response:
				// Successfully queued
			default:
				// Queue is full, log and discard
				s.logger.Warn("Response queue full, dropping response")
			}
		}
	}
}

func (s *SSEServer) enqueueError(id *protocol.RequestID, code int, message string, data interface{}) {
	errorResponse := &protocol.Message{
		JSONRPC: "2.0",
		ID:      id,
		Error: &protocol.ErrorObject{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}

	if s.isShuttingDown() && code != protocol.ErrCodeShuttingDown {
		// Don't send non-shutdown errors during shutdown
		return
	}

	select {
	case s.messageQueue <- errorResponse:
		// Message sent successfully
	default:
		// Channel is full or closed, unable to send
		s.logger.Warn("Error queue full, dropping error response")
	}
}

// Stop gracefully shuts down the server.
func (s *SSEServer) Stop() error {
	s.shutdownMu.Lock()
	if s.shuttingDown {
		s.shutdownMu.Unlock()
		return nil // Already shutting down
	}

	s.shuttingDown = true
	s.shutdownMu.Unlock()

	// Create a timeout for shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	// First, signal all operations to stop
	s.cancel()

	// Shutdown the HTTP server
	if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
		s.logger.Error("HTTP server shutdown error: %v", err)
	}

	// Now initiate MCP server component shutdown
	if err := s.mcpServer.Shutdown(shutdownCtx); err != nil {
		s.logger.Error("Error during MCP server shutdown: %v", err)
	}

	// Close the transport
	if err := s.transport.Close(); err != nil {
		s.logger.Error("Error closing transport: %v", err)
	}

	// Wait for in-flight operations with timeout
	doneCh := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(doneCh)
	}()

	select {
	case <-doneCh:
		// All operations completed successfully
		s.logger.Info("Server shutdown complete")
		return nil
	case <-shutdownCtx.Done():
		// Shutdown timed out
		s.logger.Warn("Server shutdown timed out")
		return errors.New("server shutdown timed out")
	}
}

func (s *SSEServer) handleSpecificError(id *protocol.RequestID, err error) {
	// Check for specific error types and map them to appropriate error codes
	var code int
	var message string

	// Common error types to handle
	if errors.Is(err, context.DeadlineExceeded) {
		code = protocol.ErrCodeServerTimeout
		message = "Request processing timed out"
	} else if errors.Is(err, context.Canceled) {
		code = protocol.ErrCodeShuttingDown
		message = "Server is shutting down"
	} else {
		// Default to internal error
		code = protocol.ErrCodeInternalError
		message = "Internal server error"
	}

	s.enqueueError(id, code, message, map[string]interface{}{
		"error": err.Error(),
	})
}

// SetTimeout sets the default timeout for request processing.
func (s *SSEServer) SetTimeout(timeout time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.defaultTimeout = timeout
}
