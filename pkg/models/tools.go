package models

import "encoding/json"

// Tool defines a tool that the client can call.
type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	InputSchema InputSchema `json:"inputSchema"`
}

// InputSchema defines the expected parameters for a tool using JSON Schema.
type InputSchema struct {
	Type       string                      `json:"type"`
	Properties map[string]ParameterSchema  `json:"properties"`
}

// ParameterSchema defines the schema for a single parameter.
type ParameterSchema struct {
	Type        string      `json:"type"`
	Description string      `json:"description,omitempty"`
	Required    bool        `json:"required,omitempty"`
	Minimum     *float64    `json:"minimum,omitempty"`
	Maximum     *float64    `json:"maximum,omitempty"`
}

// ListToolsRequest is sent from the client to request available tools.
type ListToolsRequest struct {
	Method string           `json:"method"`
	Params *ListToolsParams `json:"params,omitempty"`
}

// ListToolsParams contains the parameters for the list tools request.
type ListToolsParams struct {
	Cursor *Cursor `json:"cursor,omitempty"`
}

// ListToolsResult is the server's response containing available tools.
type ListToolsResult struct {
	Tools      []Tool  `json:"tools"`
	NextCursor *Cursor `json:"nextCursor,omitempty"`
}

// CallToolRequest is used by the client to invoke a tool.
type CallToolRequest struct {
	Method string          `json:"method"`
	Params CallToolParams  `json:"params"`
}

// CallToolParams contains the parameters for the tool call.
type CallToolParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

// CallToolResult is the server's response to a tool call.
type CallToolResult struct {
	Content []Content `json:"content"`
	IsError bool     `json:"isError,omitempty"`
}

// UnmarshalJSON implements custom JSON unmarshaling for CallToolResult.
func (r *CallToolResult) UnmarshalJSON(data []byte) error {
	var raw struct {
		Content []json.RawMessage `json:"content"`
		IsError bool             `json:"isError,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	r.IsError = raw.IsError
	r.Content = make([]Content, len(raw.Content))
	
	for i, contentData := range raw.Content {
		content, err := UnmarshalContent(contentData)
		if err != nil {
			return err
		}
		r.Content[i] = content
	}
	
	return nil
}
