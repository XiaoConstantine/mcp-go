package protocol

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRequestIDTypes(t *testing.T) {
	// Test string request ID
	var stringID RequestID = "request-123"
	assert.Equal(t, "request-123", stringID)

	// Test integer request ID
	var intID RequestID = 42
	assert.Equal(t, 42, intID)
}

func TestMessageMarshalUnmarshal(t *testing.T) {
	tests := []struct {
		name    string
		msg     Message
		wantErr bool
	}{
		{
			name: "request message",
			msg: Message{
				JSONRPC: "2.0",
				ID:      func() *RequestID { id := RequestID("123"); return &id }(),
				Method:  "test_method",
				Params: map[string]interface{}{
					"key": "value",
				},
			},
			wantErr: false,
		},
		{
			name: "response message",
			msg: Message{
				JSONRPC: "2.0",
				ID:      func() *RequestID { id := RequestID("123"); return &id }(),
				Result:  "test result",
			},
			wantErr: false,
		},
		{
			name: "error message",
			msg: Message{
				JSONRPC: "2.0",
				ID:      func() *RequestID { id := RequestID("123"); return &id }(),
				Error: &ErrorObject{
					Code:    -32600,
					Message: "Invalid Request",
				},
			},
			wantErr: false,
		},
		{
			name: "notification message",
			msg: Message{
				JSONRPC: "2.0",
				Method:  "test_notification",
				Params: map[string]interface{}{
					"key": "value",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.msg)
			if (err != nil) != tt.wantErr {
				t.Errorf("json.Marshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			var got Message
			err = json.Unmarshal(data, &got)
			if (err != nil) != tt.wantErr {
				t.Errorf("json.Unmarshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Basic validation of unmarshaled data
			if got.JSONRPC != tt.msg.JSONRPC {
				t.Errorf("JSONRPC = %v, want %v", got.JSONRPC, tt.msg.JSONRPC)
			}
			if got.Method != tt.msg.Method {
				t.Errorf("Method = %v, want %v", got.Method, tt.msg.Method)
			}
		})
	}
}
