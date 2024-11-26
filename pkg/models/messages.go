package models

import (
	"encoding/json"
	"fmt"
)

// Content represents the base interface for all content types
type Content interface {
	ContentType() string
}

// TextContent represents text provided to or from an LLM
type TextContent struct {
	Annotated
	Type string `json:"type"`
	Text string `json:"text"`
}

func (t TextContent) ContentType() string {
	return "text"
}

// ImageContent represents an image provided to or from an LLM
type ImageContent struct {
	Annotated
	Type     string `json:"type"`
	Data     string `json:"data"`
	MimeType string `json:"mimeType"`
}

func (i ImageContent) ContentType() string {
	return "image"
}

// contentWrapper is a helper struct for JSON marshaling/unmarshaling
type contentWrapper struct {
	Type     string          `json:"type"`
	Content  json.RawMessage `json:"content"`
}

// MarshalJSON implements custom JSON marshaling for Content interface
func MarshalContent(c Content) ([]byte, error) {
	return json.Marshal(c)
}

// UnmarshalContent implements custom JSON unmarshaling for Content interface
func UnmarshalContent(data []byte) (Content, error) {
	var wrapper map[string]interface{}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, err
	}

	contentType, ok := wrapper["type"].(string)
	if !ok {
		return nil, fmt.Errorf("content type not found or invalid")
	}

	switch contentType {
	case "text":
		var content TextContent
		if err := json.Unmarshal(data, &content); err != nil {
			return nil, err
		}
		return content, nil
	case "image":
		var content ImageContent
		if err := json.Unmarshal(data, &content); err != nil {
			return nil, err
		}
		return content, nil
	default:
		return nil, fmt.Errorf("unknown content type: %s", contentType)
	}
}

// SamplingMessage describes a message issued to or received from an LLM API
type SamplingMessage struct {
	Role    Role    `json:"role"`
	Content Content `json:"content"`
}

// UnmarshalJSON implements custom JSON unmarshaling for SamplingMessage
func (m *SamplingMessage) UnmarshalJSON(data []byte) error {
	var raw struct {
		Role    Role            `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	m.Role = raw.Role
	content, err := UnmarshalContent(raw.Content)
	if err != nil {
		return err
	}
	m.Content = content
	return nil
}

// PromptMessage describes a message returned as part of a prompt
type PromptMessage struct {
	Role    Role    `json:"role"`
	Content Content `json:"content"`
}

// UnmarshalJSON implements custom JSON unmarshaling for PromptMessage
func (m *PromptMessage) UnmarshalJSON(data []byte) error {
	var raw struct {
		Role    Role            `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	m.Role = raw.Role
	content, err := UnmarshalContent(raw.Content)
	if err != nil {
		return err
	}
	m.Content = content
	return nil
}

// PromptReference identifies a prompt
type PromptReference struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

// ResourceReference is a reference to a resource or resource template
type ResourceReference struct {
	Type string `json:"type"`
	URI  string `json:"uri"`
}
