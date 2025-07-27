package protocol

import "encoding/json"

// RequestID represents a uniquely identifying ID for a request in JSON-RPC
// It can be either a string or an integer.
type RequestID interface{}

// Message represents any JSON-RPC message.
type Message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *RequestID      `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *ErrorObject    `json:"error,omitempty"`
}

// ErrorObject represents an error response in JSON-RPC.
type ErrorObject struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Meta represents metadata that can be attached to various MCP messages.
type Meta struct {
	ProgressToken interface{} `json:"progressToken,omitempty"`
}

// Request represents a request that expects a response.
type Request struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      RequestID   `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// Response represents a successful response to a request.
type Response struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      RequestID   `json:"id"`
	Result  interface{} `json:"result"`
}

// Notification represents a notification which does not expect a response.
type Notification struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// Error codes as per JSON-RPC 2.0 specification.
const (
	// Standard JSON-RPC 2.0 error codes.
	ErrCodeParseError     = -32700
	ErrCodeInvalidRequest = -32600
	ErrCodeMethodNotFound = -32601
	ErrCodeInvalidParams  = -32602
	ErrCodeInternalError  = -32603

	// Server error codes (-32000 to -32099 are reserved for implementation-defined server-errors).
	ErrCodeServerError       = -32000 // Generic server error
	ErrCodeSessionRequired   = -32001 // Session ID required  
	ErrCodeInvalidSession    = -32002 // Invalid session ID
	ErrCodeConnectionLimit   = -32003 // Connection limit exceeded
	ErrCodeRequestTimeout    = -32004 // Request timeout
	ErrCodeServerBusy        = -32005 // Server busy
	ErrCodeUpgradeNotAllowed = -32006 // SSE upgrade not allowed
	ErrCodeInvalidUpgrade    = -32007 // Invalid upgrade request
	ErrCodeShuttingDown      = -32008 // Server shutting down
)
