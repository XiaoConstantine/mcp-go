package models

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestPromptSerialization(t *testing.T) {
	// Create a comprehensive test prompt with all fields populated
	prompt := Prompt{
		Name:        "test_prompt",
		Description: "A test prompt",
		Arguments: []PromptArgument{
			{
				Name:        "arg1",
				Description: "First argument",
				Required:    true,
			},
			{
				Name:        "arg2",
				Description: "Second argument",
				Required:    false,
			},
		},
	}

	// Test marshaling to JSON
	data, err := json.Marshal(prompt)
	if err != nil {
		t.Fatalf("Failed to marshal Prompt: %v", err)
	}

	// Test unmarshaling from JSON
	var decoded Prompt
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal Prompt: %v", err)
	}

	// Compare using cmp.Diff
	if diff := cmp.Diff(prompt, decoded); diff != "" {
		t.Errorf("Prompt mismatch (-want +got):\n%s", diff)
	}
}

func TestListPromptsRequestSerialization(t *testing.T) {
	// Create a request with a cursor
	cursor := Cursor("test-cursor")
	req := ListPromptsRequest{
		Method: "prompts/list",
		Params: struct {
			Cursor *Cursor "json:\"cursor,omitempty\""
		}{
			Cursor: &cursor,
		},
	}

	// Test marshaling to JSON
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal ListPromptsRequest: %v", err)
	}

	// Test unmarshaling from JSON
	var decoded ListPromptsRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal ListPromptsRequest: %v", err)
	}

	// Compare using cmp.Diff
	if diff := cmp.Diff(req, decoded); diff != "" {
		t.Errorf("ListPromptsRequest mismatch (-want +got):\n%s", diff)
	}
}

func TestGetPromptResultSerialization(t *testing.T) {
	// Create a test result with messages
	result := GetPromptResult{
		Messages: []PromptMessage{
			{
				Role: RoleAssistant,
				Content: TextContent{
					Type: "text",
					Text: "Test message",
				},
			},
		},
		Description: "Test description",
	}

	// Test marshaling to JSON
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Failed to marshal GetPromptResult: %v", err)
	}

	// Test unmarshaling from JSON
	var decoded GetPromptResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal GetPromptResult: %v", err)
	}

	// Create a custom comparer for the Content interface
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

	// Compare using cmp.Diff with the custom comparer
	if diff := cmp.Diff(result, decoded, contentComparer); diff != "" {
		t.Errorf("GetPromptResult mismatch (-want +got):\n%s", diff)
	}
}
