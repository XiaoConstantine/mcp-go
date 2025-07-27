package transport

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/XiaoConstantine/mcp-go/pkg/protocol"
)

func TestNewMCPError(t *testing.T) {
	code := -32600
	message := "Invalid Request"
	data := map[string]string{"test": "data"}
	
	err := NewMCPError(code, message, data)
	
	if err.Code != code {
		t.Errorf("Code = %d, want %d", err.Code, code)
	}
	
	if err.Message != message {
		t.Errorf("Message = %s, want %s", err.Message, message)
	}
	
	// Compare data (maps can't be compared directly)
	dataMap, ok := err.Data.(map[string]string)
	if !ok {
		t.Error("Data is not map[string]string")
	} else {
		for k, v := range data {
			if dataMap[k] != v {
				t.Errorf("Data[%s] = %s, want %s", k, dataMap[k], v)
			}
		}
	}
}

func TestNewMethodNotFoundError(t *testing.T) {
	method := "unknown/method"
	err := NewMethodNotFoundError(method)
	
	if err.Code != protocol.ErrCodeMethodNotFound {
		t.Errorf("Code = %d, want %d", err.Code, protocol.ErrCodeMethodNotFound)
	}
	
	if err.Message != "Method not found" {
		t.Errorf("Message = %s, want 'Method not found'", err.Message)
	}
	
	// Check data contains method
	data, ok := err.Data.(map[string]string)
	if !ok {
		t.Error("Data is not map[string]string")
	}
	
	if data["method"] != method {
		t.Errorf("Data method = %s, want %s", data["method"], method)
	}
}

func TestNewInvalidParamsError(t *testing.T) {
	data := "missing required parameter"
	err := NewInvalidParamsError(data)
	
	if err.Code != protocol.ErrCodeInvalidParams {
		t.Errorf("Code = %d, want %d", err.Code, protocol.ErrCodeInvalidParams)
	}
	
	if err.Message != "Invalid params" {
		t.Errorf("Message = %s, want 'Invalid params'", err.Message)
	}
	
	// For string data, direct comparison is fine
	if err.Data != data {
		t.Errorf("Data = %v, want %v", err.Data, data)
	}
}

func TestNewRequestTimeoutError(t *testing.T) {
	timeout := "30s"
	err := NewRequestTimeoutError(timeout)
	
	if err.Code != protocol.ErrCodeRequestTimeout {
		t.Errorf("Code = %d, want %d", err.Code, protocol.ErrCodeRequestTimeout)
	}
	
	if err.Message != "Request timeout" {
		t.Errorf("Message = %s, want 'Request timeout'", err.Message)
	}
	
	// Check data contains timeout
	data, ok := err.Data.(map[string]string)
	if !ok {
		t.Error("Data is not map[string]string")
	}
	
	if data["timeout"] != timeout {
		t.Errorf("Data timeout = %s, want %s", data["timeout"], timeout)
	}
}

func TestNewUpgradeNotAllowedError(t *testing.T) {
	err := NewUpgradeNotAllowedError()
	
	if err.Code != protocol.ErrCodeUpgradeNotAllowed {
		t.Errorf("Code = %d, want %d", err.Code, protocol.ErrCodeUpgradeNotAllowed)
	}
	
	if err.Message != "SSE upgrade not allowed" {
		t.Errorf("Message = %s, want 'SSE upgrade not allowed'", err.Message)
	}
}

func TestNewInvalidUpgradeError(t *testing.T) {
	reason := "missing Accept header"
	err := NewInvalidUpgradeError(reason)
	
	if err.Code != protocol.ErrCodeInvalidUpgrade {
		t.Errorf("Code = %d, want %d", err.Code, protocol.ErrCodeInvalidUpgrade)
	}
	
	if err.Message != "Invalid upgrade request" {
		t.Errorf("Message = %s, want 'Invalid upgrade request'", err.Message)
	}
	
	// Check data contains reason
	data, ok := err.Data.(map[string]string)
	if !ok {
		t.Error("Data is not map[string]string")
	}
	
	if data["reason"] != reason {
		t.Errorf("Data reason = %s, want %s", data["reason"], reason)
	}
}

func TestErrorResponseWriter_getHTTPStatusFromJSONRPCCode_AllCodes(t *testing.T) {
	w := httptest.NewRecorder()
	errWriter := NewErrorResponseWriter(w)
	
	tests := []struct {
		code           int
		expectedStatus int
	}{
		{protocol.ErrCodeParseError, http.StatusBadRequest},
		{protocol.ErrCodeInvalidRequest, http.StatusBadRequest},
		{protocol.ErrCodeMethodNotFound, http.StatusNotFound},
		{protocol.ErrCodeInvalidParams, http.StatusBadRequest},
		{protocol.ErrCodeInternalError, http.StatusInternalServerError},
		{protocol.ErrCodeSessionRequired, http.StatusBadRequest},
		{protocol.ErrCodeInvalidSession, http.StatusUnauthorized},
		{protocol.ErrCodeConnectionLimit, http.StatusTooManyRequests},
		{protocol.ErrCodeRequestTimeout, http.StatusGatewayTimeout},
		{protocol.ErrCodeServerBusy, http.StatusServiceUnavailable},
		{protocol.ErrCodeUpgradeNotAllowed, http.StatusNotImplemented},
		{protocol.ErrCodeInvalidUpgrade, http.StatusBadRequest},
		{-32050, http.StatusInternalServerError}, // Server error range
		{-1000, http.StatusInternalServerError},  // Unknown error
	}
	
	for _, tt := range tests {
		t.Run(fmt.Sprintf("code_%d", tt.code), func(t *testing.T) {
			status := errWriter.getHTTPStatusFromJSONRPCCode(tt.code)
			if status != tt.expectedStatus {
				t.Errorf("getHTTPStatusFromJSONRPCCode(%d) = %d, want %d", 
					tt.code, status, tt.expectedStatus)
			}
		})
	}
}

func TestErrorResponseWriter_WriteError_WithData(t *testing.T) {
	w := httptest.NewRecorder()
	errWriter := NewErrorResponseWriter(w)
	
	mcpErr := &MCPError{
		Code:    protocol.ErrCodeInvalidRequest,
		Message: "Invalid Request",
		Data:    map[string]string{"field": "missing"},
	}
	
	errWriter.WriteError(mcpErr, 123)
	
	if w.Code != http.StatusBadRequest {
		t.Errorf("HTTP status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	
	// Check content type
	contentType := w.Header().Get("Content-Type")
	if contentType != ContentTypeJSON {
		t.Errorf("Content-Type = %s, want %s", contentType, ContentTypeJSON)
	}
	
	// Verify response body contains data
	body := w.Body.String()
	if !strings.Contains(body, "missing") {
		t.Error("Response body should contain error data")
	}
}

func TestMin(t *testing.T) {
	tests := []struct {
		a, b     int
		expected int
	}{
		{1, 2, 1},
		{5, 3, 3},
		{10, 10, 10},
		{-1, 0, -1},
		{0, -5, -5},
	}
	
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d_%d", tt.a, tt.b), func(t *testing.T) {
			result := min(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("min(%d, %d) = %d, want %d", tt.a, tt.b, result, tt.expected)
			}
		})
	}
}