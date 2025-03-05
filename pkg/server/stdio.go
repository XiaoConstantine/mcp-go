package server

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/XiaoConstantine/mcp-go/pkg/protocol"
	"github.com/XiaoConstantine/mcp-go/pkg/server/core"
)

// Server represents an MCP server that communicates over STDIO.
type Server struct {
	mcpServer      core.MCPServer
	reader         *bufio.Reader
	writer         *bufio.Writer
	mu             sync.Mutex
	wg             sync.WaitGroup
	ctx            context.Context
	cancel         context.CancelFunc
	defaultTimeout time.Duration

	shutdownMu   sync.RWMutex
	shuttingDown bool
	outputSync   *sync.WaitGroup

	messageQueue chan *protocol.Message
}

// ServerConfig holds configuration options for the STDIO server.
type ServerConfig struct {
	DefaultTimeout time.Duration // Default timeout for request processing
}

// DefaultServerConfig provides reasonable default configuration values.
func DefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		DefaultTimeout: 30 * time.Second,
	}
}

// NewServer creates a new STDIO-based MCP server.
func NewServer(mcpServer core.MCPServer, config *ServerConfig) *Server {
	if config == nil {
		config = DefaultServerConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &Server{
		mcpServer:      mcpServer,
		reader:         bufio.NewReader(os.Stdin),
		writer:         bufio.NewWriter(os.Stdout),
		ctx:            ctx,
		cancel:         cancel,
		defaultTimeout: config.DefaultTimeout,
		outputSync:     &sync.WaitGroup{},
		messageQueue:   make(chan *protocol.Message, 100),
	}
}

// isShuttingDown checks whether the server is in shutdown state
func (s *Server) isShuttingDown() bool {
	s.shutdownMu.RLock()
	defer s.shutdownMu.RUnlock()
	return s.shuttingDown
}

// handleOutgoingMessages processes messages from the queue and writes them to stdout
func (s *Server) handleOutgoingMessages() {
	defer s.wg.Done()
	for msg := range s.messageQueue {
		s.writeMessageToStdout(msg)
	}
}

// Start begins the server's read-process-write loop.
func (s *Server) Start() error {
	s.shutdownMu.RLock()
	if s.shuttingDown {
		s.shutdownMu.RUnlock()
		return errors.New("server is shutting down")
	}
	s.shutdownMu.RUnlock()

	// Start notification forwarding goroutine
	s.wg.Add(2)
	go func() {
		defer s.wg.Done()
		s.forwardNotifications()
	}()
	go s.handleOutgoingMessages()
	stdinReader := bufio.NewReader(os.Stdin)
	// Use a separate reader to allow for cancellation
	readCh := make(chan string)
	errCh := make(chan error)
	// Main message processing loop
	for {
		// Start a goroutine to read the next message
		go func() {
			line, err := stdinReader.ReadString('\n')
			if err != nil {
				errCh <- err
				return
			}
			readCh <- line
		}()
		// Wait for either a message, an error, or shutdown signal
		select {
		case <-s.ctx.Done():
			// Server is shutting down
			fmt.Fprintln(os.Stderr, "Server received shutdown signal, waiting for final messages")
			close(s.messageQueue)
			s.outputSync.Wait()
			s.wg.Wait()
			return nil

		case err := <-errCh:
			if err == io.EOF {
				fmt.Fprintf(os.Stderr, "EOF on stdin, waiting for reconnection\n")
			} else {
				// Log error but don't exit - we might recover
				fmt.Fprintf(os.Stderr, "Error reading from stdin: %v\n", err)

			}
			continue

		case line := <-readCh:
			// Process the message
			var message protocol.Message
			if err := json.Unmarshal([]byte(line), &message); err != nil {
				errorMsg := &protocol.Message{
					JSONRPC: "2.0",
					Error: &protocol.ErrorObject{
						Code:    protocol.ErrCodeParseError,
						Message: "Parse error",
						Data: map[string]interface{}{
							"error": err.Error(),
							"line":  line,
						},
					},
				}
				s.outputSync.Add(1)
				s.messageQueue <- errorMsg
				// For tests: if this is a "STOP" message, gracefully exit
				if strings.Contains(line, "\"method\":\"test/stop\"") {
					s.outputSync.Wait()
					return nil
				}

				continue
			}
			// Process the message in a goroutine with timeout
			s.wg.Add(1)
			go func(msg protocol.Message) {
				defer s.wg.Done()
				s.processMessage(&msg)
			}(message)
		}

	}
}

// func (s *Server) handleMessage(msg *protocol.Message) {
// 	s.shutdownMu.RLock()
// 	isShutDown := s.shuttingDown
// 	s.shutdownMu.RUnlock()
//
// 	if isShutDown {
// 		// If server is shutting down, reject the request
// 		if msg.ID != nil {
// 			s.writeError(msg.ID, protocol.ErrCodeShuttingDown,
// 				"Server is shutting down", nil)
// 		}
// 		return
// 	}
// 	// Determine the appropriate timeout
// 	timeout := s.defaultTimeout
//
// 	// Create a context with timeout for this request
// 	ctx, cancel := context.WithTimeout(s.ctx, timeout)
// 	defer cancel()
// 	// Process the message with the timeout context
// 	responseCh := make(chan *protocol.Message, 1)
// 	errCh := make(chan error, 1)
// 	go func() {
// 		response, err := s.mcpServer.HandleMessage(ctx, msg)
// 		if err != nil {
// 			errCh <- err
// 			return
// 		}
// 		responseCh <- response
// 	}()
//
// 	select {
// 	case <-ctx.Done():
// 		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
// 			s.writeError(msg.ID, protocol.ErrCodeServerTimeout, "Request processing timed out", nil)
// 		} else if errors.Is(ctx.Err(), context.Canceled) {
// 			s.writeError(msg.ID, protocol.ErrCodeShuttingDown, "Server is shutting down", nil)
// 		}
//
// 		select {
// 		case <-responseCh:
// 		case <-errCh:
// 		default:
// 		}
//
// 	case err := <-errCh:
// 		s.handleSpecificError(msg.ID, err)
//
// 	case response := <-responseCh:
// 		// Send response if not a notification
// 		if msg.ID != nil {
// 			s.writeMessage(response)
// 		}
// 	}
// }

func (s *Server) forwardNotifications() {
	notifChan := s.mcpServer.Notifications()

	for {
		select {
		case <-s.ctx.Done():
			select {
			case <-notifChan:
			default:
			}
			return

		case notif, ok := <-notifChan:
			if ok {
				s.outputSync.Add(1)
				// s.writeMessage(&notif)
				s.messageQueue <- &notif
			}
		}
	}
}

func (s *Server) writeMessageToStdout(msg *protocol.Message) {
	defer s.outputSync.Done()
	// Check if we should allow this message during shutdown
	if s.isShuttingDown() {
		// Only allow shutdown error messages during shutdown
		isShutdownError := msg.Error != nil && msg.Error.Code == protocol.ErrCodeShuttingDown
		if !isShutdownError {
			return
		}
	}

	msgBytes, err := json.Marshal(msg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling message: %v\n", err)
		return
	}
	// Use a mutex to synchronize writes to stdout
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := s.writer.Write(msgBytes); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing message: %v\n", err)
		return
	}
	if err := s.writer.WriteByte('\n'); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing newline: %v\n", err)
		return
	}
	if err := s.writer.Flush(); err != nil {
		fmt.Fprintf(os.Stderr, "Error flushing writer: %v\n", err)
		return
	}
}

// processMessage handles an incoming message with proper timeout and error handling
func (s *Server) processMessage(msg *protocol.Message) {
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
		if msg.ID != nil {
			s.outputSync.Add(1)
			s.messageQueue <- response
		}
	}
}

func (s *Server) enqueueError(id *protocol.RequestID, code int, message string, data interface{}) {
	errorResponse := &protocol.Message{
		JSONRPC: "2.0",
		ID:      id,
		Error: &protocol.ErrorObject{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}

	s.outputSync.Add(1)
	s.messageQueue <- errorResponse
}

func (s *Server) Stop() error {
	s.shutdownMu.Lock()
	if s.shuttingDown {
		s.shutdownMu.Unlock()
		return nil // Already shutting down
	}
	s.shutdownMu.Unlock()

	// Create a timeout for shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	// First, initiate MCP server component shutdown
	if err := s.mcpServer.Shutdown(shutdownCtx); err != nil {
		fmt.Fprintf(os.Stderr, "Error during MCP server shutdown: %v\n", err)
		// Continue with shutdown anyway
	}

	// Give a brief moment for any pending messages to be rejected
	time.Sleep(200 * time.Millisecond)
	// Signal all operations to stop
	s.cancel()
	// Wait for in-flight operations with timeout
	doneCh := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(doneCh)
	}()

	select {
	case <-doneCh:
		// All operations completed successfully
		return nil
	case <-shutdownCtx.Done():
		// Shutdown timed out
		return errors.New("server shutdown timed out waiting for operations to complete")
	}
}

func (s *Server) handleSpecificError(id *protocol.RequestID, err error) {
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
		// Attempt to classify the error based on its content
		errStr := err.Error()
		switch {
		case contains(errStr, "method not found", "unknown method", "unsupported method"):
			fmt.Fprintf(os.Stderr, "Method not found: %s\n", errStr)
			code = protocol.ErrCodeMethodNotFound
			message = "Method not found"
		case contains(errStr, "invalid params", "missing parameter", "invalid argument"):
			code = protocol.ErrCodeInvalidParams
			message = "Invalid params"
		default:
			code = protocol.ErrCodeInternalError
			message = "Internal server error"
		}
	}

	s.enqueueError(id, code, message, map[string]interface{}{
		"error": err.Error(),
	})
}

func (s *Server) SetTimeout(timeout time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.defaultTimeout = timeout
}

// contains checks if the string contains any of the substrings.
func contains(s string, substrings ...string) bool {
	for _, sub := range substrings {
		if strings.Contains(strings.ToLower(s), strings.ToLower(sub)) {
			return true
		}
	}
	return false
}
