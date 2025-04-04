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
	reader       io.Reader    // The raw reader
	writer       *bufio.Writer
	mutex        sync.Mutex
	logger       logging.Logger
	bufferSize   int          // Initial buffer size for reading
	maxLineSize  int          // Maximum line size before giving up
}

// NewStdioTransport creates a new Transport that uses standard I/O.
func NewStdioTransport(reader io.Reader, writer io.Writer, logger logging.Logger) *StdioTransport {
	if logger == nil {
		logger = &logging.NoopLogger{}
	}
	
	// Use larger default buffer sizes to handle bigger messages
	const defaultBufferSize = 64 * 1024     // 64KB initial buffer
	const defaultMaxLineSize = 100 * 1024 * 1024 // 100MB max message size
	
	return &StdioTransport{
		reader:      reader,
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
	// Create a channel for the read operation
	msgCh := make(chan *protocol.Message, 1)
	errCh := make(chan error, 1)

	// Start a goroutine to read from stdin
	go func() {
		// Use a dynamic buffer approach instead of ReadString
		var buf bytes.Buffer
		reader := bufio.NewReaderSize(t.reader, t.bufferSize)
		
		// Flag to track if we've encountered a newline
		foundNewline := false
		totalBytesRead := 0
		
		for !foundNewline {
			// Read a chunk from the reader
			chunk, err := reader.ReadBytes('\n')
			
			if err != nil && err != io.EOF {
				errCh <- fmt.Errorf("failed to read message: %w", err)
				return
			}
			
			// Check if we've exceeded the maximum message size
			totalBytesRead += len(chunk)
			if totalBytesRead > t.maxLineSize {
				errCh <- fmt.Errorf("message too large: exceeded %d bytes", t.maxLineSize)
				return
			}
			
			// If we found a newline or reached EOF
			if len(chunk) > 0 {
				buf.Write(chunk)
				if chunk[len(chunk)-1] == '\n' {
					foundNewline = true
				}
			}
			
			// If we reached EOF, break if we have some data, otherwise report error
			if err == io.EOF {
				if buf.Len() > 0 {
					foundNewline = true // Treat EOF as end of message
				} else {
					errCh <- io.EOF
					return
				}
			}
		}
		
		// Get the complete message as a string
		line := buf.String()
		
		// Trim the trailing newline if any
		if len(line) > 0 && line[len(line)-1] == '\n' {
			line = line[:len(line)-1]
		}
		
		// Log the raw message received (truncate if very large)
		const maxLogSize = 4096 // Only log first 4KB of large messages
		logLine := line
		if len(logLine) > maxLogSize {
			logLine = logLine[:maxLogSize] + "... [truncated]"
		}
		t.logger.Debug("RECEIVED raw message: %s", logLine)

		var msg protocol.Message
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			errCh <- fmt.Errorf("failed to unmarshal message: %w", err)
			return
		}

		// Log the parsed message
		var idStr string
		if msg.ID != nil {
			idStr = fmt.Sprintf("%v", *msg.ID)
		} else {
			idStr = "<notification>"
		}
		t.logger.Debug("RECEIVED parsed message ID=%s, Method=%s", idStr, msg.Method)

		msgCh <- &msg
	}()
	
	// Wait for either the message to be read or the context to be cancelled
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case err := <-errCh:
		return nil, err
	case msg := <-msgCh:
		return msg, nil
	}
}

// Close implements Transport.Close for StdioTransport.
func (t *StdioTransport) Close() error {
	// For stdio, we don't actually close the reader/writer
	return nil
}
