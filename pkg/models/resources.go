package models

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
