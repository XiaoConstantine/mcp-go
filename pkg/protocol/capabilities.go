// Package protocol provides the core protocol model definitions for MCP.
package protocol

// ClientCapabilities represents the set of features that a client supports.
// These capabilities are declared during the initialization phase of the connection.
type ClientCapabilities struct {
	// Experimental contains non-standard capabilities that are still in development.
	// The outer map key is the experimental feature name, and the inner map contains
	// feature-specific configuration.
	Experimental map[string]map[string]interface{} `json:"experimental,omitempty"`

	// Roots indicates whether the client supports root-related operations.
	// This is essential for operations involving file system access.
	Roots *RootsCapability `json:"roots,omitempty"`

	// Sampling indicates whether the client supports LLM sampling operations.
	// The map contains sampling-specific configuration options.
	Sampling map[string]interface{} `json:"sampling,omitempty"`
}

// RootsCapability defines the client's support for root-related operations.
type RootsCapability struct {
	// ListChanged indicates whether the client can handle notifications about
	// changes to the list of available roots.
	ListChanged bool `json:"listChanged"`
}

// ServerCapabilities represents the set of features that a server supports.
// These capabilities are declared in response to the client's initialization request.
type ServerCapabilities struct {
	// Experimental contains non-standard capabilities that are still in development.
	// The outer map key is the experimental feature name, and the inner map contains
	// feature-specific configuration.
	Experimental map[string]map[string]interface{} `json:"experimental,omitempty"`

	// Logging indicates whether the server supports sending log messages to the client.
	// The map contains logging-specific configuration options.
	Logging map[string]interface{} `json:"logging,omitempty"`

	// Prompts indicates whether the server offers prompt templates and related features.
	Prompts *PromptsCapability `json:"prompts,omitempty"`

	// Resources indicates whether the server offers resource access capabilities.
	Resources *ResourcesCapability `json:"resources,omitempty"`

	// Tools indicates whether the server offers tool invocation capabilities.
	Tools *ToolsCapability `json:"tools,omitempty"`
}

// PromptsCapability defines the server's support for prompt-related operations.
type PromptsCapability struct {
	// ListChanged indicates whether the server supports notifications about
	// changes to the list of available prompts.
	ListChanged bool `json:"listChanged"`
}

// ResourcesCapability defines the server's support for resource-related operations.
type ResourcesCapability struct {
	// ListChanged indicates whether the server supports notifications about
	// changes to the list of available resources.
	ListChanged bool `json:"listChanged"`

	// Subscribe indicates whether the server supports subscriptions to
	// resource update notifications.
	Subscribe bool `json:"subscribe"`
}

// ToolsCapability defines the server's support for tool-related operations.
type ToolsCapability struct {
	// ListChanged indicates whether the server supports notifications about
	// changes to the list of available tools.
	ListChanged bool `json:"listChanged"`
}