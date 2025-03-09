package models

import (
	"encoding/json"
	"testing"
)

func TestTextContent(t *testing.T) {
	priority := 0.5
	textContent := TextContent{
		BaseAnnotated: BaseAnnotated{
			Annotations: &Annotations{
				Audience: []Role{RoleAssistant},
				Priority: &priority,
			},
		},
		Type: "text",
		Text: "Hello, world!",
	}

	// Test marshaling
	data, err := json.Marshal(textContent)
	if err != nil {
		t.Fatalf("Failed to marshal TextContent: %v", err)
	}

	// Test unmarshaling
	var decoded TextContent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal TextContent: %v", err)
	}

	// Verify fields
	if decoded.Type != textContent.Type {
		t.Errorf("Type mismatch. Got %s, want %s", decoded.Type, textContent.Type)
	}
	if decoded.Text != textContent.Text {
		t.Errorf("Text mismatch. Got %s, want %s", decoded.Text, textContent.Text)
	}
	if decoded.ContentType() != "text" {
		t.Errorf("ContentType mismatch. Got %s, want text", decoded.ContentType())
	}
}

func TestImageContent(t *testing.T) {
	imageContent := ImageContent{
		Type:     "image",
		Data:     "base64encodeddata",
		MimeType: "image/jpeg",
	}

	// Test marshaling
	data, err := json.Marshal(imageContent)
	if err != nil {
		t.Fatalf("Failed to marshal ImageContent: %v", err)
	}

	// Test unmarshaling
	var decoded ImageContent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal ImageContent: %v", err)
	}

	// Verify fields
	if decoded.Type != imageContent.Type {
		t.Errorf("Type mismatch. Got %s, want %s", decoded.Type, imageContent.Type)
	}
	if decoded.Data != imageContent.Data {
		t.Errorf("Data mismatch. Got %s, want %s", decoded.Data, imageContent.Data)
	}
	if decoded.MimeType != imageContent.MimeType {
		t.Errorf("MimeType mismatch. Got %s, want %s", decoded.MimeType, imageContent.MimeType)
	}
	if decoded.ContentType() != "image" {
		t.Errorf("ContentType mismatch. Got %s, want image", decoded.ContentType())
	}
}

func TestSamplingMessage(t *testing.T) {
	message := SamplingMessage{
		Role: RoleAssistant,
		Content: TextContent{
			Type: "text",
			Text: "Sample message",
		},
	}

	// Test marshaling
	data, err := json.Marshal(message)
	if err != nil {
		t.Fatalf("Failed to marshal SamplingMessage: %v", err)
	}

	// Test unmarshaling
	var decoded SamplingMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal SamplingMessage: %v", err)
	}

	// Verify fields
	if decoded.Role != message.Role {
		t.Errorf("Role mismatch. Got %s, want %s", decoded.Role, message.Role)
	}

	// Type assertion and content verification
	if textContent, ok := decoded.Content.(TextContent); !ok {
		t.Error("Failed to assert Content as TextContent")
	} else if textContent.Text != "Sample message" {
		t.Errorf("Content text mismatch. Got %s, want Sample message", textContent.Text)
	}
}

func TestPromptReference(t *testing.T) {
	ref := PromptReference{
		Type: "ref/prompt",
		Name: "test-prompt",
	}

	// Test marshaling
	data, err := json.Marshal(ref)
	if err != nil {
		t.Fatalf("Failed to marshal PromptReference: %v", err)
	}

	// Test unmarshaling
	var decoded PromptReference
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal PromptReference: %v", err)
	}

	// Verify fields
	if decoded.Type != ref.Type {
		t.Errorf("Type mismatch. Got %s, want %s", decoded.Type, ref.Type)
	}
	if decoded.Name != ref.Name {
		t.Errorf("Name mismatch. Got %s, want %s", decoded.Name, ref.Name)
	}
}

func TestResourceReference(t *testing.T) {
	ref := ResourceReference{
		Type: "ref/resource",
		URI:  "file:///path/to/resource",
	}

	// Test marshaling
	data, err := json.Marshal(ref)
	if err != nil {
		t.Fatalf("Failed to marshal ResourceReference: %v", err)
	}

	// Test unmarshaling
	var decoded ResourceReference
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal ResourceReference: %v", err)
	}

	// Verify fields
	if decoded.Type != ref.Type {
		t.Errorf("Type mismatch. Got %s, want %s", decoded.Type, ref.Type)
	}
	if decoded.URI != ref.URI {
		t.Errorf("URI mismatch. Got %s, want %s", decoded.URI, ref.URI)
	}
}
