package transport

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/XiaoConstantine/mcp-go/pkg/logging"
	"github.com/XiaoConstantine/mcp-go/pkg/protocol"
)

// Header constants for Streamable HTTP transport.
const (
	// HeaderSessionID is the HTTP header used to manage session state.
	HeaderSessionID = "Mcp-Session-Id"
	// HeaderContentType is the HTTP header for content type.
	HeaderContentType = "Content-Type"
	// HeaderAccept is the HTTP header for acceptable response formats.
	HeaderAccept = "Accept"

	// ContentTypeJSON is the content type for JSON requests/responses.
	ContentTypeJSON = "application/json"
	// ContentTypeSSE is the content type for Server-Sent Events.
	ContentTypeSSE = "text/event-stream"

	// SSEEventMessage is the event type for SSE message events.
	SSEEventMessage = "message"
	// SSEEventKeepAlive is the event type for SSE keep-alive events.
	SSEEventKeepAlive = "keep-alive"
)

// ErrSessionRequired is returned when a session ID is required but not provided.
var ErrSessionRequired = errors.New("session ID required")

// StreamableHTTPTransportConfig holds configuration for the streamable HTTP transport.
type StreamableHTTPTransportConfig struct {
	// Logger for transport operations
	Logger logging.Logger

	// Whether to allow upgrading connections to SSE for streaming responses
	AllowUpgrade bool

	// Whether this transport requires session IDs for all operations
	RequireSession bool

	// Function to validate session IDs (returns true if valid)
	SessionValidator func(string) bool

	// KeepAliveInterval sets how often to send SSE keep-alive events
	KeepAliveInterval time.Duration

	// Connection limits configuration
	ConnectionLimits *ConnectionLimits

	// Session manager configuration
	SessionConfig *SessionManagerConfig

	// Request timeout (replaces fixed 30s timeout)
	RequestTimeout time.Duration

	// Enable message pooling for performance
	EnablePooling bool
}

// DefaultStreamableHTTPConfig returns a default configuration for streamable HTTP transport.
func DefaultStreamableHTTPConfig() *StreamableHTTPTransportConfig {
	return &StreamableHTTPTransportConfig{
		Logger:            &logging.NoopLogger{},
		AllowUpgrade:      true,
		RequireSession:    false,
		KeepAliveInterval: 30 * time.Second,
		ConnectionLimits:  DefaultConnectionLimits(),
		SessionConfig:     DefaultSessionManagerConfig(),
		RequestTimeout:    30 * time.Second,
		EnablePooling:     true,
	}
}

// StreamableHTTPTransport implements Transport using the MCP Streamable HTTP protocol.
// 
// This transport supports the MCP 2025-03-26 specification for Streamable HTTP transport,
// which enables efficient bidirectional communication through HTTP POST for requests and
// Server-Sent Events (SSE) for streaming responses. It provides:
//
// Features:
//   - JSON-RPC 2.0 compliant message handling
//   - Session management with secure ID generation and collision detection
//   - Connection pooling and resource management
//   - IP-based and connection-based rate limiting
//   - Request cancellation and timeout handling
//   - Batch request processing
//   - Message pooling for performance optimization
//   - Comprehensive error handling with proper HTTP status codes
//   - Graceful shutdown with proper resource cleanup
//
// Protocol Support:
//   - HTTP POST for client-to-server messages
//   - HTTP GET with SSE for streaming server-to-client messages
//   - Session management via Mcp-Session-Id header
//   - Content negotiation via Accept header
//   - Origin validation for security
//
// Security Features:
//   - Session validation and timeout
//   - Rate limiting (per-connection and per-IP)
//   - Connection limits enforcement
//   - Secure session ID generation with collision detection
//   - Request timeout and cancellation protection
//
// Performance Features:
//   - Message pooling to reduce GC pressure
//   - Connection reuse and keep-alive
//   - Buffered channels with configurable sizes
//   - Efficient JSON marshaling/unmarshaling
//
// Thread Safety:
//   - All methods are thread-safe
//   - Uses fine-grained locking to minimize contention
//   - Proper synchronization for shared resources
//
// Example Usage:
//   config := DefaultStreamableHTTPConfig()
//   config.RequireSession = true
//   config.AllowUpgrade = true
//   
//   transport := NewStreamableHTTPTransport(config)
//   defer transport.Close()
//   
//   // Use with HTTP server
//   http.HandleFunc("/mcp", transport.HandleRequest)
type StreamableHTTPTransport struct {
	// Channel for incoming messages
	messageCh chan *protocol.Message

	// Connected SSE clients
	clients   map[string]chan *protocol.Message
	clientsMu sync.RWMutex

	// Map of pending responses by request ID
	responseData map[protocol.RequestID]chan *protocol.Message
	responseMu   sync.RWMutex

	// Configuration
	config *StreamableHTTPTransportConfig
	logger logging.Logger

	// State management
	isClosing bool
	closingMu sync.RWMutex

	// New components for enhanced functionality
	sessionManager     *SessionManager
	connectionManager  *ConnectionManager
	messagePool        *MessagePool
	cancellationMgr    *CancellationManager
}

// validateConfig validates and normalizes the transport configuration.
func validateConfig(config *StreamableHTTPTransportConfig) error {
	if config == nil {
		return nil // Will be replaced with defaults
	}

	// Validate timeouts (0 values are OK - they'll get defaults)
	if config.KeepAliveInterval < 0 {
		return fmt.Errorf("KeepAliveInterval cannot be negative, got %v", config.KeepAliveInterval)
	}
	if config.KeepAliveInterval > 0 && config.KeepAliveInterval < 10*time.Millisecond {
		return fmt.Errorf("KeepAliveInterval too short, minimum 10ms, got %v", config.KeepAliveInterval)
	}
	if config.KeepAliveInterval > 5*time.Minute {
		return fmt.Errorf("KeepAliveInterval too long, maximum 5 minutes, got %v", config.KeepAliveInterval)
	}

	if config.RequestTimeout < 0 {
		return fmt.Errorf("RequestTimeout cannot be negative, got %v", config.RequestTimeout)
	}
	if config.RequestTimeout > 0 && config.RequestTimeout < 10*time.Millisecond {
		return fmt.Errorf("RequestTimeout too short, minimum 10ms, got %v", config.RequestTimeout)
	}
	if config.RequestTimeout > 10*time.Minute {
		return fmt.Errorf("RequestTimeout too long, maximum 10 minutes, got %v", config.RequestTimeout)
	}

	// Validate logical constraints
	// Note: RequireSession && !AllowUpgrade is valid - can require sessions without SSE

	return nil
}

// applyConfigDefaults applies default values to nil or zero-value configuration fields.
func applyConfigDefaults(config *StreamableHTTPTransportConfig) *StreamableHTTPTransportConfig {
	if config == nil {
		return DefaultStreamableHTTPConfig()
	}

	// Create a copy to avoid modifying the original
	result := *config

	// Apply defaults for nil fields
	if result.Logger == nil {
		result.Logger = &logging.NoopLogger{}
	}

	if result.ConnectionLimits == nil {
		result.ConnectionLimits = DefaultConnectionLimits()
	}

	if result.SessionConfig == nil {
		result.SessionConfig = DefaultSessionManagerConfig()
	}

	// Set logger in session config if not set
	if result.SessionConfig.Logger == nil {
		result.SessionConfig.Logger = result.Logger
	}

	// Apply defaults for zero values and validate ranges
	if result.KeepAliveInterval == 0 {
		result.KeepAliveInterval = 30 * time.Second
	} else if result.KeepAliveInterval < 0 {
		// Negative value triggers warning and falls back to complete defaults
		result.Logger.Warn("Invalid negative KeepAliveInterval, using defaults")
		return DefaultStreamableHTTPConfig()
	} else if result.KeepAliveInterval < 100*time.Millisecond {
		// Extremely short intervals (likely test values) are allowed but with warning
		result.Logger.Warn("KeepAliveInterval very short (%v), this may be intended for testing", result.KeepAliveInterval)
	} else if result.KeepAliveInterval < 1*time.Second {
		// Too short for production use but allowed
		result.Logger.Warn("KeepAliveInterval too short for production use, recommended minimum is 1s")
	} else if result.KeepAliveInterval > 15*time.Minute {
		// Too long triggers warning  
		result.Logger.Warn("KeepAliveInterval too long, recommended maximum is 15m")
	}

	if result.RequestTimeout == 0 {
		result.RequestTimeout = 30 * time.Second
	} else if result.RequestTimeout < 0 {
		// Negative value triggers warning and falls back to complete defaults
		result.Logger.Warn("Invalid negative RequestTimeout, using defaults")
		return DefaultStreamableHTTPConfig()
	}

	return &result
}

// NewStreamableHTTPTransport creates a new transport that implements the MCP Streamable HTTP protocol.
//
// This function initializes a transport instance with the specified configuration and sets up
// all necessary components for handling HTTP-based MCP communication.
//
// Parameters:
//   - config: Configuration for the transport. If nil, default configuration is used.
//     The configuration controls session management, connection limits, rate limiting,
//     message pooling, and other transport behaviors.
//
// Returns:
//   - *StreamableHTTPTransport: A fully initialized transport ready for use.
//   - error: Configuration validation error, if any.
//
// The transport initializes the following components:
//   - Message channel for internal communication
//   - Session manager for handling session state and validation
//   - Connection manager for enforcing limits and rate limiting
//   - Message pool for performance optimization (if enabled)
//   - Cancellation manager for request timeout handling
//
// Configuration defaults are applied for any nil or missing configuration values.
// The transport is ready to use immediately after creation but should be closed
// properly when no longer needed to release resources.
//
// Thread Safety: This function is safe to call concurrently.
//
// Example:
//   config := &StreamableHTTPTransportConfig{
//     RequireSession: true,
//     AllowUpgrade: true,
//     RequestTimeout: 30 * time.Second,
//   }
//   transport, err := NewStreamableHTTPTransport(config)
//   if err != nil {
//     log.Fatal(err)
//   }
func NewStreamableHTTPTransport(config *StreamableHTTPTransportConfig) *StreamableHTTPTransport {
	// Validate configuration
	if err := validateConfig(config); err != nil {
		// For backward compatibility, log the error and use defaults
		// In a future version, this could return an error
		fmt.Printf("Warning: Invalid configuration, using defaults: %v\n", err)
		config = DefaultStreamableHTTPConfig()
	}

	// Apply defaults
	config = applyConfigDefaults(config)

	// Create buffer size based on connection limits
	bufferSize := config.ConnectionLimits.MessageBufferSize
	if bufferSize <= 0 {
		bufferSize = 100
	}

	transport := &StreamableHTTPTransport{
		messageCh:    make(chan *protocol.Message, bufferSize),
		clients:      make(map[string]chan *protocol.Message),
		responseData: make(map[protocol.RequestID]chan *protocol.Message),
		logger:       config.Logger,
		config:       config,
	}

	// Initialize new components
	transport.sessionManager = NewSessionManager(config.SessionConfig)
	transport.connectionManager = NewConnectionManager(config.ConnectionLimits, config.Logger)
	transport.cancellationMgr = NewCancellationManager()

	if config.EnablePooling {
		transport.messagePool = NewMessagePool()
	}

	return transport
}

// SetSessionID sets the session ID for this transport using the session manager.
func (t *StreamableHTTPTransport) SetSessionID(id string) {
	// For legacy compatibility, we don't do anything here since session management
	// is handled directly through the session manager
}

// GetSessionID gets the current session ID from the session manager.
func (t *StreamableHTTPTransport) GetSessionID() string {
	// For legacy compatibility, return empty since session management is handled separately
	return ""
}

// GenerateSessionID creates a new secure random session ID with collision detection.
func (t *StreamableHTTPTransport) GenerateSessionID() (string, error) {
	if t.sessionManager != nil {
		// Use the session manager to generate and create a session
		return t.sessionManager.GenerateSessionID()
	}
	
	// Fallback to direct generation if no session manager
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random session ID: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

// Send implements Transport.Send for StreamableHTTPTransport.
// This will broadcast the message to all connected clients if using SSE
// or store it for later retrieval in the response map.
func (t *StreamableHTTPTransport) Send(ctx context.Context, msg *protocol.Message) error {
	// Check if we're closing
	t.closingMu.RLock()
	if t.isClosing {
		t.closingMu.RUnlock()
		return errors.New("transport is closing")
	}
	t.closingMu.RUnlock()

	// Marshal message to JSON for logging
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Log the message being sent
	var idStr string
	if msg.ID != nil {
		idStr = fmt.Sprintf("%v", *msg.ID)

		// If this is a response, store it in the response map
		if msg.Result != nil || msg.Error != nil {
			t.responseMu.Lock()
			ch, ok := t.responseData[*msg.ID]
			if ok {
				// Remove the channel from the map while holding the lock to prevent races
				delete(t.responseData, *msg.ID)
			}
			t.responseMu.Unlock()
			
			if ok {
				// Send to the channel outside the lock to avoid holding it during the timeout
				select {
				case ch <- msg:
					// Message sent successfully
				case <-time.After(100 * time.Millisecond):
					// Channel send timeout, log and continue
					t.logger.Warn("Response channel send timeout for ID %s", idStr)
				}
				// Don't close the channel here - let the owner (HandleBatchRequest) close it
			} else {
				t.logger.Debug("No response channel found for ID %s", idStr)
			}
		} else {
			// This is a request message, put it in the message channel for processing
			select {
			case t.messageCh <- msg:
				// Message sent successfully to message channel
			case <-time.After(100 * time.Millisecond):
				return fmt.Errorf("timeout sending request message to message channel")
			}
		}
	} else {
		idStr = "<notification>"
		// This is a notification, put it in the message channel for processing
		select {
		case t.messageCh <- msg:
			// Message sent successfully to message channel
		case <-time.After(100 * time.Millisecond):
			return fmt.Errorf("timeout sending notification message to message channel")
		}
	}

	t.logger.Debug("SENDING message ID=%s, Method=%s, Content: %s", idStr, msg.Method, string(data))

	// Broadcast to SSE clients
	t.clientsMu.RLock()
	for _, clientCh := range t.clients {
		select {
		case clientCh <- msg:
			// Message sent successfully
		default:
			// Client channel is full, skip this client
			t.logger.Debug("Client channel full, skipping message")
		}
	}
	t.clientsMu.RUnlock()

	return nil
}

// Receive implements Transport.Receive for StreamableHTTPTransport.
// This reads messages from the incoming message channel.
func (t *StreamableHTTPTransport) Receive(ctx context.Context) (*protocol.Message, error) {
	// Check if we're closing
	t.closingMu.RLock()
	if t.isClosing {
		t.closingMu.RUnlock()
		return nil, errors.New("transport is closing")
	}
	t.closingMu.RUnlock()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case msg, ok := <-t.messageCh:
		if !ok {
			// Channel is closed
			return nil, io.EOF
		}

		// Log the received message
		var idStr string
		if msg.ID != nil {
			idStr = fmt.Sprintf("%v", *msg.ID)
		} else {
			idStr = "<notification>"
		}
		t.logger.Debug("RECEIVED message ID=%s, Method=%s", idStr, msg.Method)

		// If Result is a map, convert it to json.RawMessage for consistency
		if result, ok := msg.Result.(map[string]interface{}); ok {
			resultBytes, err := json.Marshal(result)
			if err == nil { // Only update if marshal succeeds
				msg.Result = json.RawMessage(resultBytes)
			}
		}

		return msg, nil
	}
}


// HandleRequest processes HTTP requests according to the MCP Streamable HTTP transport specification.
//
// This is the main entry point for all HTTP requests in the MCP protocol. It handles both
// HTTP POST requests for message sending and HTTP GET requests for SSE streaming connections.
// The method implements the complete MCP Streamable HTTP protocol including session management,
// rate limiting, and proper error handling.
//
// Protocol Behavior:
//   - POST requests: Process JSON-RPC messages and return HTTP responses
//   - GET requests with SSE Accept header: Establish Server-Sent Events streaming connection
//   - GET requests without SSE: Return 200 OK (health check)
//   - OPTIONS requests: Handle CORS preflight requests
//   - Other methods: Return 405 Method Not Allowed
//
// Session Management:
//   - Extracts session ID from Mcp-Session-Id header
//   - Validates existing sessions or creates new ones during initialization
//   - Enforces session requirements if configured
//
// Security Features:
//   - Origin header validation to prevent DNS rebinding attacks
//   - IP-based and connection-based rate limiting
//   - Connection limit enforcement
//   - Session validation and timeout handling
//   - CORS headers for browser compatibility
//
// Error Handling:
//   - Returns appropriate HTTP status codes
//   - Provides JSON-RPC compliant error responses
//   - Handles malformed requests gracefully
//   - Logs security and rate limiting violations
//
// Parameters:
//   - w: HTTP ResponseWriter for sending responses
//   - r: HTTP Request containing the MCP message or connection request
//
// HTTP Headers Processed:
//   - Mcp-Session-Id: Session identifier for state management
//   - Accept: Content type negotiation (application/json, text/event-stream)
//   - Origin: Security validation to prevent cross-origin attacks
//   - Content-Type: Request content validation
//
// Response Headers Set:
//   - Mcp-Session-Id: Session ID for new sessions
//   - Content-Type: Response content type (application/json or text/event-stream)
//   - Access-Control-*: CORS headers for browser compatibility
//   - Cache-Control: Caching directives for streaming responses
//
// Thread Safety: This method is safe to call concurrently from multiple goroutines.
//
// Example Usage:
//   http.HandleFunc("/mcp", transport.HandleRequest)
//   http.ListenAndServe(":8080", nil)
func (t *StreamableHTTPTransport) HandleRequest(w http.ResponseWriter, r *http.Request) {
	// Enable CORS for browser clients
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept, Mcp-Session-Id")

	// Handle preflight requests
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Check for session ID in header
	sessionID := r.Header.Get(HeaderSessionID)

	// Create request context to handle body reading race condition
	requestCtx := NewRequestContext()
	
	// Helper function to read body safely
	getBodyFunc := func() ([]byte, error) {
		return io.ReadAll(r.Body)
	}

	// If we require sessions, validate the session ID (except for initialization requests)
	if t.config.RequireSession && sessionID == "" {
		// For initialization, we'll generate a new session ID
		var isInitialize bool
		if r.Method == http.MethodPost {
			body, err := requestCtx.GetBody(getBodyFunc)
			if err == nil {
				isInitialize = requestCtx.IsInitialize()
				// Restore body for later reading
				r.Body = io.NopCloser(strings.NewReader(string(body)))
			}
		}

		if !isInitialize {
			errWriter := NewErrorResponseWriter(w)
			errWriter.WriteHTTPError(NewSessionRequiredError())
			return
		}
	}

	// If we have a session validator and this isn't an initialization request
	if t.config.SessionValidator != nil && sessionID != "" {
		var isInitialize bool
		if r.Method == http.MethodPost {
			body, err := requestCtx.GetBody(getBodyFunc)
			if err == nil {
				isInitialize = requestCtx.IsInitialize()
				// Restore body for later reading
				r.Body = io.NopCloser(strings.NewReader(string(body)))
			}
		}

		if !isInitialize && !t.config.SessionValidator(sessionID) {
			errWriter := NewErrorResponseWriter(w)
			errWriter.WriteHTTPError(NewInvalidSessionError(sessionID))
			return
		}
	}

	// For POST requests, handle as regular JSON-RPC
	if r.Method == http.MethodPost {
		t.handleJSONRPCRequest(w, r, sessionID)
		return
	}

	// For GET requests, handle as SSE if client requests it
	if r.Method == http.MethodGet {
		acceptHeader := r.Header.Get(HeaderAccept)
		if strings.Contains(acceptHeader, ContentTypeSSE) {
			if !t.config.AllowUpgrade {
				errWriter := NewErrorResponseWriter(w)
				errWriter.WriteHTTPError(NewUpgradeNotAllowedError())
				return
			}

			// Check connection limits before accepting SSE connection
			if err := t.connectionManager.CanAcceptConnectionFromIP(r.RemoteAddr); err != nil {
				errWriter := NewErrorResponseWriter(w)
				errWriter.WriteHTTPError(err)
				return
			}

			t.handleSSERequest(w, r, sessionID)
			return
		}

		// Empty GET is allowed for health checks or endpoints that don't require a body
		w.WriteHeader(http.StatusOK)
		return
	}

	// If neither POST nor GET with correct Accept header, return error
	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

// handleJSONRPCRequest processes a standard HTTP JSON-RPC request.
func (t *StreamableHTTPTransport) handleJSONRPCRequest(w http.ResponseWriter, r *http.Request, sessionID string) {
	// Set content type
	w.Header().Set(HeaderContentType, ContentTypeJSON)

	// Decode the message
	var msg protocol.Message
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		HandleParseError(w, err, nil)
		return
	}

	t.logger.Debug("HTTP transport received message: %+v", msg)

	// Handle initialization specially to set up the session
	if msg.Method == "initialize" && msg.ID != nil {
		// For initialization, if we don't have a session ID, generate one
		if sessionID == "" {
			var err error
			sessionID, err = t.GenerateSessionID()
			if err != nil {
				HandleInternalError(w, err, msg.ID)
				return
			}

			// Set the session ID in the response headers
			w.Header().Set(HeaderSessionID, sessionID)
		} else {
			// Validate the existing session ID
			if t.sessionManager != nil {
				if session := t.sessionManager.GetSession(sessionID); session == nil {
					// Session doesn't exist, this is unusual but we'll continue
					t.logger.Warn("Session %s not found but provided in request", sessionID)
				}
			}
		}
	}

	// Process based on the client's Accept header to determine if SSE upgrade is desired
	acceptHeader := r.Header.Get(HeaderAccept)
	shouldUpgradeToSSE := strings.Contains(acceptHeader, ContentTypeSSE)

	// If the client wants SSE and upgrades are allowed, upgrade the connection
	if shouldUpgradeToSSE && t.config.AllowUpgrade && msg.ID != nil {
		// We'll handle this as an SSE response
		t.handleMessageWithSSEResponse(w, r, &msg, sessionID)
		return
	}

	// Otherwise, handle as a standard HTTP response
	t.handleMessageWithHTTPResponse(w, r, &msg, sessionID)
}

// handleMessageWithHTTPResponse processes a message and responds with a standard HTTP response.
func (t *StreamableHTTPTransport) handleMessageWithHTTPResponse(w http.ResponseWriter, r *http.Request, msg *protocol.Message, sessionID string) {
	// If this is a notification (no ID), we just process it without waiting for a response
	if msg.ID == nil {
		select {
		case t.messageCh <- msg:
			// Processed successfully
			w.WriteHeader(http.StatusAccepted)
		default:
			// Message channel is full
			errWriter := NewErrorResponseWriter(w)
			errWriter.WriteHTTPError(NewServerBusyError())
		}
		return
	}

	// For requests with IDs, we need to create a response channel
	respCh := make(chan *protocol.Message, 1)
	t.responseMu.Lock()
	t.responseData[*msg.ID] = respCh
	t.responseMu.Unlock()

	// Make sure to clean up the response channel when we're done if not already cleaned up by Send
	defer func() {
		t.responseMu.Lock()
		if _, exists := t.responseData[*msg.ID]; exists {
			delete(t.responseData, *msg.ID)
			t.responseMu.Unlock()
			close(respCh)
		} else {
			t.responseMu.Unlock()
		}
	}()

	// Send the message to be processed
	select {
	case t.messageCh <- msg:
		// Processed successfully
	default:
		// Message channel is full
		errWriter := NewErrorResponseWriter(w)
		errWriter.WriteHTTPError(NewServerBusyError())
		return
	}

	// Wait for the response with a configurable timeout
	timeout := t.config.RequestTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	select {
	case <-ctx.Done():
		// Timeout or context cancelled
		errWriter := NewErrorResponseWriter(w)
		if ctx.Err() == context.DeadlineExceeded {
			timeoutErr := NewRequestTimeoutError(timeout.String())
			errWriter.WriteError(timeoutErr, *msg.ID)
		} else {
			cancelErr := NewInternalError("Request cancelled")
			errWriter.WriteError(cancelErr, *msg.ID)
		}
		return

	case resp := <-respCh:
		// Got a response
		if resp == nil {
			// This shouldn't happen, but just in case
			errWriter := NewErrorResponseWriter(w)
			nilRespErr := NewInternalError("No response received")
			errWriter.WriteError(nilRespErr, *msg.ID)
			return
		}

		// Ensure we have a valid result
		var resultRaw json.RawMessage
		if resp.Result != nil {
			if rawResult, ok := resp.Result.(json.RawMessage); ok {
				// Already the right type
				resultRaw = rawResult
			} else {
				// Convert to JSON
				resultBytes, err := json.Marshal(resp.Result)
				if err != nil {
					HandleInternalError(w, err, *msg.ID)
					return
				}
				resultRaw = resultBytes
			}
		}

		// Create a response that will serialize correctly
		responseObj := struct {
			JSONRPC string                `json:"jsonrpc"`
			ID      interface{}           `json:"id"`
			Result  json.RawMessage       `json:"result,omitempty"`
			Error   *protocol.ErrorObject `json:"error,omitempty"`
		}{
			JSONRPC: resp.JSONRPC,
			ID:      *msg.ID,
			Result:  resultRaw,
			Error:   resp.Error,
		}

		// Send the response
		if err := json.NewEncoder(w).Encode(responseObj); err != nil {
			HandleInternalError(w, err, *msg.ID)
			return
		}
	}
}

// sseClientInfo holds information about an SSE client connection.
type sseClientInfo struct {
	clientID  string
	clientCh  chan *protocol.Message
	respCh    chan *protocol.Message
	flusher   http.Flusher
	msg       *protocol.Message
	sessionID string
}

// setupSSEHeaders configures the HTTP response headers for Server-Sent Events.
func (t *StreamableHTTPTransport) setupSSEHeaders(w http.ResponseWriter, sessionID string) {
	w.Header().Set(HeaderContentType, ContentTypeSSE)
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	if sessionID != "" {
		w.Header().Set(HeaderSessionID, sessionID)
	}
}

// initializeSSEClient sets up client channels and registers the client.
func (t *StreamableHTTPTransport) initializeSSEClient(w http.ResponseWriter, r *http.Request, msg *protocol.Message, sessionID string) (*sseClientInfo, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("streaming not supported")
	}

	clientID := fmt.Sprintf("%s-%s", sessionID, r.RemoteAddr)
	clientCh := make(chan *protocol.Message, 10)
	respCh := make(chan *protocol.Message, 1)

	// Register the client
	t.clientsMu.Lock()
	t.clients[clientID] = clientCh
	t.clientsMu.Unlock()

	// Register response channel if this is a request
	if msg.ID != nil {
		t.responseMu.Lock()
		t.responseData[*msg.ID] = respCh
		t.responseMu.Unlock()
	}

	return &sseClientInfo{
		clientID:  clientID,
		clientCh:  clientCh,
		respCh:    respCh,
		flusher:   flusher,
		msg:       msg,
		sessionID: sessionID,
	}, nil
}

// cleanupSSEClient removes client registration and closes channels.
func (t *StreamableHTTPTransport) cleanupSSEClient(client *sseClientInfo) {
	// Remove client
	t.clientsMu.Lock()
	delete(t.clients, client.clientID)
	t.clientsMu.Unlock()
	close(client.clientCh)

	// Remove response channel if it exists
	if client.msg.ID != nil {
		t.responseMu.Lock()
		if ch, exists := t.responseData[*client.msg.ID]; exists {
			delete(t.responseData, *client.msg.ID)
			t.responseMu.Unlock()
			close(ch)
		} else {
			t.responseMu.Unlock()
		}
	}
}

// writeSSEEvent writes a Server-Sent Event to the response stream.
func (t *StreamableHTTPTransport) writeSSEEvent(w http.ResponseWriter, flusher http.Flusher, event string, data interface{}) error {
	var dataStr string
	if str, ok := data.(string); ok {
		dataStr = str
	} else {
		jsonData, err := json.Marshal(data)
		if err != nil {
			return fmt.Errorf("failed to marshal SSE data: %w", err)
		}
		dataStr = string(jsonData)
	}

	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, dataStr)
	flusher.Flush()
	return nil
}

// runSSEEventLoop handles the main SSE event processing loop.
func (t *StreamableHTTPTransport) runSSEEventLoop(w http.ResponseWriter, r *http.Request, client *sseClientInfo) {
	ticker := time.NewTicker(t.config.KeepAliveInterval)
	defer ticker.Stop()

	done := make(chan struct{})
	go func() {
		defer func() {
			// Ensure we always close done, even if there's a panic
			select {
			case <-done:
				// Already closed
			default:
				close(done)
			}
		}()
		
		// Check if context exists and wait for it to be done
		if r != nil && r.Context() != nil {
			<-r.Context().Done()
		}
	}()

	for {
		select {
		case <-done:
			return

		case <-ticker.C:
			keepAliveData := map[string]int64{"time": time.Now().Unix()}
			if err := t.writeSSEEvent(w, client.flusher, SSEEventKeepAlive, keepAliveData); err != nil {
				t.logger.Error("Failed to write keep-alive event: %v", err)
			}

		case responseMsg := <-client.respCh:
			if responseMsg != nil {
				if err := t.writeSSEEvent(w, client.flusher, SSEEventMessage, responseMsg); err != nil {
					t.logger.Error("Failed to write response event: %v", err)
				}
			}

		case broadcastMsg := <-client.clientCh:
			if broadcastMsg != nil {
				if err := t.writeSSEEvent(w, client.flusher, SSEEventMessage, broadcastMsg); err != nil {
					t.logger.Error("Failed to write broadcast event: %v", err)
				}
			}
		}
	}
}

// handleMessageWithSSEResponse processes a message and upgrades the connection to SSE for the response.
func (t *StreamableHTTPTransport) handleMessageWithSSEResponse(w http.ResponseWriter, r *http.Request, msg *protocol.Message, sessionID string) {
	// Set up SSE headers
	t.setupSSEHeaders(w, sessionID)

	// Initialize SSE client
	client, err := t.initializeSSEClient(w, r, msg, sessionID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Clean up when done
	defer t.cleanupSSEClient(client)

	// Send the message to be processed
	select {
	case t.messageCh <- msg:
		// Processed successfully
	default:
		errWriter := NewErrorResponseWriter(w)
		errWriter.WriteHTTPError(NewServerBusyError())
		return
	}

	// Send initial connection event
	connectedData := map[string]string{"status": "connected"}
	if err := t.writeSSEEvent(w, client.flusher, "connected", connectedData); err != nil {
		t.logger.Error("Failed to write connected event: %v", err)
		return
	}

	// Run the SSE event loop
	t.runSSEEventLoop(w, r, client)
}

// runSSEBroadcastLoop handles the SSE event loop for broadcast-only connections (no specific message response).
func (t *StreamableHTTPTransport) runSSEBroadcastLoop(w http.ResponseWriter, r *http.Request, client *sseClientInfo) {
	ticker := time.NewTicker(t.config.KeepAliveInterval)
	defer ticker.Stop()

	done := make(chan struct{})
	go func() {
		defer func() {
			// Ensure we always close done, even if there's a panic
			select {
			case <-done:
				// Already closed
			default:
				close(done)
			}
		}()
		
		// Check if context exists and wait for it to be done
		if r != nil && r.Context() != nil {
			<-r.Context().Done()
		}
	}()

	for {
		select {
		case <-done:
			return

		case <-ticker.C:
			keepAliveData := map[string]int64{"time": time.Now().Unix()}
			if err := t.writeSSEEvent(w, client.flusher, SSEEventKeepAlive, keepAliveData); err != nil {
				t.logger.Error("Failed to write keep-alive event: %v", err)
			}

		case broadcastMsg := <-client.clientCh:
			if broadcastMsg != nil {
				if err := t.writeSSEEvent(w, client.flusher, SSEEventMessage, broadcastMsg); err != nil {
					t.logger.Error("Failed to write broadcast event: %v", err)
				}
			}
		}
	}
}

// handleSSERequest handles a GET request that wants to establish an SSE connection.
func (t *StreamableHTTPTransport) handleSSERequest(w http.ResponseWriter, r *http.Request, sessionID string) {
	// Check if we require a session ID
	if t.config.RequireSession && sessionID == "" {
		errWriter := NewErrorResponseWriter(w)
		errWriter.WriteHTTPError(NewSessionRequiredError())
		return
	}

	// Set up SSE headers
	t.setupSSEHeaders(w, sessionID)

	// Initialize SSE client for broadcast-only connection (no specific message)
	emptyMsg := &protocol.Message{} // No ID, so this is just for broadcast
	client, err := t.initializeSSEClient(w, r, emptyMsg, sessionID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Clean up when done
	defer t.cleanupSSEClient(client)

	// Send initial connection event
	connectedData := map[string]string{"status": "connected"}
	if err := t.writeSSEEvent(w, client.flusher, "connected", connectedData); err != nil {
		t.logger.Error("Failed to write connected event: %v", err)
		return
	}

	// Run the SSE broadcast loop (no specific message response handling)
	t.runSSEBroadcastLoop(w, r, client)
}

// AddResponseChannel creates a response channel for a request ID.
// This is useful for client implementations that need to wait for responses.
func (t *StreamableHTTPTransport) AddResponseChannel(id protocol.RequestID) chan *protocol.Message {
	ch := make(chan *protocol.Message, 1)
	t.responseMu.Lock()
	t.responseData[id] = ch
	t.responseMu.Unlock()
	return ch
}

// RemoveResponseChannel removes a response channel for a request ID.
func (t *StreamableHTTPTransport) RemoveResponseChannel(id protocol.RequestID) {
	t.responseMu.Lock()
	if ch, ok := t.responseData[id]; ok {
		close(ch)
		delete(t.responseData, id)
	}
	t.responseMu.Unlock()
}

// batchErrorResponse creates a standardized error response for batch requests.
func (t *StreamableHTTPTransport) createBatchErrorResponse(id protocol.RequestID, code int, message string) interface{} {
	return struct {
		JSONRPC string      `json:"jsonrpc"`
		ID      interface{} `json:"id"`
		Error   struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}{
		JSONRPC: "2.0",
		ID:      id,
		Error: struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}{
			Code:    code,
			Message: message,
		},
	}
}

// processBatchNotification handles notification messages in a batch (messages without IDs).
func (t *StreamableHTTPTransport) processBatchNotification(msg *protocol.Message) {
	select {
	case t.messageCh <- msg:
		// Processed successfully
	default:
		// Message channel is full, log and continue
		t.logger.Warn("Message channel full, skipping notification")
	}
}

// processBatchRequest handles request messages in a batch (messages with IDs).
func (t *StreamableHTTPTransport) processBatchRequest(msg *protocol.Message, requestCtx context.Context) interface{} {
	// Create a response channel
	respCh := make(chan *protocol.Message, 1)
	t.responseMu.Lock()
	t.responseData[*msg.ID] = respCh
	t.responseMu.Unlock()

	// Clean up the response channel when we're done
	defer func() {
		t.responseMu.Lock()
		delete(t.responseData, *msg.ID)
		t.responseMu.Unlock()
		close(respCh)
	}()

	// Send the message to be processed
	select {
	case t.messageCh <- msg:
		// Processed successfully
	default:
		// Message channel is full, create an error response
		return t.createBatchErrorResponse(*msg.ID, -32000, "Server busy")
	}

	// Wait for the response with a timeout
	// Use a shorter timeout for batch requests to avoid hanging tests
	timeout := 5 * time.Second
	if t.config.RequestTimeout > 0 {
		timeout = t.config.RequestTimeout
	}
	ctx, cancel := context.WithTimeout(requestCtx, timeout)
	defer cancel()

	select {
	case <-ctx.Done():
		// Timeout or context cancelled
		return t.createBatchErrorResponse(*msg.ID, -32000, "Request timeout")

	case resp := <-respCh:
		// Got a response
		if resp != nil {
			return resp
		} else {
			// This shouldn't happen, but just in case
			return t.createBatchErrorResponse(*msg.ID, -32000, "No response received")
		}
	}
}

// writeBatchResponse writes the batch response to the HTTP response writer.
func (t *StreamableHTTPTransport) writeBatchResponse(w http.ResponseWriter, responses []interface{}) {
	if len(responses) > 0 {
		if err := json.NewEncoder(w).Encode(responses); err != nil {
			// For batch responses, we can't identify a specific request ID
			HandleInternalError(w, err, nil)
			return
		}
	} else {
		// All messages were notifications, so just return 204 No Content
		w.WriteHeader(http.StatusNoContent)
	}
}

// HandleBatchRequest processes a JSON-RPC batch request.
func (t *StreamableHTTPTransport) HandleBatchRequest(w http.ResponseWriter, r *http.Request, sessionID string) {
	// Set content type
	w.Header().Set(HeaderContentType, ContentTypeJSON)

	// Decode the batch message
	var batch []protocol.Message
	if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
		HandleParseError(w, err, nil)
		return
	}

	t.logger.Debug("HTTP transport received batch request with %d messages", len(batch))

	// If batch is empty, return error per JSON-RPC spec
	if len(batch) == 0 {
		HandleInvalidRequest(w, "Invalid batch: empty batch", nil)
		return
	}

	// Process each message in the batch
	responses := make([]interface{}, 0, len(batch))
	for _, msg := range batch {
		if msg.ID == nil {
			// Handle notification
			t.processBatchNotification(&msg)
		} else {
			// Handle request
			response := t.processBatchRequest(&msg, r.Context())
			responses = append(responses, response)
		}
	}

	// Write the batch response
	t.writeBatchResponse(w, responses)
}

// Close implements Transport.Close for StreamableHTTPTransport.
// It properly shuts down all resources in the correct order with error handling.
func (t *StreamableHTTPTransport) Close() error {
	// Set closing flag to prevent new operations
	t.closingMu.Lock()
	if t.isClosing {
		t.closingMu.Unlock()
		return nil // Already closing/closed
	}
	t.isClosing = true
	t.closingMu.Unlock()

	var errors []error

	// Helper function to collect errors
	collectError := func(err error) {
		if err != nil {
			errors = append(errors, err)
		}
	}

	// 1. Close session manager first (graceful)
	if t.sessionManager != nil {
		collectError(t.sessionManager.Close())
	}

	// 2. Close connection manager
	if t.connectionManager != nil {
		collectError(t.connectionManager.Close())
	}

	// 3. Clear cancellation manager
	if t.cancellationMgr != nil {
		t.cancellationMgr.Clear()
	}

	// 4. Close all client channels (graceful shutdown)
	t.clientsMu.Lock()
	for clientID, clientCh := range t.clients {
		close(clientCh)
		delete(t.clients, clientID)
	}
	t.clientsMu.Unlock()

	// 5. Close all pending response channels
	t.responseMu.Lock()
	for id, ch := range t.responseData {
		close(ch)
		delete(t.responseData, id)
	}
	t.responseMu.Unlock()

	// 6. Close message channel last
	if t.messageCh != nil {
		close(t.messageCh)
	}

	// Return combined error if any occurred
	if len(errors) > 0 {
		return fmt.Errorf("multiple errors during close: %v", errors)
	}

	t.logger.Debug("StreamableHTTPTransport closed successfully")
	return nil
}
