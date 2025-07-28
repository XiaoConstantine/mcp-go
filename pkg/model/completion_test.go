package models

import (
	"encoding/json"
	"testing"
)

func TestCompleteRequest(t *testing.T) {
	// Test creation of a request with prompt reference
	promptRef := PromptReference{
		Type: "ref/prompt",
		Name: "test-prompt",
	}
	req := NewCompleteRequest("arg1", "test", promptRef)

	// Test JSON marshaling
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal CompleteRequest: %v", err)
	}

	// Test JSON unmarshaling
	var decoded CompleteRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal CompleteRequest: %v", err)
	}

	// Verify fields
	if decoded.Method != "completion/complete" {
		t.Errorf("Wrong method: got %s, want completion/complete", decoded.Method)
	}
	if decoded.Params.Argument.Name != "arg1" {
		t.Errorf("Wrong argument name: got %s, want arg1", decoded.Params.Argument.Name)
	}
	if decoded.Params.Argument.Value != "test" {
		t.Errorf("Wrong argument value: got %s, want test", decoded.Params.Argument.Value)
	}

	// Test validation
	if err := ValidateCompleteRequest(req); err != nil {
		t.Errorf("Validation failed for valid request: %v", err)
	}

	// Test validation with invalid request
	invalidReq := &CompleteRequest{}
	if err := ValidateCompleteRequest(invalidReq); err == nil {
		t.Error("Expected validation to fail for invalid request")
	}
}

func TestCompleteResult(t *testing.T) {
	// Test creation of a result
	total := 5
	result := NewCompleteResult([]string{"opt1", "opt2"}, true, &total)

	// Test JSON marshaling
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Failed to marshal CompleteResult: %v", err)
	}

	// Test JSON unmarshaling
	var decoded CompleteResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal CompleteResult: %v", err)
	}

	// Verify fields
	if len(decoded.Completion.Values) != 2 {
		t.Errorf("Wrong number of values: got %d, want 2", len(decoded.Completion.Values))
	}
	if !decoded.Completion.HasMore {
		t.Error("Expected HasMore to be true")
	}
	if *decoded.Completion.Total != total {
		t.Errorf("Wrong total: got %d, want %d", *decoded.Completion.Total, total)
	}

	// Test validation
	if err := ValidateCompleteResult(result); err != nil {
		t.Errorf("Validation failed for valid result: %v", err)
	}

	// Test validation with invalid result
	invalidResult := &CompleteResult{}
	if err := ValidateCompleteResult(invalidResult); err == nil {
		t.Error("Expected validation to fail for invalid result")
	}
}

func TestCompleteRequestWithResourceRef(t *testing.T) {
	// Test creation of a request with resource reference
	resourceRef := ResourceReference{
		Type: "ref/resource",
		URI:  "file:///test/path",
	}
	req := NewCompleteRequest("arg1", "test", resourceRef)

	// Test JSON marshaling
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal CompleteRequest with resource ref: %v", err)
	}

	// Test JSON unmarshaling
	var decoded CompleteRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal CompleteRequest with resource ref: %v", err)
	}

	// Verify fields
	if err := ValidateCompleteRequest(req); err != nil {
		t.Errorf("Validation failed for valid request with resource ref: %v", err)
	}
}

func TestCompleteRequestValidation(t *testing.T) {
	tests := []struct {
		name    string
		request *CompleteRequest
		wantErr bool
	}{
		{
			name: "valid request with prompt reference",
			request: NewCompleteRequest("arg1", "test", PromptReference{
				Type: "ref/prompt",
				Name: "test-prompt",
			}),
			wantErr: false,
		},
		{
			name: "valid request with resource reference",
			request: NewCompleteRequest("arg1", "test", ResourceReference{
				Type: "ref/resource",
				URI:  "file://test.txt",
			}),
			wantErr: false,
		},
		{
			name: "empty argument name",
			request: &CompleteRequest{
				Method: "completion/complete",
				Params: struct {
					Argument struct {
						Name  string `json:"name"`
						Value string `json:"value"`
					} `json:"argument"`
					Ref interface{} `json:"ref"`
				}{
					Argument: struct {
						Name  string `json:"name"`
						Value string `json:"value"`
					}{
						Name:  "", // Empty name
						Value: "test",
					},
					Ref: PromptReference{Name: "test"},
				},
			},
			wantErr: true,
		},
		{
			name: "nil reference",
			request: &CompleteRequest{
				Method: "completion/complete",
				Params: struct {
					Argument struct {
						Name  string `json:"name"`
						Value string `json:"value"`
					} `json:"argument"`
					Ref interface{} `json:"ref"`
				}{
					Argument: struct {
						Name  string `json:"name"`
						Value string `json:"value"`
					}{
						Name:  "test",
						Value: "test",
					},
					Ref: nil, // Nil reference
				},
			},
			wantErr: true,
		},
		{
			name: "invalid reference type",
			request: &CompleteRequest{
				Method: "completion/complete",
				Params: struct {
					Argument struct {
						Name  string `json:"name"`
						Value string `json:"value"`
					} `json:"argument"`
					Ref interface{} `json:"ref"`
				}{
					Argument: struct {
						Name  string `json:"name"`
						Value string `json:"value"`
					}{
						Name:  "test",
						Value: "test",
					},
					Ref: "invalid reference type", // Invalid type
				},
			},
			wantErr: true,
		},
		{
			name: "prompt reference with empty name",
			request: &CompleteRequest{
				Method: "completion/complete",
				Params: struct {
					Argument struct {
						Name  string `json:"name"`
						Value string `json:"value"`
					} `json:"argument"`
					Ref interface{} `json:"ref"`
				}{
					Argument: struct {
						Name  string `json:"name"`
						Value string `json:"value"`
					}{
						Name:  "test",
						Value: "test",
					},
					Ref: PromptReference{Name: ""}, // Empty prompt name
				},
			},
			wantErr: true,
		},
		{
			name: "resource reference with empty URI",
			request: &CompleteRequest{
				Method: "completion/complete",
				Params: struct {
					Argument struct {
						Name  string `json:"name"`
						Value string `json:"value"`
					} `json:"argument"`
					Ref interface{} `json:"ref"`
				}{
					Argument: struct {
						Name  string `json:"name"`
						Value string `json:"value"`
					}{
						Name:  "test",
						Value: "test",
					},
					Ref: ResourceReference{URI: ""}, // Empty resource URI
				},
			},
			wantErr: true,
		},
		{
			name: "wrong method",
			request: &CompleteRequest{
				Method: "wrong/method",
				Params: struct {
					Argument struct {
						Name  string `json:"name"`
						Value string `json:"value"`
					} `json:"argument"`
					Ref interface{} `json:"ref"`
				}{
					Argument: struct {
						Name  string `json:"name"`
						Value string `json:"value"`
					}{
						Name:  "test",
						Value: "test",
					},
					Ref: PromptReference{Name: "test"},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCompleteRequest(tt.request)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCompleteRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCompleteResultValidation(t *testing.T) {
	tests := []struct {
		name    string
		result  *CompleteResult
		wantErr bool
	}{
		{
			name: "valid result without total",
			result: &CompleteResult{
				Completion: struct {
					Values  []string `json:"values"`
					HasMore bool     `json:"hasMore,omitempty"`
					Total   *int     `json:"total,omitempty"`
				}{
					Values: []string{"test1", "test2"},
				},
			},
			wantErr: false,
		},
		{
			name: "valid result with total",
			result: func() *CompleteResult {
				total := 5
				return NewCompleteResult([]string{"test1", "test2"}, false, &total)
			}(),
			wantErr: false,
		},
		{
			name: "invalid total less than values",
			result: func() *CompleteResult {
				total := 1
				return NewCompleteResult([]string{"test1", "test2"}, false, &total)
			}(),
			wantErr: true,
		},
		{
			name:    "nil values",
			result:  &CompleteResult{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCompleteResult(tt.result)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCompleteResult() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
