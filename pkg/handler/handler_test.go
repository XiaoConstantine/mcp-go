package handler

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	mcperrors "github.com/XiaoConstantine/mcp-go/pkg/errors"
	"github.com/XiaoConstantine/mcp-go/pkg/logging"
	"github.com/XiaoConstantine/mcp-go/pkg/protocol"
)

// MockHandler is a mock message handler for testing.
type MockHandler struct {
	HandleMessageFunc func(ctx context.Context, msg *protocol.Message) (*protocol.Message, error)
}

func (m *MockHandler) HandleMessage(ctx context.Context, msg *protocol.Message) (*protocol.Message, error) {
	if m.HandleMessageFunc != nil {
		return m.HandleMessageFunc(ctx, msg)
	}
	return nil, nil
}

func TestMessageRouter(t *testing.T) {
	logger := logging.NewNoopLogger()

	// Create a new message router
	router := NewMessageRouter(logger)
	if router == nil {
		t.Fatal("Expected non-nil router")
	}

	// Test registering and handling a message
	testMethod := "test/method"
	testID := protocol.RequestID(int64(123))

	expectedResponse := &protocol.Message{JSONRPC: "2.0", ID: &testID, Result: []byte(`{"result":"ok"}`)}

	mockHandler := &MockHandler{
		HandleMessageFunc: func(ctx context.Context, msg *protocol.Message) (*protocol.Message, error) {
			if msg.Method != testMethod {
				t.Errorf("Expected method %s, got %s", testMethod, msg.Method)
			}
			return expectedResponse, nil
		},
	}

	// Register the handler
	router.RegisterHandler(testMethod, mockHandler)

	// Test handling a message with the registered method
	msg := &protocol.Message{JSONRPC: "2.0", ID: &testID, Method: testMethod}
	response, err := router.HandleMessage(context.Background(), msg)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if response != expectedResponse {
		t.Errorf("Expected response %v, got %v", expectedResponse, response)
	}

	// Test handling a message with an unregistered method
	unknownMsg := &protocol.Message{JSONRPC: "2.0", ID: &testID, Method: "unknown/method"}
	response, err = router.HandleMessage(context.Background(), unknownMsg)
	if err == nil {
		t.Error("Expected error for unregistered method, got nil")
	}
	if response != nil {
		t.Errorf("Expected nil response for unregistered method, got %v", response)
	}

	// Test handling a response message (should be ignored)
	responseMsg := &protocol.Message{JSONRPC: "2.0", ID: &testID, Result: []byte(`{"result":"ok"}`)}
	response, err = router.HandleMessage(context.Background(), responseMsg)
	if err != nil {
		t.Errorf("Expected no error for response message, got %v", err)
	}
	if response != nil {
		t.Errorf("Expected nil response for response message, got %v", response)
	}
}

func TestRequestTracker(t *testing.T) {
	logger := logging.NewNoopLogger()
	tracker := NewRequestTracker(logger)

	if tracker == nil {
		t.Fatal("Expected non-nil tracker")
	}

	// Test NextID
	id1 := tracker.NextID()
	id2 := tracker.NextID()

	// Since RequestID may be an interface, avoid direct comparison
	// Instead, convert to string and compare, or just ensure they're different
	if id1 == id2 {
		t.Errorf("Expected second ID to be different from first ID, got %v and %v", id1, id2)
	}

	// Test TrackRequest and IsTracked
	method := "test/method"
	reqCtx := tracker.TrackRequest(id1, method)

	if !tracker.IsTracked(id1) {
		t.Errorf("Expected request %v to be tracked", id1)
	}

	// Test UntrackRequest
	tracker.UntrackRequest(id1)
	if tracker.IsTracked(id1) {
		t.Errorf("Expected request %v to be untracked after UntrackRequest", id1)
	}

	// Test HandleResponse with error
	id3 := tracker.NextID()
	reqCtx = tracker.TrackRequest(id3, method)

	// Create an error response
	errCode := 123
	errMsg := "test error"
	errResponse := &protocol.Message{
		JSONRPC: "2.0",
		ID:      &id3,
		Error: &protocol.ErrorObject{
			Code:    errCode,
			Message: errMsg,
		},
	}

	// Start a goroutine to wait for the error
	var err error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err = tracker.WaitForResponse(context.Background(), reqCtx)
	}()

	// Handle the error response
	handled := tracker.HandleResponse(errResponse)
	if !handled {
		t.Error("Expected HandleResponse to return true")
	}

	// Wait for the goroutine to complete
	wg.Wait()

	// Check that we got the expected error
	if err == nil {
		t.Fatal("Expected error from WaitForResponse, got nil")
	}
	protocolErr, ok := err.(*mcperrors.ProtocolError)
	if !ok {
		t.Fatalf("Expected *mcperrors.ProtocolError, got %T", err)
	}
	if protocolErr.Code != errCode {
		t.Errorf("Expected error code %d, got %d", errCode, protocolErr.Code)
	}
	if protocolErr.Message != errMsg {
		t.Errorf("Expected error message %q, got %q", errMsg, protocolErr.Message)
	}

	// Test HandleResponse with successful response
	id4 := tracker.NextID()
	reqCtx = tracker.TrackRequest(id4, method)
	successResponse := &protocol.Message{
		JSONRPC: "2.0",
		ID:      &id4,
		Result:  []byte(`{"result":"ok"}`),
	}

	// Start a goroutine to wait for the response
	var resp *protocol.Message
	wg.Add(1)
	go func() {
		defer wg.Done()
		resp, err = tracker.WaitForResponse(context.Background(), reqCtx)
	}()

	// Handle the success response
	handled = tracker.HandleResponse(successResponse)
	if !handled {
		t.Error("Expected HandleResponse to return true")
	}

	// Wait for the goroutine to complete
	wg.Wait()

	// Check that we got the expected response
	if err != nil {
		t.Errorf("Expected no error from WaitForResponse, got %v", err)
	}
	if resp != successResponse {
		t.Errorf("Expected response %v, got %v", successResponse, resp)
	}

	// Test timeout handling
	id5 := tracker.NextID()
	reqCtx = tracker.TrackRequest(id5, method)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err = tracker.WaitForResponse(ctx, reqCtx)
	if err == nil {
		t.Fatal("Expected timeout error, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) && !mcperrors.IsTimeout(err) {
		t.Errorf("Expected timeout error, got %v", err)
	}
}

func TestHandleResponseWithUnknownID(t *testing.T) {
	logger := logging.NewNoopLogger()
	tracker := NewRequestTracker(logger)

	// Create a response with an unknown ID
	unknownID := protocol.RequestID(int64(999))
	response := &protocol.Message{
		JSONRPC: "2.0",
		ID:      &unknownID,
		Result:  []byte(`{"result":"ok"}`),
	}

	// Handle the response
	handled := tracker.HandleResponse(response)
	if handled {
		t.Error("Expected HandleResponse to return false for unknown ID")
	}
}

func TestTypeConversion(t *testing.T) {
	logger := logging.NewNoopLogger()
	tracker := NewRequestTracker(logger)

	// Track a request with int64 ID
	intID := int64(123)
	tracker.TrackRequest(intID, "test/method")

	// Check if it can be found with a float64 ID (JSON unmarshaling often converts numbers to float64)
	floatID := float64(123)
	reqVal := protocol.RequestID(floatID)
	if !tracker.IsTracked(floatID) {
		t.Errorf("Expected request with ID float64(%v) to be tracked when original ID was int64(%v)", floatID, intID)
	}

	// Create a response with float64 ID
	response := &protocol.Message{
		JSONRPC: "2.0",
		ID:      &reqVal,
		Result:  []byte(`{"result":"ok"}`),
	}

	// Handle the response
	handled := tracker.HandleResponse(response)
	if !handled {
		t.Error("Expected HandleResponse to return true for float64 ID matching int64 request ID")
	}
}
