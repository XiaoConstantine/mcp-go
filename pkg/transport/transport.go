package transport

import (
	"bufio"
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
	reader *bufio.Reader
	writer *bufio.Writer
	mutex  sync.Mutex
	logger logging.Logger
}

// NewStdioTransport creates a new Transport that uses standard I/O.
func NewStdioTransport(reader io.Reader, writer io.Writer, logger logging.Logger) *StdioTransport {
	if logger == nil {
		logger = &logging.NoopLogger{}
	}
	return &StdioTransport{
		reader: bufio.NewReader(reader),
		writer: bufio.NewWriter(writer),
		logger: logger,
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
		// Use ReadString to read a full line up to the newline
		line, err := t.reader.ReadString('\n')
		if err != nil {
			errCh <- fmt.Errorf("failed to read message: %w", err)
			return
		}

		// Log the raw message received
		t.logger.Debug("RECEIVED raw message: %s", line)

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
