// Package protocol provides the core JSON-RPC protocol types for MCP
package protocol

// RequestID represents a uniquely identifying ID for a request in JSON-RPC
// It can be either a string or an integer
type RequestID interface{}

// Meta represents metadata that can be attached to various objects
type Meta struct {
	ProgressToken interface{} `json:"progressToken,omitempty"`
}

// Message represents the base JSON-RPC message
type Message struct {
	JSONRPC string     `json:"jsonrpc"`
	ID      *RequestID `json:"id,omitempty"`
}

// Request represents a JSON-RPC request that expects a response
type Request struct {
	Message
	Method string      `json:"method"`
	Params interface{} `json:"params,omitempty"`
}

// Response represents a successful JSON-RPC response
type Response struct {
	Message
	Result interface{} `json:"result"`
}

// Error represents a JSON-RPC error response
type Error struct {
	Message
	Error *ErrorObject `json:"error"`
}

// ErrorObject contains the error details for a JSON-RPC error response
type ErrorObject struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Notification represents a JSON-RPC notification that does not expect a response
type Notification struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// Common JSON-RPC error codes
const (
	// Standard JSON-RPC 2.0 errors
	ErrParseError     = -32700
	ErrInvalidRequest = -32600
	ErrMethodNotFound = -32601
	ErrInvalidParams  = -32602
	ErrInternalError  = -32603
	
	// Server error codes are from -32000 to -32099
	ErrServerNotInitialized = -32002
	ErrUnknownResource     = -32001
	ErrInvalidURI          = -32000
)

// Error message constants
const (
	MsgParseError     = "Parse error"
	MsgInvalidRequest = "Invalid request"
	MsgMethodNotFound = "Method not found"
	MsgInvalidParams  = "Invalid params"
	MsgInternalError  = "Internal error"
	
	MsgServerNotInitialized = "Server not initialized"
	MsgUnknownResource     = "Unknown resource"
	MsgInvalidURI          = "Invalid URI"
)

// NewRequest creates a new JSON-RPC request
func NewRequest(id RequestID, method string, params interface{}) *Request {
	return &Request{
		Message: Message{
			JSONRPC: "2.0",
			ID:      &id,
		},
		Method: method,
		Params: params,
	}
}

// NewResponse creates a new JSON-RPC response
func NewResponse(id RequestID, result interface{}) *Response {
	return &Response{
		Message: Message{
			JSONRPC: "2.0",
			ID:      &id,
		},
		Result: result,
	}
}

// NewError creates a new JSON-RPC error response
func NewError(id RequestID, code int, message string, data interface{}) *Error {
	return &Error{
		Message: Message{
			JSONRPC: "2.0",
			ID:      &id,
		},
		Error: &ErrorObject{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
}

// NewNotification creates a new JSON-RPC notification
func NewNotification(method string, params interface{}) *Notification {
	return &Notification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
}