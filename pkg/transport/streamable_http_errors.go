package transport

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/XiaoConstantine/mcp-go/pkg/protocol"
)

// Standard JSON-RPC error codes.
const (
	// JSON-RPC 2.0 standard error codes.
	JSONRPCParseError     = -32700 // Parse error - Invalid JSON was received by the server
	JSONRPCInvalidRequest = -32600 // Invalid Request - The JSON sent is not a valid Request object
	JSONRPCMethodNotFound = -32601 // Method not found - The method does not exist / is not available
	JSONRPCInvalidParams  = -32602 // Invalid params - Invalid method parameter(s)
	JSONRPCInternalError  = -32603 // Internal error - Internal JSON-RPC error

	// Server error codes (-32000 to -32099 are reserved for implementation-defined server-errors).
	JSONRPCServerError       = -32000 // Generic server error
	JSONRPCSessionRequired   = -32001 // Session ID required
	JSONRPCInvalidSession    = -32002 // Invalid session ID
	JSONRPCConnectionLimit   = -32003 // Connection limit exceeded
	JSONRPCRequestTimeout    = -32004 // Request timeout
	JSONRPCServerBusy        = -32005 // Server busy
	JSONRPCUpgradeNotAllowed = -32006 // SSE upgrade not allowed
	JSONRPCInvalidUpgrade    = -32007 // Invalid upgrade request
)

// MCPError represents a standardized MCP JSON-RPC error.
type MCPError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Error implements the error interface.
func (e *MCPError) Error() string {
	if e.Data != nil {
		return fmt.Sprintf("MCP Error %d: %s (data: %v)", e.Code, e.Message, e.Data)
	}
	return fmt.Sprintf("MCP Error %d: %s", e.Code, e.Message)
}

// ToProtocolError converts MCPError to protocol.ErrorObject.
func (e *MCPError) ToProtocolError() *protocol.ErrorObject {
	return &protocol.ErrorObject{
		Code:    e.Code,
		Message: e.Message,
		Data:    e.Data,
	}
}

// NewMCPError creates a new MCPError.
func NewMCPError(code int, message string, data interface{}) *MCPError {
	return &MCPError{
		Code:    code,
		Message: message,
		Data:    data,
	}
}

// Predefined error constructors for common errors.
func NewParseError(data interface{}) *MCPError {
	return &MCPError{
		Code:    JSONRPCParseError,
		Message: "Parse error",
		Data:    data,
	}
}

func NewInvalidRequestError(data interface{}) *MCPError {
	return &MCPError{
		Code:    JSONRPCInvalidRequest,
		Message: "Invalid Request",
		Data:    data,
	}
}

func NewMethodNotFoundError(method string) *MCPError {
	return &MCPError{
		Code:    JSONRPCMethodNotFound,
		Message: "Method not found",
		Data:    map[string]string{"method": method},
	}
}

func NewInvalidParamsError(data interface{}) *MCPError {
	return &MCPError{
		Code:    JSONRPCInvalidParams,
		Message: "Invalid params",
		Data:    data,
	}
}

func NewInternalError(data interface{}) *MCPError {
	return &MCPError{
		Code:    JSONRPCInternalError,
		Message: "Internal error",
		Data:    data,
	}
}

func NewServerError(data interface{}) *MCPError {
	return &MCPError{
		Code:    JSONRPCServerError,
		Message: "Server error",
		Data:    data,
	}
}

func NewSessionRequiredError() *MCPError {
	return &MCPError{
		Code:    JSONRPCSessionRequired,
		Message: "Session ID required",
	}
}

func NewInvalidSessionError(sessionID string) *MCPError {
	return &MCPError{
		Code:    JSONRPCInvalidSession,
		Message: "Invalid session ID",
		Data:    map[string]string{"sessionId": sessionID},
	}
}

func NewConnectionLimitError(limit int) *MCPError {
	return &MCPError{
		Code:    JSONRPCConnectionLimit,
		Message: "Connection limit exceeded",
		Data:    map[string]int{"limit": limit},
	}
}

func NewRequestTimeoutError(timeout string) *MCPError {
	return &MCPError{
		Code:    JSONRPCRequestTimeout,
		Message: "Request timeout",
		Data:    map[string]string{"timeout": timeout},
	}
}

func NewServerBusyError() *MCPError {
	return &MCPError{
		Code:    JSONRPCServerBusy,
		Message: "Server busy",
	}
}

func NewUpgradeNotAllowedError() *MCPError {
	return &MCPError{
		Code:    JSONRPCUpgradeNotAllowed,
		Message: "SSE upgrade not allowed",
	}
}

func NewInvalidUpgradeError(reason string) *MCPError {
	return &MCPError{
		Code:    JSONRPCInvalidUpgrade,
		Message: "Invalid upgrade request",
		Data:    map[string]string{"reason": reason},
	}
}

// ErrorResponseWriter is a helper for writing standardized error responses.
type ErrorResponseWriter struct {
	writer http.ResponseWriter
}

// NewErrorResponseWriter creates a new ErrorResponseWriter.
func NewErrorResponseWriter(w http.ResponseWriter) *ErrorResponseWriter {
	return &ErrorResponseWriter{writer: w}
}

// WriteError writes a standardized error response.
func (e *ErrorResponseWriter) WriteError(mcpErr *MCPError, requestID interface{}) {
	e.writer.Header().Set("Content-Type", ContentTypeJSON)

	// Determine HTTP status code based on JSON-RPC error code
	httpStatus := e.getHTTPStatusFromJSONRPCCode(mcpErr.Code)
	e.writer.WriteHeader(httpStatus)

	// Create JSON-RPC error response
	response := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      requestID,
		"error": map[string]interface{}{
			"code":    mcpErr.Code,
			"message": mcpErr.Message,
		},
	}

	if mcpErr.Data != nil {
		response["error"].(map[string]interface{})["data"] = mcpErr.Data
	}

	if err := json.NewEncoder(e.writer).Encode(response); err != nil {
		// Log error but don't fail the response
		e.writer.WriteHeader(http.StatusInternalServerError)
	}
}

// WriteHTTPError writes a simple HTTP error without JSON-RPC formatting.
func (e *ErrorResponseWriter) WriteHTTPError(mcpErr *MCPError) {
	httpStatus := e.getHTTPStatusFromJSONRPCCode(mcpErr.Code)
	http.Error(e.writer, mcpErr.Message, httpStatus)
}

// getHTTPStatusFromJSONRPCCode maps JSON-RPC error codes to HTTP status codes.
func (e *ErrorResponseWriter) getHTTPStatusFromJSONRPCCode(code int) int {
	switch code {
	case JSONRPCParseError:
		return http.StatusBadRequest
	case JSONRPCInvalidRequest:
		return http.StatusBadRequest
	case JSONRPCMethodNotFound:
		return http.StatusNotFound
	case JSONRPCInvalidParams:
		return http.StatusBadRequest
	case JSONRPCInternalError:
		return http.StatusInternalServerError
	case JSONRPCSessionRequired:
		return http.StatusBadRequest
	case JSONRPCInvalidSession:
		return http.StatusUnauthorized
	case JSONRPCConnectionLimit:
		return http.StatusTooManyRequests
	case JSONRPCRequestTimeout:
		return http.StatusGatewayTimeout
	case JSONRPCServerBusy:
		return http.StatusServiceUnavailable
	case JSONRPCUpgradeNotAllowed:
		return http.StatusNotImplemented
	case JSONRPCInvalidUpgrade:
		return http.StatusBadRequest
	default:
		// Default server error range (-32000 to -32099)
		if code >= -32099 && code <= -32000 {
			return http.StatusInternalServerError
		}
		return http.StatusInternalServerError
	}
}

// Helper functions for common error scenarios

// HandleParseError handles JSON parsing errors.
func HandleParseError(w http.ResponseWriter, err error, requestID interface{}) {
	errWriter := NewErrorResponseWriter(w)
	parseErr := NewParseError(err.Error())
	errWriter.WriteError(parseErr, requestID)
}

// HandleInvalidRequest handles invalid request errors.
func HandleInvalidRequest(w http.ResponseWriter, reason string, requestID interface{}) {
	errWriter := NewErrorResponseWriter(w)
	invalidErr := NewInvalidRequestError(reason)
	errWriter.WriteError(invalidErr, requestID)
}

// HandleSessionError handles session-related errors.
func HandleSessionError(w http.ResponseWriter, mcpErr *MCPError) {
	errWriter := NewErrorResponseWriter(w)
	errWriter.WriteHTTPError(mcpErr)
}

// HandleInternalError handles internal server errors.
func HandleInternalError(w http.ResponseWriter, err error, requestID interface{}) {
	errWriter := NewErrorResponseWriter(w)
	internalErr := NewInternalError(err.Error())
	errWriter.WriteError(internalErr, requestID)
}
