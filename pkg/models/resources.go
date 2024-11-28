package models

import (
    "encoding/json"
    "fmt"
)

// Resource represents a known resource that the server can read
type Resource struct {
    Annotated
    Name        string `json:"name"`
    URI         string `json:"uri"`
    Description string `json:"description,omitempty"`
    MimeType    string `json:"mimeType,omitempty"`
}

// ResourceContents represents the base contents of a specific resource
type ResourceContents struct {
    URI      string `json:"uri"`
    MimeType string `json:"mimeType,omitempty"`
}

// TextResourceContents represents text-based resource contents
type TextResourceContents struct {
    ResourceContents
    Text string `json:"text"`
}

// BlobResourceContents represents binary resource contents
type BlobResourceContents struct {
    ResourceContents
    Blob string `json:"blob"`
}

// ResourceTemplate represents a template for resources available on the server
type ResourceTemplate struct {
    Annotated
    Name        string `json:"name"`
    URITemplate string `json:"uriTemplate"`
    Description string `json:"description,omitempty"`
    MimeType    string `json:"mimeType,omitempty"`
}

// EmbeddedResource represents resource contents embedded in a prompt or tool call
type EmbeddedResource struct {
    Annotated
    Type     string          `json:"type"`
    Resource ResourceContent `json:"resource"`
}

// ResourceContent represents either text or blob resource content
type ResourceContent interface {
    isResourceContent()
}

func (TextResourceContents) isResourceContent() {}
func (BlobResourceContents) isResourceContent() {}

// ReadResourceResult represents the response to a resource read request
type ReadResourceResult struct {
    Contents []ResourceContent `json:"contents"`
}

// UnmarshalJSON implements custom JSON unmarshaling for ReadResourceResult
func (r *ReadResourceResult) UnmarshalJSON(data []byte) error {
    // First, unmarshal into a temporary structure with raw message for contents
    var raw struct {
        Contents []json.RawMessage `json:"contents"`
    }
    if err := json.Unmarshal(data, &raw); err != nil {
        return err
    }

    // Initialize the contents slice with the correct length
    r.Contents = make([]ResourceContent, len(raw.Contents))

    // Process each content item
    for i, contentData := range raw.Contents {
        // Unmarshal into a temporary structure to check the content type
        var base struct {
            URI      string `json:"uri"`
            MimeType string `json:"mimeType,omitempty"`
            Text     string `json:"text,omitempty"`
            Blob     string `json:"blob,omitempty"`
        }
        if err := json.Unmarshal(contentData, &base); err != nil {
            return fmt.Errorf("failed to unmarshal resource content: %w", err)
        }

        // Determine the type based on which fields are present
        if base.Text != "" {
            r.Contents[i] = &TextResourceContents{
                ResourceContents: ResourceContents{
                    URI:      base.URI,
                    MimeType: base.MimeType,
                },
                Text: base.Text,
            }
        } else if base.Blob != "" {
            r.Contents[i] = &BlobResourceContents{
                ResourceContents: ResourceContents{
                    URI:      base.URI,
                    MimeType: base.MimeType,
                },
                Blob: base.Blob,
            }
        } else {
            return fmt.Errorf("resource content must contain either text or blob data")
        }
    }
    return nil
}