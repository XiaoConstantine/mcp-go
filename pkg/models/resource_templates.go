package models

// ListResourceTemplatesRequest is sent from the client to request available resource templates.
// It supports pagination through an optional cursor.
type ListResourceTemplatesRequest struct {
	Method string                    `json:"method"`
	Params *ListResourceTemplatesParams `json:"params,omitempty"`
}

// ListResourceTemplatesParams contains the parameters for the list templates request
type ListResourceTemplatesParams struct {
	Cursor *Cursor `json:"cursor,omitempty"`
}

// ListResourceTemplatesResult is the server's response containing available resource templates.
// It includes pagination support through nextCursor.
type ListResourceTemplatesResult struct {
	ResourceTemplates []ResourceTemplate `json:"resourceTemplates"`
	NextCursor       *Cursor            `json:"nextCursor,omitempty"`
}
