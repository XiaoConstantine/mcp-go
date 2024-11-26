package protocol

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestJSONRPCRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     *Request
		wantErr bool
	}{
		{
			name: "basic request",
			req: NewRequest("123", "test_method", map[string]string{
				"param1": "value1",
			}),
			wantErr: false,
		},
		{
			name: "request with nil params",
			req:  NewRequest(1, "test_method", nil),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("json.Marshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err == nil {
				var got Request
				err = json.Unmarshal(data, &got)
				if err != nil {
					t.Errorf("json.Unmarshal() error = %v", err)
					return
				}

				if got.JSONRPC != "2.0" {
					t.Errorf("JSONRPC version mismatch, got = %v, want = 2.0", got.JSONRPC)
				}

				if got.Method != tt.req.Method {
					t.Errorf("Method mismatch, got = %v, want = %v", got.Method, tt.req.Method)
				}
			}
		})
	}
}

func TestJSONRPCResponse(t *testing.T) {
	tests := []struct {
		name    string
		resp    *Response
		wantErr bool
	}{
		{
			name: "basic response",
			resp: NewResponse("123", map[string]string{
				"result": "success",
			}),
			wantErr: false,
		},
		{
			name: "response with nil result",
			resp: NewResponse(1, nil),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.resp)
			if (err != nil) != tt.wantErr {
				t.Errorf("json.Marshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err == nil {
				var got Response
				err = json.Unmarshal(data, &got)
				if err != nil {
					t.Errorf("json.Unmarshal() error = %v", err)
					return
				}

				if got.JSONRPC != "2.0" {
					t.Errorf("JSONRPC version mismatch, got = %v, want = 2.0", got.JSONRPC)
				}

				if !reflect.DeepEqual(got.Result, tt.resp.Result) {
					t.Errorf("Result mismatch, got = %v, want = %v", got.Result, tt.resp.Result)
				}
			}
		})
	}
}

func TestJSONRPCError(t *testing.T) {
	tests := []struct {
		name    string
		err     *Error
		wantErr bool
	}{
		{
			name: "parse error",
			err:  NewError("123", ErrParseError, MsgParseError, nil),
			wantErr: false,
		},
		{
			name: "error with data",
			err:  NewError(1, ErrInvalidParams, MsgInvalidParams, "Invalid parameter: id"),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.err)
			if (err != nil) != tt.wantErr {
				t.Errorf("json.Marshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err == nil {
				var got Error
				err = json.Unmarshal(data, &got)
				if err != nil {
					t.Errorf("json.Unmarshal() error = %v", err)
					return
				}

				if got.JSONRPC != "2.0" {
					t.Errorf("JSONRPC version mismatch, got = %v, want = 2.0", got.JSONRPC)
				}

				if got.Error.Code != tt.err.Error.Code {
					t.Errorf("Error code mismatch, got = %v, want = %v", got.Error.Code, tt.err.Error.Code)
				}

				if got.Error.Message != tt.err.Error.Message {
					t.Errorf("Error message mismatch, got = %v, want = %v", got.Error.Message, tt.err.Error.Message)
				}
			}
		})
	}
}

func TestJSONRPCNotification(t *testing.T) {
	tests := []struct {
		name    string
		notif   *Notification
		wantErr bool
	}{
		{
			name: "basic notification",
			notif: NewNotification("test_notification", map[string]string{
				"param1": "value1",
			}),
			wantErr: false,
		},
		{
			name: "notification with nil params",
			notif: NewNotification("test_notification", nil),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.notif)
			if (err != nil) != tt.wantErr {
				t.Errorf("json.Marshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err == nil {
				var got Notification
				err = json.Unmarshal(data, &got)
				if err != nil {
					t.Errorf("json.Unmarshal() error = %v", err)
					return
				}

				if got.JSONRPC != "2.0" {
					t.Errorf("JSONRPC version mismatch, got = %v, want = 2.0", got.JSONRPC)
				}

				if got.Method != tt.notif.Method {
					t.Errorf("Method mismatch, got = %v, want = %v", got.Method, tt.notif.Method)
				}
			}
		})
	}
}