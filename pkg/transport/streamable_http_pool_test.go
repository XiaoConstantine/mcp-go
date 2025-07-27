package transport

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/XiaoConstantine/mcp-go/pkg/protocol"
)

func TestNewMessagePool(t *testing.T) {
	pool := NewMessagePool()

	if pool == nil {
		t.Error("NewMessagePool() returned nil")
	}
}

func TestMessagePool_GetMessage(t *testing.T) {
	pool := NewMessagePool()

	msg1 := pool.GetMessage()
	if msg1 == nil {
		t.Fatal("GetMessage() returned nil")
	}

	if msg1.Message == nil {
		t.Error("PooledMessage.Message is nil")
	}

	if msg1.Buffer == nil {
		t.Error("PooledMessage.Buffer is nil")
	}

	// Get another message to test pooling
	msg2 := pool.GetMessage()
	if msg2 == nil {
		t.Error("Second GetMessage() returned nil")
	}

	// Messages should be different objects
	if msg1 == msg2 {
		t.Error("GetMessage() returned same object twice")
	}

	// Release and get again to test reuse
	msg1.Release()
	msg3 := pool.GetMessage()
	if msg3 == nil {
		t.Error("GetMessage() after release returned nil")
	}
}

func TestMessagePool_GetBuffer(t *testing.T) {
	pool := NewMessagePool()

	buf1 := pool.GetBuffer()
	if buf1 == nil {
		t.Error("GetBuffer() returned nil")
	}

	if buf1.Len() != 0 {
		t.Errorf("GetBuffer() buffer length = %d, want 0", buf1.Len())
	}

	// Write to buffer
	buf1.WriteString("test")

	// Put buffer back
	pool.PutBuffer(buf1)

	// Get buffer again - should be reset
	buf2 := pool.GetBuffer()
	if buf2.Len() != 0 {
		t.Errorf("GetBuffer() after PutBuffer() length = %d, want 0", buf2.Len())
	}
}

func TestMessagePool_GetResponse(t *testing.T) {
	pool := NewMessagePool()

	resp1 := pool.GetResponse()
	if resp1 == nil {
		t.Fatal("GetResponse() returned nil")
	}

	if resp1.Response == nil {
		t.Error("PooledResponse.Response is nil")
	}

	if resp1.Buffer == nil {
		t.Error("PooledResponse.Buffer is nil")
	}

	if len(resp1.Response) != 0 {
		t.Errorf("PooledResponse.Response length = %d, want 0", len(resp1.Response))
	}
}

func TestPooledMessage_MarshalJSON(t *testing.T) {
	pool := NewMessagePool()
	pooledMsg := pool.GetMessage()
	defer pooledMsg.Release()

	// Set up message
	pooledMsg.Message.JSONRPC = "2.0"
	pooledMsg.Message.Method = "test/method"
	pooledMsg.Message.ID = reqID(1)

	// Marshal to JSON
	data, err := pooledMsg.MarshalJSON()
	if err != nil {
		t.Errorf("MarshalJSON() error = %v", err)
	}

	if len(data) == 0 {
		t.Error("MarshalJSON() returned empty data")
	}

	// Unmarshal to verify structure
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Errorf("Unmarshaling result error = %v", err)
	}

	if result["jsonrpc"] != "2.0" {
		t.Errorf("jsonrpc = %v, want 2.0", result["jsonrpc"])
	}

	if result["method"] != "test/method" {
		t.Errorf("method = %v, want test/method", result["method"])
	}
}

func TestPooledMessage_UnmarshalJSON(t *testing.T) {
	pool := NewMessagePool()
	pooledMsg := pool.GetMessage()
	defer pooledMsg.Release()

	testJSON := `{"jsonrpc":"2.0","method":"test/method","id":1}`

	err := pooledMsg.UnmarshalJSON([]byte(testJSON))
	if err != nil {
		t.Errorf("UnmarshalJSON() error = %v", err)
	}

	if pooledMsg.Message.JSONRPC != "2.0" {
		t.Errorf("JSONRPC = %v, want 2.0", pooledMsg.Message.JSONRPC)
	}

	if pooledMsg.Message.Method != "test/method" {
		t.Errorf("Method = %v, want test/method", pooledMsg.Message.Method)
	}
}

func TestPooledResponse_SetStandardFields(t *testing.T) {
	pool := NewMessagePool()
	resp := pool.GetResponse()
	defer resp.Release()

	resp.SetStandardFields("2.0", 123)

	if resp.Response["jsonrpc"] != "2.0" {
		t.Errorf("jsonrpc = %v, want 2.0", resp.Response["jsonrpc"])
	}

	if resp.Response["id"] != 123 {
		t.Errorf("id = %v, want 123", resp.Response["id"])
	}
}

func TestPooledResponse_SetResult(t *testing.T) {
	pool := NewMessagePool()
	resp := pool.GetResponse()
	defer resp.Release()

	testResult := map[string]string{"status": "ok"}
	resp.SetResult(testResult)

	if resp.Response["result"] == nil {
		t.Error("result not set")
	}

	// Verify error field was removed
	if _, exists := resp.Response["error"]; exists {
		t.Error("error field should be removed when setting result")
	}
}

func TestPooledResponse_SetError(t *testing.T) {
	pool := NewMessagePool()
	resp := pool.GetResponse()
	defer resp.Release()

	// Set result first
	resp.SetResult("test")

	// Now set error
	testError := &protocol.ErrorObject{
		Code:    -32600,
		Message: "Invalid Request",
	}
	resp.SetError(testError)

	if resp.Response["error"] == nil {
		t.Error("error not set")
	}

	// Verify result field was removed
	if _, exists := resp.Response["result"]; exists {
		t.Error("result field should be removed when setting error")
	}
}

func TestPooledResponse_MarshalJSON(t *testing.T) {
	pool := NewMessagePool()
	resp := pool.GetResponse()
	defer resp.Release()

	resp.SetStandardFields("2.0", 1)
	resp.SetResult(map[string]string{"status": "ok"})

	data, err := resp.MarshalJSON()
	if err != nil {
		t.Errorf("MarshalJSON() error = %v", err)
	}

	if len(data) == 0 {
		t.Error("MarshalJSON() returned empty data")
	}

	// Verify JSON structure
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Errorf("Unmarshaling result error = %v", err)
	}

	if result["jsonrpc"] != "2.0" {
		t.Errorf("jsonrpc = %v, want 2.0", result["jsonrpc"])
	}
}

func TestPooledResponse_JSONString(t *testing.T) {
	pool := NewMessagePool()
	resp := pool.GetResponse()
	defer resp.Release()

	resp.SetStandardFields("2.0", 1)
	resp.SetResult("test")

	jsonStr, err := resp.JSONString()
	if err != nil {
		t.Errorf("JSONString() error = %v", err)
	}

	if jsonStr == "" {
		t.Error("JSONString() returned empty string")
	}

	// Should be valid JSON
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		t.Errorf("JSONString() result not valid JSON: %v", err)
	}
}

func TestNewCancellationManager(t *testing.T) {
	cm := NewCancellationManager()

	if cm == nil {
		t.Fatal("NewCancellationManager() returned nil")
	}

	if cm.cancellations == nil {
		t.Error("cancellations map is nil")
	}
}

func TestCancellationManager_AddCancellation(t *testing.T) {
	cm := NewCancellationManager()

	requestID := protocol.RequestID(123)
	cancelCh := cm.AddCancellation(requestID)

	if cancelCh == nil {
		t.Error("AddCancellation() returned nil channel")
	}

	// Verify cancellation was added
	cm.mu.RLock()
	_, exists := cm.cancellations[requestID]
	cm.mu.RUnlock()

	if !exists {
		t.Error("Cancellation not added to map")
	}
}

func TestCancellationManager_CancelRequest(t *testing.T) {
	cm := NewCancellationManager()

	requestID := protocol.RequestID(123)
	cancelCh := cm.AddCancellation(requestID)

	// Cancel the request
	success := cm.CancelRequest(requestID)
	if !success {
		t.Error("CancelRequest() returned false for existing request")
	}

	// Verify channel was closed
	select {
	case <-cancelCh:
		// Channel was closed as expected
	default:
		t.Error("Cancellation channel was not closed")
	}

	// Try to cancel non-existent request
	success = cm.CancelRequest(protocol.RequestID(999))
	if success {
		t.Error("CancelRequest() returned true for non-existent request")
	}
}

func TestCancellationManager_GetCancellation(t *testing.T) {
	cm := NewCancellationManager()

	requestID := protocol.RequestID(123)
	originalCh := cm.AddCancellation(requestID)

	// Get cancellation
	gotCh, exists := cm.GetCancellation(requestID)
	if !exists {
		t.Error("GetCancellation() returned exists=false for existing request")
	}

	if gotCh != originalCh {
		t.Error("GetCancellation() returned different channel")
	}

	// Try to get non-existent cancellation
	_, exists = cm.GetCancellation(protocol.RequestID(999))
	if exists {
		t.Error("GetCancellation() returned exists=true for non-existent request")
	}
}

func TestCancellationManager_RemoveCancellation(t *testing.T) {
	cm := NewCancellationManager()

	requestID := protocol.RequestID(123)
	cancelCh := cm.AddCancellation(requestID)

	// Remove cancellation
	cm.RemoveCancellation(requestID)

	// Verify channel was closed
	select {
	case <-cancelCh:
		// Channel was closed as expected
	default:
		t.Error("Cancellation channel was not closed")
	}

	// Verify cancellation was removed
	_, exists := cm.GetCancellation(requestID)
	if exists {
		t.Error("Cancellation not removed from map")
	}
}

func TestCancellationManager_Clear(t *testing.T) {
	cm := NewCancellationManager()

	// Add multiple cancellations
	channels := make([]chan struct{}, 3)
	for i := 0; i < 3; i++ {
		requestID := *reqID(i)
		channels[i] = cm.AddCancellation(requestID)
	}

	// Clear all
	cm.Clear()

	// Verify all channels were closed
	for i, ch := range channels {
		select {
		case <-ch:
			// Channel was closed as expected
		default:
			t.Errorf("Channel %d was not closed", i)
		}
	}

	// Verify map is empty
	cm.mu.RLock()
	if len(cm.cancellations) != 0 {
		t.Errorf("cancellations map length = %d, want 0", len(cm.cancellations))
	}
	cm.mu.RUnlock()
}

func TestNewBatchProcessor(t *testing.T) {
	pool := NewMessagePool()
	bp := NewBatchProcessor(pool)

	if bp == nil {
		t.Fatal("NewBatchProcessor() returned nil")
	}

	if bp.pool != pool {
		t.Error("BatchProcessor.pool not set correctly")
	}
}

func TestBatchProcessor_ProcessBatch(t *testing.T) {
	pool := NewMessagePool()
	bp := NewBatchProcessor(pool)

	// Create test batch
	batch := []protocol.Message{
		{
			JSONRPC: "2.0",
			Method:  "test/method1",
			ID:      reqID(1),
		},
		{
			JSONRPC: "2.0",
			Method:  "test/notification", // No ID - notification
		},
		{
			JSONRPC: "2.0",
			Method:  "test/method2",
			ID:      reqID(2),
		},
	}

	responses, err := bp.ProcessBatch(batch)
	if err != nil {
		t.Errorf("ProcessBatch() error = %v", err)
	}

	// Should have 2 responses (notifications don't get responses)
	if len(responses) != 2 {
		t.Errorf("ProcessBatch() returned %d responses, want 2", len(responses))
	}

	// Verify responses have correct IDs
	expectedIDs := []interface{}{1, 2}
	for i, resp := range responses {
		if resp.Response["id"] != expectedIDs[i] {
			t.Errorf("Response %d ID = %v, want %v", i, resp.Response["id"], expectedIDs[i])
		}

		if resp.Response["jsonrpc"] != "2.0" {
			t.Errorf("Response %d jsonrpc = %v, want 2.0", i, resp.Response["jsonrpc"])
		}
	}

	// Clean up
	bp.ReleaseBatch(responses)
}

func TestBatchProcessor_ReleaseBatch(t *testing.T) {
	pool := NewMessagePool()
	bp := NewBatchProcessor(pool)

	// Create some responses
	responses := []*PooledResponse{
		pool.GetResponse(),
		pool.GetResponse(),
		nil, // Test nil handling
	}

	// This should not panic
	bp.ReleaseBatch(responses)
}

func TestNewRequestContext(t *testing.T) {
	rc := NewRequestContext()

	if rc == nil {
		t.Error("NewRequestContext() returned nil")
	}
}

func TestRequestContext_GetBody(t *testing.T) {
	rc := NewRequestContext()

	testBody := []byte(`{"jsonrpc":"2.0","method":"initialize","id":1}`)
	callCount := 0

	getBodyFunc := func() ([]byte, error) {
		callCount++
		return testBody, nil
	}

	// First call
	body1, err1 := rc.GetBody(getBodyFunc)
	if err1 != nil {
		t.Errorf("GetBody() error = %v", err1)
	}

	if !bytes.Equal(body1, testBody) {
		t.Errorf("GetBody() = %s, want %s", body1, testBody)
	}

	if callCount != 1 {
		t.Errorf("getBodyFunc called %d times, want 1", callCount)
	}

	// Second call - should use cached result
	body2, err2 := rc.GetBody(getBodyFunc)
	if err2 != nil {
		t.Errorf("Second GetBody() error = %v", err2)
	}

	if !bytes.Equal(body2, testBody) {
		t.Errorf("Second GetBody() = %s, want %s", body2, testBody)
	}

	if callCount != 1 {
		t.Errorf("getBodyFunc called %d times after second call, want 1", callCount)
	}
}

func TestRequestContext_IsInitialize(t *testing.T) {
	rc := NewRequestContext()

	// Test with initialize request
	initBody := []byte(`{"jsonrpc":"2.0","method":"initialize","id":1}`)
	_, err := rc.GetBody(func() ([]byte, error) {
		return initBody, nil
	})
	if err != nil {
		t.Errorf("GetBody() error = %v", err)
	}

	if !rc.IsInitialize() {
		t.Error("IsInitialize() = false, want true for initialize request")
	}

	if rc.GetMethod() != "initialize" {
		t.Errorf("GetMethod() = %s, want initialize", rc.GetMethod())
	}

	// Test with non-initialize request
	rc2 := NewRequestContext()
	testBody := []byte(`{"jsonrpc":"2.0","method":"test/method","id":1}`)
	_, err = rc2.GetBody(func() ([]byte, error) {
		return testBody, nil
	})
	if err != nil {
		t.Errorf("GetBody() error = %v", err)
	}

	if rc2.IsInitialize() {
		t.Error("IsInitialize() = true, want false for non-initialize request")
	}

	if rc2.GetMethod() != "test/method" {
		t.Errorf("GetMethod() = %s, want test/method", rc2.GetMethod())
	}
}

func TestRequestContext_Reset(t *testing.T) {
	rc := NewRequestContext()

	// Set up context
	testBody := []byte(`{"jsonrpc":"2.0","method":"initialize","id":1}`)
	_, err := rc.GetBody(func() ([]byte, error) {
		return testBody, nil
	})
	if err != nil {
		t.Errorf("GetBody() error = %v", err)
	}

	// Verify state
	if !rc.IsInitialize() {
		t.Error("IsInitialize() should be true before reset")
	}

	// Reset
	rc.Reset()

	// Verify reset state
	if rc.IsInitialize() {
		t.Error("IsInitialize() should be false after reset")
	}

	if rc.GetMethod() != "" {
		t.Errorf("GetMethod() = %s, want empty string after reset", rc.GetMethod())
	}
}

