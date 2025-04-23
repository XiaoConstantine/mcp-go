package transport

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/XiaoConstantine/mcp-go/pkg/logging"
	"github.com/XiaoConstantine/mcp-go/pkg/protocol"
)

// Transport represents a bidirectional communication channel for MCP messages.
type Transport interface {
	// Send sends a message to the other end
	Send(ctx context.Context, msg *protocol.Message) error

	// Receive returns the next message from the other end
	Receive(ctx context.Context) (*protocol.Message, error)

	// Close closes the transport
	Close() error
}

// StdioTransport implements Transport using standard I/O.
type StdioTransport struct {
	reader      *bufio.Reader
	writer      *bufio.Writer
	mutex       sync.Mutex
	logger      logging.Logger
	bufferSize  int // Initial buffer size for reading
	maxLineSize int // Maximum line size before giving up
}

// NewStdioTransport creates a new Transport that uses standard I/O.
func NewStdioTransport(reader io.Reader, writer io.Writer, logger logging.Logger) *StdioTransport {
	if logger == nil {
		logger = &logging.NoopLogger{}
	}

	// Use larger default buffer sizes to handle bigger messages
	const defaultBufferSize = 64 * 1024          // 64KB initial buffer
	const defaultMaxLineSize = 100 * 1024 * 1024 // 100MB max message size

	return &StdioTransport{
		reader:      bufio.NewReaderSize(reader, defaultBufferSize),
		writer:      bufio.NewWriter(writer),
		logger:      logger,
		bufferSize:  defaultBufferSize,
		maxLineSize: defaultMaxLineSize,
	}
}

// Send implements Transport.Send for StdioTransport.
func (t *StdioTransport) Send(ctx context.Context, msg *protocol.Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Log the message being sent
	var idStr string
	if msg.ID != nil {
		idStr = fmt.Sprintf("%v", *msg.ID)
	} else {
		idStr = "<notification>"
	}
	t.logger.Debug("SENDING message ID=%s, Method=%s, Content: %s", idStr, msg.Method, string(data))

	t.mutex.Lock()
	defer t.mutex.Unlock()

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		if _, err := t.writer.Write(data); err != nil {
			return fmt.Errorf("failed to write message: %w", err)
		}

		if _, err := t.writer.Write([]byte("\n")); err != nil {
			return fmt.Errorf("failed to write newline: %w", err)
		}

		if err := t.writer.Flush(); err != nil {
			return fmt.Errorf("failed to flush writer: %w", err)
		}
	}

	return nil
}

// Receive implements Transport.Receive for StdioTransport.
func (t *StdioTransport) Receive(ctx context.Context) (*protocol.Message, error) {
	var buf bytes.Buffer
	totalBytesRead := 0
	foundNewline := false

	for !foundNewline {
		// Read a chunk from the persistent reader
		chunk, err := t.reader.ReadBytes('\n')

		if len(chunk) > 0 {
			// Check size limit before appending
			if totalBytesRead+len(chunk) > t.maxLineSize {
				// Drain the rest of the oversized line to avoid leaving partial data
				// in the buffer for the next read. This is best effort.
				for err == nil && bytes.LastIndexByte(chunk, '\n') == -1 {
					chunk, err = t.reader.ReadBytes('\n')
				}
				return nil, fmt.Errorf("message too large: exceeded %d bytes limit", t.maxLineSize)
			}
			buf.Write(chunk)
			totalBytesRead += len(chunk)
			if bytes.HasSuffix(chunk, []byte("\n")) {
				foundNewline = true
			}
		}

		// Handle errors after potentially processing the chunk
		if err != nil {
			if err == io.EOF {
				// If we got EOF but read some data without a newline, treat it as a complete message.
				if buf.Len() > 0 {
					break // Exit loop, process buffer below
				} else {
					return nil, io.EOF // Genuine EOF with no data
				}
			}
			// Any other read error
			return nil, fmt.Errorf("failed to read message chunk: %w", err)
		}
	}

	// Get the complete message bytes
	lineBytes := buf.Bytes()

	// Trim the trailing newline if any
	if len(lineBytes) > 0 && lineBytes[len(lineBytes)-1] == '\n' {
		lineBytes = lineBytes[:len(lineBytes)-1]
	}

	// Log the raw message received (truncate if very large)
	const maxLogSize = 4096      // Only log first 4KB of large messages
	logLine := string(lineBytes) // Convert only for logging if needed
	if len(logLine) > maxLogSize {
		logLine = logLine[:maxLogSize] + "... [truncated]"
	}
	t.logger.Debug("RECEIVED raw message", "content", logLine)

	var msg protocol.Message
	if err := json.Unmarshal(lineBytes, &msg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal message: %w", err)
	}

	// Log the parsed message
	var idStr string
	if msg.ID != nil {
		idStr = fmt.Sprintf("%v", *msg.ID)
	} else {
		idStr = "<notification>"
	}
	t.logger.Debug("RECEIVED parsed message", "id", idStr, "method", msg.Method)

	return &msg, nil
}

// // Close implements Transport.Close for StdioTransport.
func (t *StdioTransport) Close() error {
	// For stdio, we don't actually close the reader/writer
	return nil
}
