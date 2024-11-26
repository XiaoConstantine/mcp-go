package models

// Prompt represents a prompt or prompt template that the server offers.
// It provides information about what arguments the prompt accepts and how it should be used.
type Prompt struct {
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Arguments   []PromptArgument `json:"arguments,omitempty"`
}

// PromptArgument describes an argument that a prompt can accept.
// Arguments can be either required or optional, and may include a description
// to help users understand their purpose.
type PromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// ListPromptsRequest is sent from the client to request available prompts.
// It supports pagination through an optional cursor.
type ListPromptsRequest struct {
	Method string `json:"method"`
	Params struct {
		Cursor *Cursor `json:"cursor,omitempty"`
	} `json:"params"`
}

// ListPromptsResult is the server's response containing available prompts.
// It includes pagination support through nextCursor.
type ListPromptsResult struct {
	Prompts     []Prompt `json:"prompts"`
	NextCursor  *Cursor  `json:"nextCursor,omitempty"`
}

// GetPromptRequest is used to retrieve a specific prompt by name.
// It can include optional arguments for template processing.
type GetPromptRequest struct {
	Method string `json:"method"`
	Params struct {
		Name      string            `json:"name"`
		Arguments map[string]string `json:"arguments,omitempty"`
	} `json:"params"`
}

// GetPromptResult contains the requested prompt's messages and metadata.
type GetPromptResult struct {
	Messages    []PromptMessage `json:"messages"`
	Description string          `json:"description,omitempty"`
}
