package models

import (
	"encoding/json"

	"github.com/XiaoConstantine/mcp-go/pkg/protocol"
)

// InitializeRequest represents the initial request from client.
type InitializeRequest struct {
	Method string `json:"method"`
	Params struct {
		Capabilities    protocol.ClientCapabilities `json:"capabilities"`
		ClientInfo      Implementation              `json:"clientInfo"`
		ProtocolVersion string                      `json:"protocolVersion"`
	} `json:"params"`
}

// InitializeResult represents the server's response to initialization.
type InitializeResult struct {
	Capabilities    protocol.ServerCapabilities `json:"capabilities"`
	Instructions    string                      `json:"instructions,omitempty"`
	ProtocolVersion string                      `json:"protocolVersion"`
	ServerInfo      Implementation              `json:"serverInfo"`
}

// ListResourcesRequest represents a request to list available resources.
type ListResourcesRequest struct {
	Method string `json:"method"`
	Params struct {
		Cursor *Cursor `json:"cursor,omitempty"`
	} `json:"params"`
}

// ListResourcesResult represents the response to a resources list request.
type ListResourcesResult struct {
	Resources  []Resource `json:"resources"`
	NextCursor *Cursor    `json:"nextCursor,omitempty"`
}

// CreateMessageRequest represents a request to sample from an LLM.
type CreateMessageRequest struct {
	Method string `json:"method"`
	Params struct {
		MaxTokens        int                    `json:"maxTokens"`
		Messages         []SamplingMessage      `json:"messages"`
		SystemPrompt     string                 `json:"systemPrompt,omitempty"`
		Temperature      *float64               `json:"temperature,omitempty"`
		StopSequences    []string               `json:"stopSequences,omitempty"`
		ModelPreferences *ModelPreferences      `json:"modelPreferences,omitempty"`
		IncludeContext   string                 `json:"includeContext,omitempty"`
		Metadata         map[string]interface{} `json:"metadata,omitempty"`
	} `json:"params"`
}

// CreateMessageResult represents the response to a message creation request.
type CreateMessageResult struct {
	Content    Content `json:"content"`
	Model      string  `json:"model"`
	Role       Role    `json:"role"`
	StopReason string  `json:"stopReason,omitempty"`
}

// UnmarshalJSON implements custom JSON unmarshaling for CreateMessageResult.
func (r *CreateMessageResult) UnmarshalJSON(data []byte) error {
	var tmp struct {
		Content    json.RawMessage `json:"content"`
		Model      string          `json:"model"`
		Role       Role            `json:"role"`
		StopReason string          `json:"stopReason,omitempty"`
	}

	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}

	content, err := UnmarshalContent(tmp.Content)
	if err != nil {
		return err
	}

	r.Content = content
	r.Model = tmp.Model
	r.Role = tmp.Role
	r.StopReason = tmp.StopReason
	return nil
}

// Root represents a root directory or file the server can operate on.
type Root struct {
	URI  string `json:"uri"`
	Name string `json:"name,omitempty"`
}

// ListRootsResult represents the response to a roots list request.
type ListRootsResult struct {
	Roots []Root `json:"roots"`
}
