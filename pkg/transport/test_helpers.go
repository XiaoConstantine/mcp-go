package transport

import "github.com/XiaoConstantine/mcp-go/pkg/protocol"

// Helper function to create RequestID pointer for tests.
func reqID(id interface{}) *protocol.RequestID {
	requestID := protocol.RequestID(id)
	return &requestID
}
