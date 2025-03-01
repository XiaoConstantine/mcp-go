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
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.forwardNotifications()
	}()

	// Use a separate reader to allow for cancellation
	readCh := make(chan string)
	errCh := make(chan error)
	// Main message processing loop
	for {
		// Start a goroutine to read the next message
		go func() {
			line, err := s.reader.ReadString('\n')
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
			fmt.Println("Server received shutdown signal, waiting for final messages")
			s.outputSync.Wait()
			s.wg.Wait()
			return nil

		case err := <-errCh:
			if err == io.EOF {
				return nil
			}
			// Log error but don't exit - we might recover
			fmt.Fprintf(os.Stderr, "Error reading from stdin: %v\n", err)
			continue

		case line := <-readCh:
			fmt.Println("RECEIVED MESSAGE:", line)
			// Process the message
			var message protocol.Message
			if err := json.Unmarshal([]byte(line), &message); err != nil {
				s.writeError(message.ID, protocol.ErrCodeParseError, "Parse error", map[string]interface{}{
					"error": err.Error(),
					"line":  line,
				})
				// For tests: if this is a "STOP" message, gracefully exit
				if strings.Contains(line, "\"method\":\"test/stop\"") {
					s.outputSync.Wait()
					return nil
				}

				continue
			}
			// Add this section to check shutdown state AFTER parsing the message
			s.shutdownMu.RLock()
			isShutDown := s.shuttingDown
			s.shutdownMu.RUnlock()
			fmt.Println("==== Checking shut down ====")

			if isShutDown {
				// Send shutdown error for requests with an ID
				if message.ID != nil {
					fmt.Println("Server is shutting down, sending error response")
					s.writeError(message.ID, protocol.ErrCodeShuttingDown, "Server is shutting down", nil)
				}
				continue // Skip normal processing
			}

			// Process the message in a goroutine with timeout
			s.wg.Add(1)
			go func(msg protocol.Message) {
				defer s.wg.Done()
				s.handleMessage(&msg)
			}(message)
		}

	}
}

func (s *Server) handleMessage(msg *protocol.Message) {
	s.shutdownMu.RLock()
	isShutDown := s.shuttingDown
	s.shutdownMu.RUnlock()

	if isShutDown {

		fmt.Println("Found request during shutdown, ID:", msg.ID)
		// If server is shutting down, reject the request
		if msg.ID != nil {

			fmt.Println("Found request during shutdown, ID:", msg.ID)

			s.writeError(msg.ID, protocol.ErrCodeShuttingDown,
				"Server is shutting down", nil)
		}
		return
	}
	// Determine the appropriate timeout
	timeout := s.defaultTimeout

	// Create a context with timeout for this request
	ctx, cancel := context.WithTimeout(s.ctx, timeout)
	defer cancel()
	//	defer s.outputSync.Done()
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
			s.writeError(msg.ID, protocol.ErrCodeServerTimeout, "Request processing timed out", nil)
		} else if errors.Is(ctx.Err(), context.Canceled) {
			s.writeError(msg.ID, protocol.ErrCodeShuttingDown, "Server is shutting down", nil)
		}

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
			s.writeMessage(response)
		}
	}
}

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
			// if !ok {
			// 	// Channel closed
			// 	return
			// }
			// s.writeMessage(&notif)
			//
			if ok {
				s.outputSync.Add(1)
				s.writeMessage(&notif)
			}
		}
	}
}

func (s *Server) writeMessage(msg *protocol.Message) {
	isShutdownMessage := false
	if msg.Error != nil && msg.Error.Code == protocol.ErrCodeShuttingDown {
		isShutdownMessage = true
	}

	// Check shutdown state with proper protection
	s.shutdownMu.RLock()
	isShuttingDown := s.shuttingDown
	s.shutdownMu.RUnlock()

	// Skip regular messages during shutdown, but allow shutdown error messages
	if isShuttingDown && !isShutdownMessage {
		fmt.Println("Skipping write, server is shutting down")
		return
	}

	msgBytes, err := json.Marshal(msg)
	//	fmt.Fprintf(os.Stderr, "Writing message: %s\n", string(msgBytes))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling message: %v\n", err)
		return
	}

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

	// fmt.Fprintf(os.Stderr, "Writer flushed successfully\n")
}

func (s *Server) writeError(id *protocol.RequestID, code int, message string, data interface{}) {
	errorResponse := &protocol.Message{
		JSONRPC: "2.0",
		ID:      id,
		Error: &protocol.ErrorObject{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}

	s.writeMessage(errorResponse)
}

func (s *Server) Stop() error {
	s.shutdownMu.Lock()
	s.shuttingDown = true
	s.shutdownMu.Unlock()

	// Give a breath time for server to reject requests while shutdown
	time.Sleep(500 * time.Millisecond)
	// Signal all operations to stop
	s.cancel()

	// Create a timeout for shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

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

	s.writeError(id, code, message, map[string]interface{}{
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
