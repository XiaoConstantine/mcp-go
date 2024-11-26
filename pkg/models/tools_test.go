package models

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestToolSerialization(t *testing.T) {
	min := float64(0)
	max := float64(100)
	tool := Tool{
		Name:        "test_tool",
		Description: "A test tool",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]ParameterSchema{
				"param1": {
					Type:        "string",
					Description: "First parameter",
				},
				"param2": {
					Type:        "number",
					Minimum:     &min,
					Maximum:     &max,
					Required:    true,
				},
			},
		},
	}

	data, err := json.Marshal(tool)
	if err != nil {
		t.Fatalf("Failed to marshal Tool: %v", err)
	}

	var decoded Tool
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal Tool: %v", err)
	}

	if diff := cmp.Diff(tool, decoded); diff != "" {
		t.Errorf("Tool mismatch (-want +got):\n%s", diff)
	}
}

func TestCallToolRequestSerialization(t *testing.T) {
	req := CallToolRequest{
		Method: "tools/call",
		Params: CallToolParams{
			Name: "test_tool",
			Arguments: map[string]interface{}{
				"param1": "test value",
				"param2": float64(42),
			},
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal CallToolRequest: %v", err)
	}

	var decoded CallToolRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal CallToolRequest: %v", err)
	}

	if diff := cmp.Diff(req, decoded); diff != "" {
		t.Errorf("CallToolRequest mismatch (-want +got):\n%s", diff)
	}
}

func TestCallToolResultSerialization(t *testing.T) {
	result := CallToolResult{
		Content: []Content{
			TextContent{
				Type: "text",
				Text: "Test result",
			},
			ImageContent{
				Type:     "image",
				Data:     "base64data",
				MimeType: "image/png",
			},
		},
		IsError: false,
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Failed to marshal CallToolResult: %v", err)
	}

	var decoded CallToolResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal CallToolResult: %v", err)
	}

	// Create custom comparer for Content interface
	contentComparer := cmp.Comparer(func(x, y Content) bool {
		switch x := x.(type) {
		case TextContent:
			if y, ok := y.(TextContent); ok {
				return x.Type == y.Type && x.Text == y.Text
			}
		case ImageContent:
			if y, ok := y.(ImageContent); ok {
				return x.Type == y.Type && x.Data == y.Data && x.MimeType == y.MimeType
			}
		}
		return false
	})

	if diff := cmp.Diff(result, decoded, contentComparer); diff != "" {
		t.Errorf("CallToolResult mismatch (-want +got):\n%s", diff)
	}
}

func TestToolErrorScenario(t *testing.T) {
	result := CallToolResult{
		Content: []Content{
			TextContent{
				Type: "text",
				Text: "Error: Invalid parameters",
			},
		},
		IsError: true,
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Failed to marshal error CallToolResult: %v", err)
	}

	var decoded CallToolResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal error CallToolResult: %v", err)
	}

	contentComparer := cmp.Comparer(func(x, y Content) bool {
		switch x := x.(type) {
		case TextContent:
			if y, ok := y.(TextContent); ok {
				return x.Type == y.Type && x.Text == y.Text
			}
		}
		return false
	})

	if diff := cmp.Diff(result, decoded, contentComparer); diff != "" {
		t.Errorf("Error CallToolResult mismatch (-want +got):\n%s", diff)
	}
}
