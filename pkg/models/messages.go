package models

import (
	"encoding/json"
	"fmt"
)

// Content represents the base interface for all content types
type Content interface {
	ContentType() string
}

// contentWrapper is a helper struct for JSON marshaling/unmarshaling
type contentWrapper struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// UnmarshalJSON implements custom JSON unmarshaling for Content interface
func UnmarshalContent(data []byte) (Content, error) {
	var wrapper contentWrapper
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, err
	}

	var content Content
	switch wrapper.Type {
	case "text":
		var c TextContent
		if err := json.Unmarshal(data, &c); err != nil {
			return nil, err
		}
		content = c
	case "image":
		var c ImageContent
		if err := json.Unmarshal(data, &c); err != nil {
			return nil, err
		}
		content = c
	default:
		return nil, fmt.Errorf("unknown content type: %s", wrapper.Type)
	}

	return content, nil
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

// SamplingMessage describes a message issued to or received from an LLM API
type SamplingMessage struct {
	Role    Role    `json:"role"`
	Content Content `json:"content"`
}

// UnmarshalJSON implements custom JSON unmarshaling for SamplingMessage
func (m *SamplingMessage) UnmarshalJSON(data []byte) error {
	type Alias SamplingMessage
	var tmp struct {
		Role    Role            `json:"role"`
		Content json.RawMessage `json:"content"`
	}

	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}

	content, err := UnmarshalContent(tmp.Content)
	if err != nil {
		return err
	}

	m.Role = tmp.Role
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
	type Alias PromptMessage
	var tmp struct {
		Role    Role            `json:"role"`
		Content json.RawMessage `json:"content"`
	}

	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}

	content, err := UnmarshalContent(tmp.Content)
	if err != nil {
		return err
	}

	m.Role = tmp.Role
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