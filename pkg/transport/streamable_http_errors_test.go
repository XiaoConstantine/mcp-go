package transport

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMCPError(t *testing.T) {
	tests := []struct {
		name     string
		err      *MCPError
		wantCode int
		wantMsg  string
	}{
		{
			name:     "Parse error",
			err:      NewParseError("invalid JSON"),
			wantCode: JSONRPCParseError,
			wantMsg:  "Parse error",
		},
		{
			name:     "Invalid request",
			err:      NewInvalidRequestError("missing method"),
			wantCode: JSONRPCInvalidRequest,
			wantMsg:  "Invalid Request",
		},
		{
			name:     "Session required",
			err:      NewSessionRequiredError(),
			wantCode: JSONRPCSessionRequired,
			wantMsg:  "Session ID required",
		},
		{
			name:     "Connection limit",
			err:      NewConnectionLimitError(100),
			wantCode: JSONRPCConnectionLimit,
			wantMsg:  "Connection limit exceeded",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Code != tt.wantCode {
				t.Errorf("MCPError.Code = %d, want %d", tt.err.Code, tt.wantCode)
			}
			if tt.err.Message != tt.wantMsg {
				t.Errorf("MCPError.Message = %s, want %s", tt.err.Message, tt.wantMsg)
			}

			// Test Error() method
			errStr := tt.err.Error()
			if errStr == "" {
				t.Error("MCPError.Error() returned empty string")
			}

			// Test ToProtocolError()
			protocolErr := tt.err.ToProtocolError()
			if protocolErr.Code != tt.wantCode {
				t.Errorf("ToProtocolError().Code = %d, want %d", protocolErr.Code, tt.wantCode)
			}
		})
	}
}

func TestErrorResponseWriter(t *testing.T) {
	tests := []struct {
		name           string
		err            *MCPError
		requestID      interface{}
		wantHTTPStatus int
	}{
		{
			name:           "Parse error",
			err:            NewParseError(nil),
			requestID:      "test-1",
			wantHTTPStatus: http.StatusBadRequest,
		},
		{
			name:           "Internal error",
			err:            NewInternalError(nil),
			requestID:      42,
			wantHTTPStatus: http.StatusInternalServerError,
		},
		{
			name:           "Session required",
			err:            NewSessionRequiredError(),
			requestID:      nil,
			wantHTTPStatus: http.StatusBadRequest,
		},
		{
			name:           "Connection limit",
			err:            NewConnectionLimitError(100),
			requestID:      "limit-test",
			wantHTTPStatus: http.StatusTooManyRequests,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			errWriter := NewErrorResponseWriter(rr)

			errWriter.WriteError(tt.err, tt.requestID)

			if rr.Code != tt.wantHTTPStatus {
				t.Errorf("WriteError() status = %d, want %d", rr.Code, tt.wantHTTPStatus)
			}

			// Check content type
			contentType := rr.Header().Get("Content-Type")
			if contentType != ContentTypeJSON {
				t.Errorf("WriteError() Content-Type = %s, want %s", contentType, ContentTypeJSON)
			}

			// Check JSON structure
			var response map[string]interface{}
			if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
				t.Fatalf("WriteError() response not valid JSON: %v", err)
			}

			if response["jsonrpc"] != "2.0" {
				t.Error("WriteError() response missing jsonrpc field")
			}

			// Handle JSON number conversion
			if tt.requestID != nil {
				if jsonID, ok := response["id"].(float64); ok && tt.requestID == 42 {
					if int(jsonID) != 42 {
						t.Errorf("WriteError() response id = %v, want %v", response["id"], tt.requestID)
					}
				} else if response["id"] != tt.requestID {
					t.Errorf("WriteError() response id = %v, want %v", response["id"], tt.requestID)
				}
			}

			errorObj, ok := response["error"].(map[string]interface{})
			if !ok {
				t.Fatal("WriteError() response missing error object")
			}

			if int(errorObj["code"].(float64)) != tt.err.Code {
				t.Errorf("WriteError() error code = %v, want %d", errorObj["code"], tt.err.Code)
			}

			if errorObj["message"] != tt.err.Message {
				t.Errorf("WriteError() error message = %v, want %s", errorObj["message"], tt.err.Message)
			}
		})
	}
}

func TestErrorResponseWriter_WriteHTTPError(t *testing.T) {
	tests := []struct {
		name           string
		err            *MCPError
		wantHTTPStatus int
	}{
		{
			name:           "Session required",
			err:            NewSessionRequiredError(),
			wantHTTPStatus: http.StatusBadRequest,
		},
		{
			name:           "Invalid session",
			err:            NewInvalidSessionError("bad-session"),
			wantHTTPStatus: http.StatusUnauthorized,
		},
		{
			name:           "Server busy",
			err:            NewServerBusyError(),
			wantHTTPStatus: http.StatusServiceUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			errWriter := NewErrorResponseWriter(rr)

			errWriter.WriteHTTPError(tt.err)

			if rr.Code != tt.wantHTTPStatus {
				t.Errorf("WriteHTTPError() status = %d, want %d", rr.Code, tt.wantHTTPStatus)
			}

			// Check that body contains error message
			body := rr.Body.String()
			if body == "" {
				t.Error("WriteHTTPError() returned empty body")
			}
		})
	}
}

func TestHandleFunctions(t *testing.T) {
	t.Run("HandleParseError", func(t *testing.T) {
		rr := httptest.NewRecorder()
		HandleParseError(rr, &json.SyntaxError{}, "test-id")

		if rr.Code != http.StatusBadRequest {
			t.Errorf("HandleParseError() status = %d, want %d", rr.Code, http.StatusBadRequest)
		}
	})

	t.Run("HandleInvalidRequest", func(t *testing.T) {
		rr := httptest.NewRecorder()
		HandleInvalidRequest(rr, "test reason", "test-id")

		if rr.Code != http.StatusBadRequest {
			t.Errorf("HandleInvalidRequest() status = %d, want %d", rr.Code, http.StatusBadRequest)
		}
	})

	t.Run("HandleSessionError", func(t *testing.T) {
		rr := httptest.NewRecorder()
		HandleSessionError(rr, NewSessionRequiredError())

		if rr.Code != http.StatusBadRequest {
			t.Errorf("HandleSessionError() status = %d, want %d", rr.Code, http.StatusBadRequest)
		}
	})

	t.Run("HandleInternalError", func(t *testing.T) {
		rr := httptest.NewRecorder()
		HandleInternalError(rr, &json.SyntaxError{}, "test-id")

		if rr.Code != http.StatusInternalServerError {
			t.Errorf("HandleInternalError() status = %d, want %d", rr.Code, http.StatusInternalServerError)
		}
	})
}