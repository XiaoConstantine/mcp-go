package transport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/XiaoConstantine/mcp-go/pkg/logging"
	"github.com/XiaoConstantine/mcp-go/pkg/protocol"
)

// mockReader implements io.Reader for testing with controlled errors
type mockReader struct {
	data      string
	index     int
	err       error
	delay     time.Duration
	callCount int
}

func (m *mockReader) Read(p []byte) (n int, err error) {
	m.callCount++
	if m.delay > 0 {
		time.Sleep(m.delay)
	}
	if m.err != nil {
		return 0, m.err
	}
	if m.index >= len(m.data) {
		return 0, io.EOF
	}
	n = copy(p, m.data[m.index:])
	m.index += n
	return n, nil
}

// mockWriter implements io.Writer for testing with controlled errors
type mockWriter struct {
	buffer    strings.Builder
	err       error
	callCount int
}

func (m *mockWriter) Write(p []byte) (n int, err error) {
	m.callCount++
	if m.err != nil {
		return 0, m.err
	}
	return m.buffer.Write(p)
}

// TestNewStdioTransport tests the NewStdioTransport function
func TestNewStdioTransport(t *testing.T) {
	// Test with nil logger
	reader := strings.NewReader("test")
	writer := &strings.Builder{}

	transport := NewStdioTransport(reader, writer, nil)
	if transport == nil {
		t.Fatal("Expected non-nil transport")
	}

	// Test with explicit logger
	logger := logging.NewNoopLogger()
	transport = NewStdioTransport(reader, writer, logger)
	if transport == nil {
		t.Fatal("Expected non-nil transport")
	}
}

// TestStdioTransportSend tests the Send method
func TestStdioTransportSend(t *testing.T) {
	tests := []struct {
		name      string
		message   *protocol.Message
		writerErr error
		wantErr   bool
	}{
		{
			name: "send request success",
			message: &protocol.Message{
				JSONRPC: "2.0",
				ID:      getRequestID(1),
				Method:  "test",
			},
			writerErr: nil,
			wantErr:   false,
		},
		{
			name: "send notification success",
			message: &protocol.Message{
				JSONRPC: "2.0",
				Method:  "test",
			},
			writerErr: nil,
			wantErr:   false,
		},
		{
			name: "writer error",
			message: &protocol.Message{
				JSONRPC: "2.0",
				ID:      getRequestID(1),
				Method:  "test",
			},
			writerErr: errors.New("write error"),
			wantErr:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			writer := &mockWriter{err: tc.writerErr}
			transport := NewStdioTransport(strings.NewReader(""), writer, logging.NewNoopLogger())

			err := transport.Send(context.Background(), tc.message)

			if (err != nil) != tc.wantErr {
				t.Errorf("Send() error = %v, wantErr %v", err, tc.wantErr)
				return
			}

			if !tc.wantErr && writer.callCount == 0 {
				t.Error("Expected writer to be called at least once")
			}
		})
	}

	// Test context cancellation
	writer := &mockWriter{}
	transport := NewStdioTransport(strings.NewReader(""), writer, logging.NewNoopLogger())

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel before Send

	err := transport.Send(ctx, &protocol.Message{JSONRPC: "2.0", Method: "test"})
	if err == nil {
		t.Error("Expected error when context is canceled")
	}
}

// TestStdioTransportReceive tests the Receive method
func TestStdioTransportReceive(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		readerErr   error
		readerDelay time.Duration
		wantErr     bool
		wantMethod  string
	}{
		{
			name:       "receive request success",
			input:      `{"jsonrpc":"2.0","id":1,"method":"test"}` + "\n",
			readerErr:  nil,
			wantErr:    false,
			wantMethod: "test",
		},
		{
			name:       "receive notification success",
			input:      `{"jsonrpc":"2.0","method":"notify"}` + "\n",
			readerErr:  nil,
			wantErr:    false,
			wantMethod: "notify",
		},
		{
			name:      "reader error",
			input:     `{"jsonrpc":"2.0","id":1,"method":"test"}` + "\n",
			readerErr: errors.New("read error"),
			wantErr:   true,
		},
		{
			name:      "invalid json",
			input:     `{"jsonrpc":"2.0",INVALID}` + "\n",
			readerErr: nil,
			wantErr:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reader := &mockReader{data: tc.input, err: tc.readerErr, delay: tc.readerDelay}
			transport := NewStdioTransport(reader, &strings.Builder{}, logging.NewNoopLogger())

			msg, err := transport.Receive(context.Background())

			if (err != nil) != tc.wantErr {
				t.Errorf("Receive() error = %v, wantErr %v", err, tc.wantErr)
				return
			}

			if !tc.wantErr && reader.callCount == 0 {
				t.Error("Expected reader to be called at least once")
			}

			if !tc.wantErr && msg.Method != tc.wantMethod {
				t.Errorf("Expected method %s, got %s", tc.wantMethod, msg.Method)
			}
		})
	}

	// Test context cancellation
	reader := &mockReader{data: "", delay: 100 * time.Millisecond}
	transport := NewStdioTransport(reader, &strings.Builder{}, logging.NewNoopLogger())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := transport.Receive(ctx)
	if err == nil {
		t.Error("Expected error when context deadline exceeded")
	}
}

// TestStdioTransportClose tests the Close method
func TestStdioTransportClose(t *testing.T) {
	transport := NewStdioTransport(strings.NewReader(""), &strings.Builder{}, logging.NewNoopLogger())

	err := transport.Close()
	if err != nil {
		t.Errorf("Close() error = %v, expected nil", err)
	}
}

// TestFullMessageCycle tests sending and receiving a complete message cycle
func TestFullMessageCycle(t *testing.T) {
	// Create a pipe for bidirectional communication
	reader, writer := io.Pipe()

	// Create two transports that communicate with each other
	serverTransport := NewStdioTransport(reader, writer, logging.NewNoopLogger())
	clientTransport := NewStdioTransport(reader, writer, logging.NewNoopLogger())

	// Message to send
	reqID := protocol.RequestID(123)
	requestMsg := &protocol.Message{
		JSONRPC: "2.0",
		ID:      &reqID,
		Method:  "testMethod",
		Params:  json.RawMessage(`{"key":"value"}`),
	}

	// Use channels for synchronization
	done := make(chan struct{})

	// Start receiving in a goroutine
	go func() {
		defer close(done)

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		receivedMsg, err := clientTransport.Receive(ctx)
		if err != nil {
			t.Errorf("Receive error: %v", err)
			return
		}

		// Verify the received message matches what was sent
		if receivedMsg.JSONRPC != requestMsg.JSONRPC {
			t.Errorf("Expected JSONRPC %s, got %s", requestMsg.JSONRPC, receivedMsg.JSONRPC)
		}
		if fmt.Sprintf("%v", *receivedMsg.ID) != fmt.Sprintf("%v", *requestMsg.ID) {
			t.Errorf("Expected ID %v (type: %T), got %v (type: %T)",
				*requestMsg.ID, *requestMsg.ID,
				*receivedMsg.ID, *receivedMsg.ID)
		}
		if receivedMsg.Method != requestMsg.Method {
			t.Errorf("Expected Method %s, got %s", requestMsg.Method, receivedMsg.Method)
		}
	}()

	// Wait a bit to ensure receiver is ready
	time.Sleep(10 * time.Millisecond)

	// Send the message
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	if err := serverTransport.Send(ctx, requestMsg); err != nil {
		t.Fatalf("Send error: %v", err)
	}

	// Wait for receive to complete
	select {
	case <-done:
		// Success
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Timed out waiting for message to be received")
	}
}

// Helper function to create a protocol.RequestID
func getRequestID(id int) *protocol.RequestID {
	reqID := protocol.RequestID(id)
	return &reqID
}
