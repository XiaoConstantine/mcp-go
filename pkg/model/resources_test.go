package models

import (
	"encoding/json"
	"testing"
)

func TestTextResourceContents_isResourceContent(t *testing.T) {
	content := TextResourceContents{
		ResourceContents: ResourceContents{
			URI:      "file://test.txt",
			MimeType: "text/plain",
		},
		Text: "test content",
	}
	
	// This should not panic - interface method is implemented
	content.isResourceContent()
	
	// Also test via interface to ensure method is callable
	var resourceContent ResourceContent = content
	resourceContent.isResourceContent()
}

func TestBlobResourceContents_isResourceContent(t *testing.T) {
	content := BlobResourceContents{
		ResourceContents: ResourceContents{
			URI:      "file://test.bin",
			MimeType: "application/octet-stream",
		},
		Blob: "dGVzdCBkYXRh", // base64 encoded "test data"
	}
	
	// This should not panic - interface method is implemented
	content.isResourceContent()
	
	// Also test via interface to ensure method is callable
	var resourceContent ResourceContent = content
	resourceContent.isResourceContent()
}

func TestReadResourceResult_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantErr bool
		check   func(t *testing.T, result *ReadResourceResult)
	}{
		{
			name: "text content",
			json: `{"contents": [{"uri": "file://test.txt", "mimeType": "text/plain", "text": "hello world"}]}`,
			wantErr: false,
			check: func(t *testing.T, result *ReadResourceResult) {
				if len(result.Contents) != 1 {
					t.Errorf("Expected 1 content, got %d", len(result.Contents))
					return
				}
				textContent, ok := result.Contents[0].(*TextResourceContents)
				if !ok {
					t.Errorf("Expected TextResourceContents, got %T", result.Contents[0])
					return
				}
				if textContent.URI != "file://test.txt" {
					t.Errorf("Expected URI 'file://test.txt', got '%s'", textContent.URI)
				}
				if textContent.MimeType != "text/plain" {
					t.Errorf("Expected MimeType 'text/plain', got '%s'", textContent.MimeType)
				}
				if textContent.Text != "hello world" {
					t.Errorf("Expected text 'hello world', got '%s'", textContent.Text)
				}
			},
		},
		{
			name: "blob content",
			json: `{"contents": [{"uri": "file://test.bin", "mimeType": "application/octet-stream", "blob": "dGVzdA=="}]}`,
			wantErr: false,
			check: func(t *testing.T, result *ReadResourceResult) {
				if len(result.Contents) != 1 {
					t.Errorf("Expected 1 content, got %d", len(result.Contents))
					return
				}
				blobContent, ok := result.Contents[0].(*BlobResourceContents)
				if !ok {
					t.Errorf("Expected BlobResourceContents, got %T", result.Contents[0])
					return
				}
				if blobContent.URI != "file://test.bin" {
					t.Errorf("Expected URI 'file://test.bin', got '%s'", blobContent.URI)
				}
				if blobContent.MimeType != "application/octet-stream" {
					t.Errorf("Expected MimeType 'application/octet-stream', got '%s'", blobContent.MimeType)
				}
				if blobContent.Blob != "dGVzdA==" {
					t.Errorf("Expected blob 'dGVzdA==', got '%s'", blobContent.Blob)
				}
			},
		},
		{
			name: "multiple contents",
			json: `{"contents": [
				{"uri": "file://test.txt", "text": "hello"},
				{"uri": "file://test.bin", "blob": "dGVzdA=="}
			]}`,
			wantErr: false,
			check: func(t *testing.T, result *ReadResourceResult) {
				if len(result.Contents) != 2 {
					t.Errorf("Expected 2 contents, got %d", len(result.Contents))
					return
				}
				
				// Check first content (text)
				textContent, ok := result.Contents[0].(*TextResourceContents)
				if !ok {
					t.Errorf("Expected first content to be TextResourceContents, got %T", result.Contents[0])
				} else if textContent.Text != "hello" {
					t.Errorf("Expected first content text 'hello', got '%s'", textContent.Text)
				}
				
				// Check second content (blob)
				blobContent, ok := result.Contents[1].(*BlobResourceContents)
				if !ok {
					t.Errorf("Expected second content to be BlobResourceContents, got %T", result.Contents[1])
				} else if blobContent.Blob != "dGVzdA==" {
					t.Errorf("Expected second content blob 'dGVzdA==', got '%s'", blobContent.Blob)
				}
			},
		},
		{
			name: "content without text or blob",
			json: `{"contents": [{"uri": "file://test.txt", "mimeType": "text/plain"}]}`,
			wantErr: true,
			check: nil,
		},
		{
			name: "invalid JSON",
			json: `{"contents": [invalid json]}`,
			wantErr: true,
			check: nil,
		},
		{
			name: "malformed content structure",
			json: `{"contents": [{"invalid": "structure"}]}`,
			wantErr: true,
			check: nil,
		},
		{
			name: "empty contents array",
			json: `{"contents": []}`,
			wantErr: false,
			check: func(t *testing.T, result *ReadResourceResult) {
				if len(result.Contents) != 0 {
					t.Errorf("Expected 0 contents, got %d", len(result.Contents))
				}
			},
		},
		{
			name: "text content with empty text",
			json: `{"contents": [{"uri": "file://test.txt", "text": ""}]}`,
			wantErr: true,
			check: nil,
		},
		{
			name: "blob content with empty blob",
			json: `{"contents": [{"uri": "file://test.bin", "blob": ""}]}`,
			wantErr: true,
			check: nil,
		},
		{
			name: "content with both text and blob",
			json: `{"contents": [{"uri": "file://test", "text": "hello", "blob": "dGVzdA=="}]}`,
			wantErr: false,
			check: func(t *testing.T, result *ReadResourceResult) {
				// Should prefer text when both are present
				if len(result.Contents) != 1 {
					t.Errorf("Expected 1 content, got %d", len(result.Contents))
					return
				}
				textContent, ok := result.Contents[0].(*TextResourceContents)
				if !ok {
					t.Errorf("Expected TextResourceContents when both text and blob present, got %T", result.Contents[0])
					return
				}
				if textContent.Text != "hello" {
					t.Errorf("Expected text 'hello', got '%s'", textContent.Text)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result ReadResourceResult
			err := json.Unmarshal([]byte(tt.json), &result)
			
			if (err != nil) != tt.wantErr {
				t.Errorf("ReadResourceResult.UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			
			if !tt.wantErr && tt.check != nil {
				tt.check(t, &result)
			}
		})
	}
}

func TestResource(t *testing.T) {
	resource := Resource{
		BaseAnnotated: BaseAnnotated{},
		Name:         "test-resource",
		URI:          "file://test.txt",
		Description:  "Test resource",
		MimeType:     "text/plain",
	}

	// Test JSON marshaling
	data, err := json.Marshal(resource)
	if err != nil {
		t.Fatalf("Failed to marshal Resource: %v", err)
	}

	// Test JSON unmarshaling
	var decoded Resource
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal Resource: %v", err)
	}

	// Verify fields
	if decoded.Name != "test-resource" {
		t.Errorf("Wrong name: got %s, want test-resource", decoded.Name)
	}
	if decoded.URI != "file://test.txt" {
		t.Errorf("Wrong URI: got %s, want file://test.txt", decoded.URI)
	}
}

func TestResourceTemplate(t *testing.T) {
	template := ResourceTemplate{
		BaseAnnotated: BaseAnnotated{},
		Name:         "test-template",
		URITemplate:  "file://test-{id}.txt",
		Description:  "Test template",
		MimeType:     "text/plain",
	}

	// Test JSON marshaling
	data, err := json.Marshal(template)
	if err != nil {
		t.Fatalf("Failed to marshal ResourceTemplate: %v", err)
	}

	// Test JSON unmarshaling
	var decoded ResourceTemplate
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal ResourceTemplate: %v", err)
	}

	// Verify fields
	if decoded.Name != "test-template" {
		t.Errorf("Wrong name: got %s, want test-template", decoded.Name)
	}
	if decoded.URITemplate != "file://test-{id}.txt" {
		t.Errorf("Wrong URI template: got %s, want file://test-{id}.txt", decoded.URITemplate)
	}
}

func TestEmbeddedResource(t *testing.T) {
	textContent := &TextResourceContents{
		ResourceContents: ResourceContents{
			URI:      "file://test.txt",
			MimeType: "text/plain",
		},
		Text: "embedded content",
	}

	embedded := EmbeddedResource{
		BaseAnnotated: BaseAnnotated{},
		Type:         "resource",
		Resource:     textContent,
	}

	// Test JSON marshaling
	data, err := json.Marshal(embedded)
	if err != nil {
		t.Fatalf("Failed to marshal EmbeddedResource: %v", err)
	}

	// Test that it contains expected structure
	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal to map: %v", err)
	}

	if decoded["type"] != "resource" {
		t.Errorf("Wrong type: got %v, want resource", decoded["type"])
	}
}

func TestResourceContents(t *testing.T) {
	// Test TextResourceContents
	textContent := TextResourceContents{
		ResourceContents: ResourceContents{
			URI:      "file://test.txt",
			MimeType: "text/plain",
		},
		Text: "test content",
	}

	data, err := json.Marshal(textContent)
	if err != nil {
		t.Fatalf("Failed to marshal TextResourceContents: %v", err)
	}

	var decodedText TextResourceContents
	if err := json.Unmarshal(data, &decodedText); err != nil {
		t.Fatalf("Failed to unmarshal TextResourceContents: %v", err)
	}

	if decodedText.Text != "test content" {
		t.Errorf("Wrong text: got %s, want 'test content'", decodedText.Text)
	}

	// Test BlobResourceContents
	blobContent := BlobResourceContents{
		ResourceContents: ResourceContents{
			URI:      "file://test.bin",
			MimeType: "application/octet-stream",
		},
		Blob: "dGVzdA==",
	}

	data, err = json.Marshal(blobContent)
	if err != nil {
		t.Fatalf("Failed to marshal BlobResourceContents: %v", err)
	}

	var decodedBlob BlobResourceContents
	if err := json.Unmarshal(data, &decodedBlob); err != nil {
		t.Fatalf("Failed to unmarshal BlobResourceContents: %v", err)
	}

	if decodedBlob.Blob != "dGVzdA==" {
		t.Errorf("Wrong blob: got %s, want 'dGVzdA=='", decodedBlob.Blob)
	}
}