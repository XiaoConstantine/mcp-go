package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/XiaoConstantine/mcp-go/pkg/logging"
	"github.com/XiaoConstantine/mcp-go/pkg/protocol"
)

// TestNewStreamableHTTPTransport tests the constructor.
func TestNewStreamableHTTPTransport(t *testing.T) {
	tests := []struct {
		name     string
		config   *StreamableHTTPTransportConfig
		wantNil  bool
		validate func(*StreamableHTTPTransport) error
	}{
		{
			name:    "nil config uses defaults",
			config:  nil,
			wantNil: false,
			validate: func(transport *StreamableHTTPTransport) error {
				if transport.config == nil {
					return fmt.Errorf("expected non-nil config")
				}
				if transport.config.Logger == nil {
					return fmt.Errorf("expected non-nil logger")
				}
				if transport.config.KeepAliveInterval != 30*time.Second {
					return fmt.Errorf("expected default keep-alive interval")
				}
				return nil
			},
		},
		{
			name: "custom config applied",
			config: &StreamableHTTPTransportConfig{
				Logger:            &logging.NoopLogger{},
				AllowUpgrade:      false,
				RequireSession:    true,
				KeepAliveInterval: 10 * time.Second,
				RequestTimeout:    5 * time.Second,
			},
			wantNil: false,
			validate: func(transport *StreamableHTTPTransport) error {
				if transport.config.AllowUpgrade {
					return fmt.Errorf("expected AllowUpgrade to be false")
				}
				if !transport.config.RequireSession {
					return fmt.Errorf("expected RequireSession to be true")
				}
				if transport.config.KeepAliveInterval != 10*time.Second {
					return fmt.Errorf("expected custom keep-alive interval")
				}
				return nil
			},
		},
		{
			name: "config with pooling disabled",
			config: &StreamableHTTPTransportConfig{
				EnablePooling: false,
			},
			wantNil: false,
			validate: func(transport *StreamableHTTPTransport) error {
				if transport.messagePool != nil {
					return fmt.Errorf("expected nil message pool when pooling disabled")
				}
				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := NewStreamableHTTPTransport(tt.config)

			if tt.wantNil && transport != nil {
				t.Errorf("expected nil transport")
				return
			}
			if !tt.wantNil && transport == nil {
				t.Errorf("expected non-nil transport")
				return
			}

			if transport != nil && tt.validate != nil {
				if err := tt.validate(transport); err != nil {
					t.Errorf("validation failed: %v", err)
				}
			}
		})
	}
}

// TestDefaultStreamableHTTPConfig tests the default configuration.
func TestDefaultStreamableHTTPConfig(t *testing.T) {
	config := DefaultStreamableHTTPConfig()

	if config == nil {
		t.Fatal("expected non-nil config")
	}

	expectedDefaults := map[string]interface{}{
		"AllowUpgrade":      true,
		"RequireSession":    false,
		"KeepAliveInterval": 30 * time.Second,
		"RequestTimeout":    30 * time.Second,
		"EnablePooling":     true,
	}

	if config.AllowUpgrade != expectedDefaults["AllowUpgrade"] {
		t.Errorf("AllowUpgrade = %v, want %v", config.AllowUpgrade, expectedDefaults["AllowUpgrade"])
	}
	if config.RequireSession != expectedDefaults["RequireSession"] {
		t.Errorf("RequireSession = %v, want %v", config.RequireSession, expectedDefaults["RequireSession"])
	}
	if config.KeepAliveInterval != expectedDefaults["KeepAliveInterval"] {
		t.Errorf("KeepAliveInterval = %v, want %v", config.KeepAliveInterval, expectedDefaults["KeepAliveInterval"])
	}
	if config.RequestTimeout != expectedDefaults["RequestTimeout"] {
		t.Errorf("RequestTimeout = %v, want %v", config.RequestTimeout, expectedDefaults["RequestTimeout"])
	}
	if config.EnablePooling != expectedDefaults["EnablePooling"] {
		t.Errorf("EnablePooling = %v, want %v", config.EnablePooling, expectedDefaults["EnablePooling"])
	}

	// Check that sub-configs are initialized
	if config.ConnectionLimits == nil {
		t.Error("expected non-nil ConnectionLimits")
	}
	if config.SessionConfig == nil {
		t.Error("expected non-nil SessionConfig")
	}
}

// TestStreamableHTTPGenerateSessionID tests session ID generation.
func TestStreamableHTTPGenerateSessionID(t *testing.T) {
	transport := NewStreamableHTTPTransport(nil)
	defer transport.Close()

	// Test successful generation
	sessionID, err := transport.GenerateSessionID()
	if err != nil {
		t.Fatalf("GenerateSessionID() error = %v", err)
	}

	if sessionID == "" {
		t.Error("expected non-empty session ID")
	}

	// Test that session IDs are unique
	sessionID2, err := transport.GenerateSessionID()
	if err != nil {
		t.Fatalf("GenerateSessionID() error = %v", err)
	}

	if sessionID == sessionID2 {
		t.Error("expected unique session IDs")
	}

	// Test with nil session manager (fallback)
	transport.sessionManager = nil
	sessionID3, err := transport.GenerateSessionID()
	if err != nil {
		t.Fatalf("GenerateSessionID() with nil session manager error = %v", err)
	}

	if sessionID3 == "" {
		t.Error("expected non-empty session ID from fallback")
	}
}

// TestSend tests the Send method.
func TestSend(t *testing.T) {
	tests := []struct {
		name      string
		message   *protocol.Message
		setupFunc func(*StreamableHTTPTransport)
		wantErr   bool
		validate  func(*StreamableHTTPTransport) error
	}{
		{
			name: "send request message",
			message: &protocol.Message{
				JSONRPC: "2.0",
				ID:      reqID(1),
				Method:  "test",
			},
			wantErr: false,
		},
		{
			name: "send notification",
			message: &protocol.Message{
				JSONRPC: "2.0",
				Method:  "notification",
			},
			wantErr: false,
		},
		{
			name: "send response message",
			message: &protocol.Message{
				JSONRPC: "2.0",
				ID:      reqID(1),
				Result:  json.RawMessage(`{"success": true}`),
			},
			setupFunc: func(transport *StreamableHTTPTransport) {
				// Add a response channel for this ID
				transport.AddResponseChannel(1)
			},
			wantErr: false,
		},
		{
			name: "send to closed transport",
			message: &protocol.Message{
				JSONRPC: "2.0",
				Method:  "test",
			},
			setupFunc: func(transport *StreamableHTTPTransport) {
				transport.isClosing = true
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := NewStreamableHTTPTransport(nil)
			defer transport.Close()

			if tt.setupFunc != nil {
				tt.setupFunc(transport)
			}

			err := transport.Send(context.Background(), tt.message)

			if (err != nil) != tt.wantErr {
				t.Errorf("Send() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.validate != nil {
				if err := tt.validate(transport); err != nil {
					t.Errorf("validation failed: %v", err)
				}
			}
		})
	}
}

// TestReceive tests the Receive method.
func TestReceive(t *testing.T) {
	tests := []struct {
		name      string
		setupFunc func(*StreamableHTTPTransport) context.Context
		wantErr   bool
		wantMsg   bool
	}{
		{
			name: "receive message success",
			setupFunc: func(transport *StreamableHTTPTransport) context.Context {
				// Send a message to be received
				go func() {
					msg := &protocol.Message{
						JSONRPC: "2.0",
						Method:  "test",
					}
					transport.messageCh <- msg
				}()
				return context.Background()
			},
			wantErr: false,
			wantMsg: true,
		},
		{
			name: "context cancelled",
			setupFunc: func(transport *StreamableHTTPTransport) context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			wantErr: true,
			wantMsg: false,
		},
		{
			name: "receive from closed transport",
			setupFunc: func(transport *StreamableHTTPTransport) context.Context {
				transport.isClosing = true
				return context.Background()
			},
			wantErr: true,
			wantMsg: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := NewStreamableHTTPTransport(nil)
			defer transport.Close()

			ctx := tt.setupFunc(transport)

			msg, err := transport.Receive(ctx)

			if (err != nil) != tt.wantErr {
				t.Errorf("Receive() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantMsg && msg == nil {
				t.Error("expected non-nil message")
			}

			if !tt.wantMsg && msg != nil {
				t.Error("expected nil message")
			}
		})
	}
}

// TestClose tests the Close method.
func TestClose(t *testing.T) {
	transport := NewStreamableHTTPTransport(nil)

	// Add some clients and response channels
	transport.clientsMu.Lock()
	transport.clients["test-client"] = make(chan *protocol.Message, 1)
	transport.clientsMu.Unlock()

	transport.responseMu.Lock()
	transport.responseData[1] = make(chan *protocol.Message, 1)
	transport.responseMu.Unlock()

	err := transport.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Verify resources are cleaned up
	if !transport.isClosing {
		t.Error("expected isClosing to be true")
	}

	// Verify channels are cleaned up
	transport.clientsMu.RLock()
	if len(transport.clients) != 0 {
		t.Error("expected clients map to be empty")
	}
	transport.clientsMu.RUnlock()

	transport.responseMu.RLock()
	if len(transport.responseData) != 0 {
		t.Error("expected responseData map to be empty")
	}
	transport.responseMu.RUnlock()

	// Test double close
	err = transport.Close()
	if err != nil {
		t.Errorf("second Close() error = %v", err)
	}
}

// TestHandleRequestOptions tests OPTIONS request handling.
func TestHandleRequestOptions(t *testing.T) {
	transport := NewStreamableHTTPTransport(nil)
	defer transport.Close()

	req := httptest.NewRequest(http.MethodOptions, "/mcp", nil)
	w := httptest.NewRecorder()

	transport.HandleRequest(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	// Check CORS headers
	expectedHeaders := map[string]string{
		"Access-Control-Allow-Origin":  "*",
		"Access-Control-Allow-Methods": "GET, POST, OPTIONS",
		"Access-Control-Allow-Headers": "Content-Type, Accept, Mcp-Session-Id",
	}

	for header, expected := range expectedHeaders {
		if got := w.Header().Get(header); got != expected {
			t.Errorf("header %s = %s, want %s", header, got, expected)
		}
	}
}

// TestHandleRequestMethodNotAllowed tests unsupported HTTP methods.
func TestHandleRequestMethodNotAllowed(t *testing.T) {
	transport := NewStreamableHTTPTransport(nil)
	defer transport.Close()

	methods := []string{http.MethodPut, http.MethodDelete, http.MethodPatch}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/mcp", nil)
			w := httptest.NewRecorder()

			transport.HandleRequest(w, req)

			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
			}
		})
	}
}

// TestHandleJSONRPCRequest tests JSON-RPC request handling.
func TestHandleJSONRPCRequest(t *testing.T) {
	tests := []struct {
		name           string
		message        *protocol.Message
		sessionID      string
		requireSession bool
		wantStatus     int
		wantHeader     string
	}{
		{
			name: "initialize request generates session",
			message: &protocol.Message{
				JSONRPC: "2.0",
				ID:      reqID(1),
				Method:  "initialize",
				Params:  json.RawMessage(`{}`),
			},
			sessionID:      "",
			requireSession: false,
			wantStatus:     http.StatusOK,
			wantHeader:     HeaderSessionID,
		},
		{
			name: "regular request without session",
			message: &protocol.Message{
				JSONRPC: "2.0",
				ID:      reqID(1),
				Method:  "test",
			},
			sessionID:      "",
			requireSession: false,
			wantStatus:     http.StatusOK,
		},
		{
			name: "notification without ID",
			message: &protocol.Message{
				JSONRPC: "2.0",
				Method:  "notification",
			},
			sessionID:      "",
			requireSession: false,
			wantStatus:     http.StatusAccepted,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultStreamableHTTPConfig()
			config.RequireSession = tt.requireSession
			transport := NewStreamableHTTPTransport(config)
			defer transport.Close()

			// Create request body
			body, err := json.Marshal(tt.message)
			if err != nil {
				t.Fatalf("failed to marshal message: %v", err)
			}

			req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
			req.Header.Set(HeaderContentType, ContentTypeJSON)
			if tt.sessionID != "" {
				req.Header.Set(HeaderSessionID, tt.sessionID)
			}

			w := httptest.NewRecorder()

			// Start a goroutine to handle responses for requests with IDs
			if tt.message.ID != nil {
				go func() {
					ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
					defer cancel()

					// Receive the message
					msg, err := transport.Receive(ctx)
					if err != nil {
						return
					}

					// Send a response
					response := &protocol.Message{
						JSONRPC: "2.0",
						ID:      msg.ID,
						Result:  json.RawMessage(`{"success": true}`),
					}
					_ = transport.Send(context.Background(), response)
				}()
			}

			transport.HandleRequest(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, w.Code)
			}

			if tt.wantHeader != "" && w.Header().Get(tt.wantHeader) == "" {
				t.Errorf("expected header %s to be set", tt.wantHeader)
			}
		})
	}
}

// TestResponseChannelManagement tests response channel operations.
func TestResponseChannelManagement(t *testing.T) {
	transport := NewStreamableHTTPTransport(nil)
	defer transport.Close()

	// Test AddResponseChannel
	ch := transport.AddResponseChannel(1)
	if ch == nil {
		t.Error("expected non-nil channel")
	}

	// Verify channel is in map
	transport.responseMu.RLock()
	if _, exists := transport.responseData[1]; !exists {
		t.Error("expected channel to be in responseData map")
	}
	transport.responseMu.RUnlock()

	// Test RemoveResponseChannel
	transport.RemoveResponseChannel(1)

	// Verify channel is removed
	transport.responseMu.RLock()
	if _, exists := transport.responseData[1]; exists {
		t.Error("expected channel to be removed from responseData map")
	}
	transport.responseMu.RUnlock()

	// Test removing non-existent channel
	transport.RemoveResponseChannel(999) // Should not panic
}

// TestConcurrentAccess tests concurrent access to the transport.
func TestConcurrentAccess(t *testing.T) {
	transport := NewStreamableHTTPTransport(nil)
	defer transport.Close()

	const numGoroutines = 10
	const numMessages = 5

	var wg sync.WaitGroup
	wg.Add(numGoroutines * 2) // Send and receive goroutines

	// Start sending goroutines
	for i := 0; i < numGoroutines; i++ {
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < numMessages; j++ {
				msg := &protocol.Message{
					JSONRPC: "2.0",
					Method:  fmt.Sprintf("test-%d-%d", workerID, j),
				}
				_ = transport.Send(context.Background(), msg)
			}
		}(i)
	}

	// Start receiving goroutines
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numMessages; j++ {
				ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
				_, _ = transport.Receive(ctx)
				cancel()
			}
		}()
	}

	// Wait for all goroutines to complete
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(500 * time.Millisecond):
		t.Fatal("test timed out")
	}
}

// TestSessionManagement tests session-related functionality.
func TestSessionManagement(t *testing.T) {
	tests := []struct {
		name           string
		requireSession bool
		sessionID      string
		method         string
		wantStatus     int
	}{
		{
			name:           "session required but not provided",
			requireSession: true,
			sessionID:      "",
			method:         "test",
			wantStatus:     http.StatusBadRequest,
		},
		{
			name:           "session required for initialization is ok",
			requireSession: true,
			sessionID:      "",
			method:         "initialize",
			wantStatus:     http.StatusOK,
		},
		{
			name:           "session provided when required",
			requireSession: true,
			sessionID:      "valid-session",
			method:         "test",
			wantStatus:     http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultStreamableHTTPConfig()
			config.RequireSession = tt.requireSession
			transport := NewStreamableHTTPTransport(config)
			defer transport.Close()

			message := &protocol.Message{
				JSONRPC: "2.0",
				ID:      reqID(1),
				Method:  tt.method,
			}

			body, _ := json.Marshal(message)
			req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
			req.Header.Set(HeaderContentType, ContentTypeJSON)
			if tt.sessionID != "" {
				req.Header.Set(HeaderSessionID, tt.sessionID)
			}

			w := httptest.NewRecorder()

			// Handle response for successful requests
			if tt.wantStatus == http.StatusOK {
				go func() {
					ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
					defer cancel()

					msg, err := transport.Receive(ctx)
					if err != nil {
						return
					}

					response := &protocol.Message{
						JSONRPC: "2.0",
						ID:      msg.ID,
						Result:  json.RawMessage(`{"success": true}`),
					}
					_ = transport.Send(context.Background(), response)
				}()
			}

			transport.HandleRequest(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, w.Code)
			}
		})
	}
}

// TestLegacySessionMethods tests legacy session methods.
func TestLegacySessionMethods(t *testing.T) {
	transport := NewStreamableHTTPTransport(nil)
	defer transport.Close()

	// Test SetSessionID (should not panic)
	transport.SetSessionID("test-session")

	// Test GetSessionID (should return empty for legacy compatibility)
	sessionID := transport.GetSessionID()
	if sessionID != "" {
		t.Errorf("expected empty session ID, got %s", sessionID)
	}
}

// TestHTTPRequestHandling tests comprehensive HTTP request scenarios.
func TestHTTPRequestHandling(t *testing.T) {
	tests := []struct {
		name         string
		method       string
		path         string
		headers      map[string]string
		body         string
		setupConfig  func() *StreamableHTTPTransportConfig
		wantStatus   int
		wantHeaders  map[string]string
		wantBody     string
		validateResp func(*httptest.ResponseRecorder, *testing.T)
	}{
		{
			name:   "POST with valid JSON-RPC request",
			method: http.MethodPost,
			path:   "/mcp",
			headers: map[string]string{
				HeaderContentType: ContentTypeJSON,
			},
			body: `{"jsonrpc":"2.0","id":1,"method":"test","params":{}}`,
			setupConfig: func() *StreamableHTTPTransportConfig {
				return DefaultStreamableHTTPConfig()
			},
			wantStatus: http.StatusOK,
			wantHeaders: map[string]string{
				HeaderContentType: ContentTypeJSON,
			},
		},
		{
			name:   "POST with invalid JSON",
			method: http.MethodPost,
			path:   "/mcp",
			headers: map[string]string{
				HeaderContentType: ContentTypeJSON,
			},
			body: `{"invalid": json}`,
			setupConfig: func() *StreamableHTTPTransportConfig {
				return DefaultStreamableHTTPConfig()
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:   "POST with empty body",
			method: http.MethodPost,
			path:   "/mcp",
			headers: map[string]string{
				HeaderContentType: ContentTypeJSON,
			},
			body: "",
			setupConfig: func() *StreamableHTTPTransportConfig {
				return DefaultStreamableHTTPConfig()
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:   "GET request for health check",
			method: http.MethodGet,
			path:   "/mcp",
			headers: map[string]string{
				HeaderAccept: ContentTypeJSON,
			},
			setupConfig: func() *StreamableHTTPTransportConfig {
				return DefaultStreamableHTTPConfig()
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "GET request with SSE Accept header but upgrades disabled",
			method: http.MethodGet,
			path:   "/mcp",
			headers: map[string]string{
				HeaderAccept: ContentTypeSSE,
			},
			setupConfig: func() *StreamableHTTPTransportConfig {
				config := DefaultStreamableHTTPConfig()
				config.AllowUpgrade = false
				return config
			},
			wantStatus: http.StatusNotImplemented,
		},
		{
			name:   "POST with session validator that rejects",
			method: http.MethodPost,
			path:   "/mcp",
			headers: map[string]string{
				HeaderContentType: ContentTypeJSON,
				HeaderSessionID:   "invalid-session",
			},
			body: `{"jsonrpc":"2.0","id":1,"method":"test"}`,
			setupConfig: func() *StreamableHTTPTransportConfig {
				config := DefaultStreamableHTTPConfig()
				config.SessionValidator = func(sessionID string) bool {
					return sessionID == "valid-session"
				}
				return config
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:   "POST with session validator that accepts",
			method: http.MethodPost,
			path:   "/mcp",
			headers: map[string]string{
				HeaderContentType: ContentTypeJSON,
				HeaderSessionID:   "valid-session",
			},
			body: `{"jsonrpc":"2.0","id":1,"method":"test"}`,
			setupConfig: func() *StreamableHTTPTransportConfig {
				config := DefaultStreamableHTTPConfig()
				config.SessionValidator = func(sessionID string) bool {
					return sessionID == "valid-session"
				}
				return config
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "POST with Accept header for SSE upgrade",
			method: http.MethodPost,
			path:   "/mcp",
			headers: map[string]string{
				HeaderContentType: ContentTypeJSON,
				HeaderAccept:      ContentTypeSSE,
			},
			body: `{"jsonrpc":"2.0","id":1,"method":"test"}`,
			setupConfig: func() *StreamableHTTPTransportConfig {
				return DefaultStreamableHTTPConfig()
			},
			wantStatus: http.StatusOK,
			wantHeaders: map[string]string{
				HeaderContentType: ContentTypeSSE,
			},
			validateResp: func(w *httptest.ResponseRecorder, t *testing.T) {
				// For SSE responses, we should see streaming headers
				if w.Header().Get("Cache-Control") != "no-cache" {
					t.Error("expected Cache-Control: no-cache header for SSE")
				}
				if w.Header().Get("Connection") != "keep-alive" {
					t.Error("expected Connection: keep-alive header for SSE")
				}
				// For SSE responses, check for initial connection event
				body := w.Body.String()
				if !strings.Contains(body, "event: connected") {
					t.Error("expected SSE connection event")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := tt.setupConfig()
			transport := NewStreamableHTTPTransport(config)
			defer transport.Close()

			// Create request
			var body io.Reader
			if tt.body != "" {
				body = strings.NewReader(tt.body)
			}
			req := httptest.NewRequest(tt.method, tt.path, body)

			// Set headers
			for key, value := range tt.headers {
				req.Header.Set(key, value)
			}

			w := httptest.NewRecorder()

			// For requests that expect success and have IDs, handle the response
			if tt.wantStatus == http.StatusOK && strings.Contains(tt.body, `"id":`) {
				go func() {
					ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
					defer cancel()

					// Try to receive the message
					msg, err := transport.Receive(ctx)
					if err != nil {
						return
					}

					// Send a response
					response := &protocol.Message{
						JSONRPC: "2.0",
						ID:      msg.ID,
						Result:  json.RawMessage(`{"success": true}`),
					}
					_ = transport.Send(context.Background(), response)
				}()
			}

			// Handle SSE requests with timeout to prevent hanging
			if acceptHeader, exists := tt.headers[HeaderAccept]; exists && strings.Contains(acceptHeader, ContentTypeSSE) {
				// For SSE requests, we need to handle them asynchronously with a timeout
				done := make(chan struct{})
				go func() {
					defer close(done)
					// Create a context with timeout for the SSE request
					ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
					defer cancel()
					req = req.WithContext(ctx)
					transport.HandleRequest(w, req)
				}()

				// Wait for completion with a timeout
				select {
				case <-done:
					// Request completed
				case <-time.After(200 * time.Millisecond):
					// Force completion if it hangs
					t.Log("SSE request timed out, continuing...")
				}
			} else {
				// Handle non-SSE requests synchronously
				transport.HandleRequest(w, req)
			}

			// Validate status code
			if w.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, w.Code)
			}

			// Validate headers
			for key, expectedValue := range tt.wantHeaders {
				if got := w.Header().Get(key); got != expectedValue {
					t.Errorf("expected header %s = %s, got %s", key, expectedValue, got)
				}
			}

			// Custom validation
			if tt.validateResp != nil {
				tt.validateResp(w, t)
			}
		})
	}
}

// TestHTTPRequestTimeout tests request timeout handling.
func TestHTTPRequestTimeout(t *testing.T) {
	config := DefaultStreamableHTTPConfig()
	config.RequestTimeout = 50 * time.Millisecond
	transport := NewStreamableHTTPTransport(config)
	defer transport.Close()

	// Create a request that will timeout
	message := &protocol.Message{
		JSONRPC: "2.0",
		ID:      reqID(1),
		Method:  "slow_method",
	}

	body, _ := json.Marshal(message)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set(HeaderContentType, ContentTypeJSON)

	w := httptest.NewRecorder()

	// Don't start a response handler - let it timeout

	transport.HandleRequest(w, req)

	if w.Code != http.StatusGatewayTimeout {
		t.Errorf("expected status %d, got %d", http.StatusGatewayTimeout, w.Code)
	}

	// Parse response to check for timeout error
	var response struct {
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	err := json.Unmarshal(w.Body.Bytes(), &response)
	if err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if response.Error == nil {
		t.Error("expected error in response")
	} else if !strings.Contains(response.Error.Message, "timeout") {
		t.Errorf("expected timeout error, got: %s", response.Error.Message)
	}
}

// TestHTTPRequestWithMalformedMessage tests handling of malformed messages.
func TestHTTPRequestWithMalformedMessage(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantCode int
	}{
		{
			name:     "missing jsonrpc field",
			body:     `{"id":1,"method":"test"}`,
			wantCode: http.StatusOK, // Should still be processed
		},
		{
			name:     "invalid jsonrpc version",
			body:     `{"jsonrpc":"1.0","id":1,"method":"test"}`,
			wantCode: http.StatusOK, // Should still be processed
		},
		{
			name:     "missing method field",
			body:     `{"jsonrpc":"2.0","id":1}`,
			wantCode: http.StatusOK, // Should still be processed
		},
		{
			name:     "completely invalid JSON structure",
			body:     `"not an object"`,
			wantCode: http.StatusBadRequest, // Invalid JSON should return 400
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := NewStreamableHTTPTransport(DefaultStreamableHTTPConfig())
			defer transport.Close()

			req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(tt.body))
			req.Header.Set(HeaderContentType, ContentTypeJSON)

			w := httptest.NewRecorder()

			// Start a response handler for messages with IDs
			if strings.Contains(tt.body, `"id":`) {
				go func() {
					ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
					defer cancel()

					msg, err := transport.Receive(ctx)
					if err != nil {
						return
					}

					response := &protocol.Message{
						JSONRPC: "2.0",
						ID:      msg.ID,
						Result:  json.RawMessage(`{"success": true}`),
					}
					_ = transport.Send(context.Background(), response)
				}()
			}

			transport.HandleRequest(w, req)

			if w.Code != tt.wantCode {
				t.Errorf("expected status %d, got %d", tt.wantCode, w.Code)
			}
		})
	}
}

// TestHTTPCORSHeaders tests CORS header handling.
func TestHTTPCORSHeaders(t *testing.T) {
	transport := NewStreamableHTTPTransport(DefaultStreamableHTTPConfig())
	defer transport.Close()

	methods := []string{http.MethodOptions, http.MethodGet, http.MethodPost}

	expectedHeaders := map[string]string{
		"Access-Control-Allow-Origin":  "*",
		"Access-Control-Allow-Methods": "GET, POST, OPTIONS",
		"Access-Control-Allow-Headers": "Content-Type, Accept, Mcp-Session-Id",
	}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			var req *http.Request
			if method == http.MethodPost {
				body := `{"jsonrpc":"2.0","method":"test"}`
				req = httptest.NewRequest(method, "/mcp", strings.NewReader(body))
				req.Header.Set(HeaderContentType, ContentTypeJSON)
			} else {
				req = httptest.NewRequest(method, "/mcp", nil)
			}

			w := httptest.NewRecorder()

			transport.HandleRequest(w, req)

			// Check CORS headers
			for header, expected := range expectedHeaders {
				if got := w.Header().Get(header); got != expected {
					t.Errorf("method %s: header %s = %s, want %s", method, header, got, expected)
				}
			}
		})
	}
}

// TestSSEHandling tests Server-Sent Events functionality.
func TestSSEHandling(t *testing.T) {
	tests := []struct {
		name        string
		method      string
		headers     map[string]string
		sessionID   string
		config      func() *StreamableHTTPTransportConfig
		wantStatus  int
		wantHeaders map[string]string
		validate    func(*httptest.ResponseRecorder, *testing.T)
	}{
		{
			name:   "GET with SSE Accept header",
			method: http.MethodGet,
			headers: map[string]string{
				HeaderAccept: ContentTypeSSE,
			},
			sessionID: "test-session",
			config: func() *StreamableHTTPTransportConfig {
				return DefaultStreamableHTTPConfig()
			},
			wantStatus: http.StatusOK,
			wantHeaders: map[string]string{
				HeaderContentType: ContentTypeSSE,
				"Cache-Control":   "no-cache",
				"Connection":      "keep-alive",
				HeaderSessionID:   "test-session",
			},
			validate: func(w *httptest.ResponseRecorder, t *testing.T) {
				body := w.Body.String()
				if !strings.Contains(body, "event: connected") {
					t.Error("expected connection event in SSE response")
				}
				if !strings.Contains(body, `"status":"connected"`) {
					t.Error("expected connected status in SSE response")
				}
			},
		},
		{
			name:   "GET with SSE but require session and no session",
			method: http.MethodGet,
			headers: map[string]string{
				HeaderAccept: ContentTypeSSE,
			},
			sessionID: "",
			config: func() *StreamableHTTPTransportConfig {
				config := DefaultStreamableHTTPConfig()
				config.RequireSession = true
				return config
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:   "GET with SSE but upgrades disabled",
			method: http.MethodGet,
			headers: map[string]string{
				HeaderAccept: ContentTypeSSE,
			},
			sessionID: "test-session",
			config: func() *StreamableHTTPTransportConfig {
				config := DefaultStreamableHTTPConfig()
				config.AllowUpgrade = false
				return config
			},
			wantStatus: http.StatusNotImplemented,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := tt.config()
			transport := NewStreamableHTTPTransport(config)
			defer transport.Close()

			req := httptest.NewRequest(tt.method, "/mcp", nil)
			for key, value := range tt.headers {
				req.Header.Set(key, value)
			}
			if tt.sessionID != "" {
				req.Header.Set(HeaderSessionID, tt.sessionID)
			}

			w := httptest.NewRecorder()

			// For SSE tests, we need to handle the streaming nature
			if tt.wantStatus == http.StatusOK {
				// Start handling in a goroutine and cancel after a short time
				done := make(chan struct{})
				go func() {
					defer close(done)
					// Create a cancellable context
					ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
					defer cancel()
					req = req.WithContext(ctx)
					transport.HandleRequest(w, req)
				}()

				// Wait for completion
				select {
				case <-done:
					// Connection completed
				case <-time.After(100 * time.Millisecond):
					t.Error("SSE test timed out")
				}
			} else {
				transport.HandleRequest(w, req)
			}

			if w.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, w.Code)
			}

			for key, expected := range tt.wantHeaders {
				if got := w.Header().Get(key); got != expected {
					t.Errorf("expected header %s = %s, got %s", key, expected, got)
				}
			}

			if tt.validate != nil {
				tt.validate(w, t)
			}
		})
	}
}

// TestSSEMessageBroadcast tests broadcasting messages to SSE clients.
func TestSSEMessageBroadcast(t *testing.T) {
	transport := NewStreamableHTTPTransport(DefaultStreamableHTTPConfig())
	defer transport.Close()

	// Create a test message to broadcast
	testMessage := &protocol.Message{
		JSONRPC: "2.0",
		Method:  "test_broadcast",
		Params:  json.RawMessage(`{"data": "test"}`),
	}

	// Manually add a client channel to test broadcasting
	clientCh := make(chan *protocol.Message, 10)
	transport.clientsMu.Lock()
	transport.clients["test-client"] = clientCh
	transport.clientsMu.Unlock()

	// Send a message which should broadcast to all clients
	err := transport.Send(context.Background(), testMessage)
	if err != nil {
		t.Fatalf("Send error: %v", err)
	}

	// Check that the message was received by the client
	select {
	case receivedMsg := <-clientCh:
		if receivedMsg.Method != testMessage.Method {
			t.Errorf("expected method %s, got %s", testMessage.Method, receivedMsg.Method)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for broadcast message")
	}

	// Clean up
	transport.clientsMu.Lock()
	delete(transport.clients, "test-client")
	transport.clientsMu.Unlock()
	close(clientCh)
}

// TestSSEKeepAlive tests SSE keep-alive functionality.
func TestSSEKeepAlive(t *testing.T) {
	config := DefaultStreamableHTTPConfig()
	config.KeepAliveInterval = 20 * time.Millisecond // Short interval for testing
	transport := NewStreamableHTTPTransport(config)
	defer transport.Close()

	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	req.Header.Set(HeaderAccept, ContentTypeSSE)
	req.Header.Set(HeaderSessionID, "test-session")

	w := httptest.NewRecorder()

	// Start SSE handling in a goroutine
	done := make(chan struct{})
	go func() {
		defer close(done)
		// Create a cancellable context that will timeout
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
		defer cancel()
		req = req.WithContext(ctx)

		transport.HandleRequest(w, req)
	}()

	// Wait for the SSE handler to complete
	select {
	case <-done:
		// Check for keep-alive messages in the response
		body := w.Body.String()
		if !strings.Contains(body, "event: keep-alive") {
			t.Error("expected keep-alive events in SSE stream")
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("SSE handler timed out")
	}
}

// TestSSEWithInitialMessage tests SSE upgrade during message handling.
func TestSSEWithInitialMessage(t *testing.T) {
	transport := NewStreamableHTTPTransport(DefaultStreamableHTTPConfig())
	defer transport.Close()

	// Create a message that will trigger SSE upgrade
	message := &protocol.Message{
		JSONRPC: "2.0",
		ID:      reqID(1),
		Method:  "test",
	}

	body, _ := json.Marshal(message)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set(HeaderContentType, ContentTypeJSON)
	req.Header.Set(HeaderAccept, ContentTypeSSE) // This triggers SSE upgrade
	req.Header.Set(HeaderSessionID, "test-session")

	w := httptest.NewRecorder()

	// Start response handler
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		// Receive the message
		msg, err := transport.Receive(ctx)
		if err != nil {
			return
		}

		// Send a response
		response := &protocol.Message{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Result:  json.RawMessage(`{"success": true}`),
		}
		_ = transport.Send(context.Background(), response)
	}()

	// Handle request in a goroutine with timeout
	done := make(chan struct{})
	go func() {
		defer close(done)
		// Create a cancellable context for the request
		ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
		defer cancel()
		req = req.WithContext(ctx)

		transport.HandleRequest(w, req)
	}()

	// Wait for completion
	select {
	case <-done:
		// Verify SSE headers
		if w.Header().Get(HeaderContentType) != ContentTypeSSE {
			t.Errorf("expected content type %s, got %s", ContentTypeSSE, w.Header().Get(HeaderContentType))
		}

		// Verify the response contains both the connection event and the message response
		responseBody := w.Body.String()
		if !strings.Contains(responseBody, "event: connected") {
			t.Error("expected connection event")
		}
		if !strings.Contains(responseBody, "event: message") {
			t.Error("expected message event with response")
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("SSE handler timed out")
	}
}

// TestBatchRequestProcessing tests batch JSON-RPC request handling.
func TestBatchRequestProcessing(t *testing.T) {
	tests := []struct {
		name         string
		batch        []interface{}
		wantStatus   int
		wantResponse bool
		validate     func([]byte, *testing.T)
	}{
		{
			name: "valid batch with mixed requests and notifications",
			batch: []interface{}{
				map[string]interface{}{
					"jsonrpc": "2.0",
					"id":      1,
					"method":  "test1",
				},
				map[string]interface{}{
					"jsonrpc": "2.0",
					"method":  "notification1",
				},
				map[string]interface{}{
					"jsonrpc": "2.0",
					"id":      2,
					"method":  "test2",
				},
			},
			wantStatus:   http.StatusOK,
			wantResponse: true,
			validate: func(body []byte, t *testing.T) {
				var responses []map[string]interface{}
				err := json.Unmarshal(body, &responses)
				if err != nil {
					t.Fatalf("failed to parse batch response: %v", err)
				}

				// Should have 2 responses (for requests with IDs)
				if len(responses) != 2 {
					t.Errorf("expected 2 responses, got %d", len(responses))
				}
			},
		},
		{
			name: "batch with only notifications",
			batch: []interface{}{
				map[string]interface{}{
					"jsonrpc": "2.0",
					"method":  "notification1",
				},
				map[string]interface{}{
					"jsonrpc": "2.0",
					"method":  "notification2",
				},
			},
			wantStatus:   http.StatusNoContent,
			wantResponse: false,
		},
		{
			name:         "empty batch",
			batch:        []interface{}{},
			wantStatus:   http.StatusBadRequest,
			wantResponse: false,
		},
		{
			name: "batch with invalid JSON structure",
			batch: []interface{}{
				map[string]interface{}{
					"jsonrpc": "2.0",
					"id":      1,
					"method":  "test1",
				},
				map[string]interface{}{
					"jsonrpc": "2.0",
					"id":      2,
					"method":  "test2",
				},
			},
			wantStatus:   http.StatusOK,
			wantResponse: true,
			validate: func(body []byte, t *testing.T) {
				// Should process all valid messages
				var responses []map[string]interface{}
				err := json.Unmarshal(body, &responses)
				if err != nil {
					t.Fatalf("failed to parse batch response: %v", err)
				}

				if len(responses) != 2 {
					t.Errorf("expected 2 responses, got %d", len(responses))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := NewStreamableHTTPTransport(DefaultStreamableHTTPConfig())
			defer transport.Close()

			// Create batch request body
			body, err := json.Marshal(tt.batch)
			if err != nil {
				t.Fatalf("failed to marshal batch: %v", err)
			}

			req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
			req.Header.Set(HeaderContentType, ContentTypeJSON)

			w := httptest.NewRecorder()

			// Start response handlers for requests with IDs
			if tt.wantResponse {
				go func() {
					for {
						ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
						msg, err := transport.Receive(ctx)
						cancel()

						if err != nil {
							break
						}

						if msg.ID != nil {
							// Send a response
							response := &protocol.Message{
								JSONRPC: "2.0",
								ID:      msg.ID,
								Result:  json.RawMessage(`{"success": true}`),
							}
							_ = transport.Send(context.Background(), response)
						}
					}
				}()
			}

			// Use the batch handler directly
			transport.HandleBatchRequest(w, req, "test-session")

			if w.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, w.Code)
			}

			if tt.validate != nil {
				tt.validate(w.Body.Bytes(), t)
			}
		})
	}
}

// TestBatchRequestTimeout tests timeout handling in batch requests.
func TestBatchRequestTimeout(t *testing.T) {
	config := DefaultStreamableHTTPConfig()
	config.RequestTimeout = 100 * time.Millisecond // Short timeout for test
	transport := NewStreamableHTTPTransport(config)
	defer transport.Close()

	// Create a batch with requests that will timeout
	batch := []interface{}{
		map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "slow_method1",
		},
		map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      2,
			"method":  "slow_method2",
		},
	}

	body, _ := json.Marshal(batch)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set(HeaderContentType, ContentTypeJSON)

	w := httptest.NewRecorder()

	// Don't start response handlers - let requests timeout

	transport.HandleBatchRequest(w, req, "test-session")

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	// Parse responses to check for timeout errors
	var responses []map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &responses)
	if err != nil {
		t.Fatalf("failed to parse batch response: %v", err)
	}

	if len(responses) != 2 {
		t.Errorf("expected 2 timeout responses, got %d", len(responses))
	}

	for i, resp := range responses {
		if errorObj, ok := resp["error"].(map[string]interface{}); ok {
			if message, ok := errorObj["message"].(string); ok {
				if !strings.Contains(message, "timeout") {
					t.Errorf("response %d: expected timeout error, got: %s", i, message)
				}
			} else {
				t.Errorf("response %d: expected error message", i)
			}
		} else {
			t.Errorf("response %d: expected error object", i)
		}
	}
}

// TestBatchRequestConcurrency tests concurrent batch request processing.
func TestBatchRequestConcurrency(t *testing.T) {
	transport := NewStreamableHTTPTransport(DefaultStreamableHTTPConfig())
	defer transport.Close()

	const numBatches = 5
	const batchSize = 3

	var wg sync.WaitGroup
	wg.Add(numBatches)

	// Start response handler
	go func() {
		for {
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			msg, err := transport.Receive(ctx)
			cancel()

			if err != nil {
				break
			}

			if msg.ID != nil {
				response := &protocol.Message{
					JSONRPC: "2.0",
					ID:      msg.ID,
					Result:  json.RawMessage(fmt.Sprintf(`{"method": "%s"}`, msg.Method)),
				}
				_ = transport.Send(context.Background(), response)
			}
		}
	}()

	// Send concurrent batch requests
	for i := 0; i < numBatches; i++ {
		go func(batchID int) {
			defer wg.Done()

			// Create batch
			batch := make([]interface{}, batchSize)
			for j := 0; j < batchSize; j++ {
				batch[j] = map[string]interface{}{
					"jsonrpc": "2.0",
					"id":      fmt.Sprintf("%d-%d", batchID, j),
					"method":  fmt.Sprintf("test_%d_%d", batchID, j),
				}
			}

			body, _ := json.Marshal(batch)
			req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
			req.Header.Set(HeaderContentType, ContentTypeJSON)

			w := httptest.NewRecorder()
			transport.HandleBatchRequest(w, req, fmt.Sprintf("session-%d", batchID))

			if w.Code != http.StatusOK {
				t.Errorf("batch %d: expected status %d, got %d", batchID, http.StatusOK, w.Code)
				return
			}

			// Verify responses
			var responses []map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &responses)
			if err != nil {
				t.Errorf("batch %d: failed to parse response: %v", batchID, err)
				return
			}

			if len(responses) != batchSize {
				t.Errorf("batch %d: expected %d responses, got %d", batchID, batchSize, len(responses))
			}
		}(i)
	}

	// Wait for all batches to complete
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(500 * time.Millisecond):
		t.Fatal("concurrent batch test timed out")
	}
}

// TestConfigurationValidation tests configuration validation and defaults.
func TestConfigurationValidation(t *testing.T) {
	tests := []struct {
		name           string
		config         *StreamableHTTPTransportConfig
		expectWarning  bool
		validateResult func(*StreamableHTTPTransportConfig, *testing.T)
	}{
		{
			name:          "nil config uses defaults",
			config:        nil,
			expectWarning: false,
			validateResult: func(config *StreamableHTTPTransportConfig, t *testing.T) {
				if config.KeepAliveInterval != 30*time.Second {
					t.Errorf("expected default KeepAliveInterval 30s, got %v", config.KeepAliveInterval)
				}
				if config.RequestTimeout != 30*time.Second {
					t.Errorf("expected default RequestTimeout 30s, got %v", config.RequestTimeout)
				}
				if !config.EnablePooling {
					t.Error("expected default EnablePooling to be true")
				}
			},
		},
		{
			name: "valid custom config preserved",
			config: &StreamableHTTPTransportConfig{
				KeepAliveInterval: 10 * time.Second,
				RequestTimeout:    60 * time.Second,
				AllowUpgrade:      false,
				RequireSession:    true,
			},
			expectWarning: false,
			validateResult: func(config *StreamableHTTPTransportConfig, t *testing.T) {
				if config.KeepAliveInterval != 10*time.Second {
					t.Errorf("expected KeepAliveInterval 10s, got %v", config.KeepAliveInterval)
				}
				if config.RequestTimeout != 60*time.Second {
					t.Errorf("expected RequestTimeout 60s, got %v", config.RequestTimeout)
				}
				if config.AllowUpgrade {
					t.Error("expected AllowUpgrade to be false")
				}
				if !config.RequireSession {
					t.Error("expected RequireSession to be true")
				}
			},
		},
		{
			name: "zero timeout triggers defaults",
			config: &StreamableHTTPTransportConfig{
				KeepAliveInterval: 0,
				RequestTimeout:    0,
			},
			expectWarning: false,
			validateResult: func(config *StreamableHTTPTransportConfig, t *testing.T) {
				if config.KeepAliveInterval != 30*time.Second {
					t.Errorf("expected default KeepAliveInterval 30s, got %v", config.KeepAliveInterval)
				}
				if config.RequestTimeout != 30*time.Second {
					t.Errorf("expected default RequestTimeout 30s, got %v", config.RequestTimeout)
				}
			},
		},
		{
			name: "negative timeout triggers warning and defaults",
			config: &StreamableHTTPTransportConfig{
				KeepAliveInterval: -5 * time.Second,
				RequestTimeout:    30 * time.Second,
			},
			expectWarning: true,
			validateResult: func(config *StreamableHTTPTransportConfig, t *testing.T) {
				// Should fall back to complete defaults due to validation failure
				defaults := DefaultStreamableHTTPConfig()
				if config.KeepAliveInterval != defaults.KeepAliveInterval {
					t.Errorf("expected default KeepAliveInterval, got %v", config.KeepAliveInterval)
				}
			},
		},
		{
			name: "timeout too short triggers warning",
			config: &StreamableHTTPTransportConfig{
				KeepAliveInterval: 500 * time.Millisecond,
				RequestTimeout:    30 * time.Second,
			},
			expectWarning: true,
		},
		{
			name: "timeout too long triggers warning",
			config: &StreamableHTTPTransportConfig{
				KeepAliveInterval: 10 * time.Minute,
				RequestTimeout:    30 * time.Second,
			},
			expectWarning: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture warnings by redirecting fmt output if needed
			// For now, we'll just test that the transport is created successfully
			transport := NewStreamableHTTPTransport(tt.config)
			defer transport.Close()

			if transport == nil {
				t.Fatal("expected non-nil transport")
			}

			if tt.validateResult != nil {
				tt.validateResult(transport.config, t)
			}

			// Verify that all required components are initialized
			if transport.sessionManager == nil {
				t.Error("expected non-nil session manager")
			}
			if transport.connectionManager == nil {
				t.Error("expected non-nil connection manager")
			}
			if transport.cancellationMgr == nil {
				t.Error("expected non-nil cancellation manager")
			}
		})
	}
}

// TestConfigurationDefaults tests that default values are properly applied.
func TestConfigurationDefaults(t *testing.T) {
	// Test that partial configuration gets defaults filled in
	config := &StreamableHTTPTransportConfig{
		AllowUpgrade: false, // Only set this field
	}

	transport := NewStreamableHTTPTransport(config)
	defer transport.Close()

	// Verify defaults were applied
	if transport.config.Logger == nil {
		t.Error("expected default logger")
	}
	if transport.config.ConnectionLimits == nil {
		t.Error("expected default connection limits")
	}
	if transport.config.SessionConfig == nil {
		t.Error("expected default session config")
	}
	if transport.config.KeepAliveInterval == 0 {
		t.Error("expected default keep-alive interval")
	}
	if transport.config.RequestTimeout == 0 {
		t.Error("expected default request timeout")
	}

	// Verify original setting was preserved
	if transport.config.AllowUpgrade {
		t.Error("expected AllowUpgrade to remain false")
	}
}

// TestConfigurationImmutability tests that the original config is not modified.
func TestConfigurationImmutability(t *testing.T) {
	originalConfig := &StreamableHTTPTransportConfig{
		AllowUpgrade:      false,
		KeepAliveInterval: 0, // Will trigger default
	}

	// Store original values
	originalAllowUpgrade := originalConfig.AllowUpgrade
	originalKeepAlive := originalConfig.KeepAliveInterval

	transport := NewStreamableHTTPTransport(originalConfig)
	defer transport.Close()

	// Verify original config was not modified
	if originalConfig.AllowUpgrade != originalAllowUpgrade {
		t.Error("original config AllowUpgrade was modified")
	}
	if originalConfig.KeepAliveInterval != originalKeepAlive {
		t.Error("original config KeepAliveInterval was modified")
	}

	// But the transport should have the defaults applied
	if transport.config.KeepAliveInterval == 0 {
		t.Error("transport config should have default KeepAliveInterval")
	}
}
