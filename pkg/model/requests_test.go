package models

import (
	"encoding/json"
	"testing"

	"github.com/XiaoConstantine/mcp-go/pkg/protocol"
)

func TestInitializeRequest(t *testing.T) {
	request := InitializeRequest{
		Method: "initialize",
		Params: struct {
			Capabilities    protocol.ClientCapabilities `json:"capabilities"`
			ClientInfo      Implementation              `json:"clientInfo"`
			ProtocolVersion string                      `json:"protocolVersion"`
		}{
			Capabilities: protocol.ClientCapabilities{
				Sampling: map[string]interface{}{
					"enabled": true,
				},
			},
			ClientInfo: Implementation{
				Name:    "test-client",
				Version: "1.0.0",
			},
			ProtocolVersion: "1.0",
		},
	}

	// Test marshaling
	data, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("Failed to marshal InitializeRequest: %v", err)
	}

	// Test unmarshaling
	var decoded InitializeRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal InitializeRequest: %v", err)
	}

	// Verify fields
	if decoded.Method != request.Method {
		t.Errorf("Method mismatch. Got %s, want %s", decoded.Method, request.Method)
	}
	if decoded.Params.ProtocolVersion != request.Params.ProtocolVersion {
		t.Errorf("ProtocolVersion mismatch. Got %s, want %s",
			decoded.Params.ProtocolVersion, request.Params.ProtocolVersion)
	}
	if decoded.Params.ClientInfo.Name != request.Params.ClientInfo.Name {
		t.Errorf("ClientInfo.Name mismatch. Got %s, want %s",
			decoded.Params.ClientInfo.Name, request.Params.ClientInfo.Name)
	}
}

func TestInitializeResult(t *testing.T) {
	result := InitializeResult{
		Capabilities: protocol.ServerCapabilities{
			Logging: map[string]interface{}{
				"enabled": true,
			},
		},
		Instructions:    "Test instructions",
		ProtocolVersion: "1.0",
		ServerInfo: Implementation{
			Name:    "test-server",
			Version: "1.0.0",
		},
	}

	// Test marshaling
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Failed to marshal InitializeResult: %v", err)
	}

	// Test unmarshaling
	var decoded InitializeResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal InitializeResult: %v", err)
	}

	// Verify fields
	if decoded.Instructions != result.Instructions {
		t.Errorf("Instructions mismatch. Got %s, want %s", decoded.Instructions, result.Instructions)
	}
	if decoded.ProtocolVersion != result.ProtocolVersion {
		t.Errorf("ProtocolVersion mismatch. Got %s, want %s", decoded.ProtocolVersion, result.ProtocolVersion)
	}
}

func TestListResourcesRequest(t *testing.T) {
	cursor := Cursor("test-cursor")
	request := ListResourcesRequest{
		Method: "resources/list",
		Params: struct {
			Cursor *Cursor `json:"cursor,omitempty"`
		}{
			Cursor: &cursor,
		},
	}

	// Test marshaling
	data, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("Failed to marshal ListResourcesRequest: %v", err)
	}

	// Test unmarshaling
	var decoded ListResourcesRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal ListResourcesRequest: %v", err)
	}

	// Verify fields
	if decoded.Method != request.Method {
		t.Errorf("Method mismatch. Got %s, want %s", decoded.Method, request.Method)
	}
	if *decoded.Params.Cursor != *request.Params.Cursor {
		t.Errorf("Cursor mismatch. Got %s, want %s", *decoded.Params.Cursor, *request.Params.Cursor)
	}
}

func TestCreateMessageRequest(t *testing.T) {
	temp := 0.7
	request := CreateMessageRequest{
		Method: "sampling/createMessage",
		Params: struct {
			MaxTokens        int                    `json:"maxTokens"`
			Messages         []SamplingMessage      `json:"messages"`
			SystemPrompt     string                 `json:"systemPrompt,omitempty"`
			Temperature      *float64               `json:"temperature,omitempty"`
			StopSequences    []string               `json:"stopSequences,omitempty"`
			ModelPreferences *ModelPreferences      `json:"modelPreferences,omitempty"`
			IncludeContext   string                 `json:"includeContext,omitempty"`
			Metadata         map[string]interface{} `json:"metadata,omitempty"`
		}{
			MaxTokens:    100,
			Temperature:  &temp,
			SystemPrompt: "Test system prompt",
			Messages: []SamplingMessage{
				{
					Role: RoleUser,
					Content: TextContent{
						Type: "text",
						Text: "Hello",
					},
				},
			},
			StopSequences: []string{"stop1", "stop2"},
			Metadata: map[string]interface{}{
				"key": "value",
			},
		},
	}

	// Test marshaling
	data, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("Failed to marshal CreateMessageRequest: %v", err)
	}

	// Test unmarshaling
	var decoded CreateMessageRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal CreateMessageRequest: %v", err)
	}

	// Verify fields
	if decoded.Method != request.Method {
		t.Errorf("Method mismatch. Got %s, want %s", decoded.Method, request.Method)
	}
	if decoded.Params.MaxTokens != request.Params.MaxTokens {
		t.Errorf("MaxTokens mismatch. Got %d, want %d", decoded.Params.MaxTokens, request.Params.MaxTokens)
	}
	if *decoded.Params.Temperature != *request.Params.Temperature {
		t.Errorf("Temperature mismatch. Got %f, want %f", *decoded.Params.Temperature, *request.Params.Temperature)
	}
}

func TestCreateMessageResult(t *testing.T) {
	result := CreateMessageResult{
		Content: TextContent{
			Type: "text",
			Text: "Response text",
		},
		Model:      "test-model",
		Role:       RoleAssistant,
		StopReason: "length",
	}

	// Test marshaling
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Failed to marshal CreateMessageResult: %v", err)
	}

	// Test unmarshaling
	var decoded CreateMessageResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal CreateMessageResult: %v", err)
	}

	// Verify fields
	if decoded.Model != result.Model {
		t.Errorf("Model mismatch. Got %s, want %s", decoded.Model, result.Model)
	}
	if decoded.Role != result.Role {
		t.Errorf("Role mismatch. Got %s, want %s", decoded.Role, result.Role)
	}
	if decoded.StopReason != result.StopReason {
		t.Errorf("StopReason mismatch. Got %s, want %s", decoded.StopReason, result.StopReason)
	}
}

func TestRoot(t *testing.T) {
	root := Root{
		URI:  "file:///path/to/root",
		Name: "test-root",
	}

	// Test marshaling
	data, err := json.Marshal(root)
	if err != nil {
		t.Fatalf("Failed to marshal Root: %v", err)
	}

	// Test unmarshaling
	var decoded Root
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal Root: %v", err)
	}

	// Verify fields
	if decoded.URI != root.URI {
		t.Errorf("URI mismatch. Got %s, want %s", decoded.URI, root.URI)
	}
	if decoded.Name != root.Name {
		t.Errorf("Name mismatch. Got %s, want %s", decoded.Name, root.Name)
	}
}

func TestListRootsResult(t *testing.T) {
	result := ListRootsResult{
		Roots: []Root{
			{
				URI:  "file:///path/to/root1",
				Name: "root1",
			},
			{
				URI:  "file:///path/to/root2",
				Name: "root2",
			},
		},
	}

	// Test marshaling
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Failed to marshal ListRootsResult: %v", err)
	}

	// Test unmarshaling
	var decoded ListRootsResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal ListRootsResult: %v", err)
	}

	// Verify fields
	if len(decoded.Roots) != len(result.Roots) {
		t.Errorf("Roots length mismatch. Got %d, want %d", len(decoded.Roots), len(result.Roots))
	}
	for i := range decoded.Roots {
		if decoded.Roots[i].URI != result.Roots[i].URI {
			t.Errorf("Root[%d] URI mismatch. Got %s, want %s", i, decoded.Roots[i].URI, result.Roots[i].URI)
		}
		if decoded.Roots[i].Name != result.Roots[i].Name {
			t.Errorf("Root[%d] Name mismatch. Got %s, want %s", i, decoded.Roots[i].Name, result.Roots[i].Name)
		}
	}
}
