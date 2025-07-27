package transport

import (
	"bytes"
	"encoding/json"
	"sync"

	"github.com/XiaoConstantine/mcp-go/pkg/protocol"
)

// MessagePool manages reusable message objects to reduce GC pressure.
type MessagePool struct {
	messagePool  sync.Pool
	bufferPool   sync.Pool
	responsePool sync.Pool
}

// PooledMessage is a reusable message wrapper.
type PooledMessage struct {
	Message *protocol.Message
	Buffer  *bytes.Buffer
	pool    *MessagePool
}

// PooledResponse is a reusable response wrapper.
type PooledResponse struct {
	Response map[string]interface{}
	Buffer   *bytes.Buffer
	pool     *MessagePool
}

// NewMessagePool creates a new message pool.
func NewMessagePool() *MessagePool {
	pool := &MessagePool{}

	pool.messagePool = sync.Pool{
		New: func() interface{} {
			return &PooledMessage{
				Message: &protocol.Message{},
				Buffer:  &bytes.Buffer{},
				pool:    pool,
			}
		},
	}

	pool.bufferPool = sync.Pool{
		New: func() interface{} {
			return &bytes.Buffer{}
		},
	}

	pool.responsePool = sync.Pool{
		New: func() interface{} {
			return &PooledResponse{
				Response: make(map[string]interface{}),
				Buffer:   &bytes.Buffer{},
				pool:     pool,
			}
		},
	}

	return pool
}

// GetMessage gets a pooled message.
func (mp *MessagePool) GetMessage() *PooledMessage {
	pm := mp.messagePool.Get().(*PooledMessage)
	pm.reset()
	return pm
}

// GetBuffer gets a pooled buffer.
func (mp *MessagePool) GetBuffer() *bytes.Buffer {
	buf := mp.bufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	return buf
}

// GetResponse gets a pooled response.
func (mp *MessagePool) GetResponse() *PooledResponse {
	pr := mp.responsePool.Get().(*PooledResponse)
	pr.reset()
	return pr
}

// PutBuffer returns a buffer to the pool.
func (mp *MessagePool) PutBuffer(buf *bytes.Buffer) {
	if buf != nil {
		buf.Reset()
		mp.bufferPool.Put(buf)
	}
}

// reset resets the pooled message for reuse.
func (pm *PooledMessage) reset() {
	pm.Message.JSONRPC = ""
	pm.Message.Method = ""
	pm.Message.ID = nil
	pm.Message.Params = nil
	pm.Message.Result = nil
	pm.Message.Error = nil
	pm.Buffer.Reset()
}

// Release returns the pooled message to the pool.
func (pm *PooledMessage) Release() {
	if pm.pool != nil {
		pm.pool.messagePool.Put(pm)
	}
}

// MarshalJSON marshals the message to JSON using the pooled buffer.
func (pm *PooledMessage) MarshalJSON() ([]byte, error) {
	pm.Buffer.Reset()
	encoder := json.NewEncoder(pm.Buffer)
	if err := encoder.Encode(pm.Message); err != nil {
		return nil, err
	}

	// Remove the trailing newline added by Encode
	data := pm.Buffer.Bytes()
	if len(data) > 0 && data[len(data)-1] == '\n' {
		data = data[:len(data)-1]
	}

	// Return a copy since the buffer will be reused
	result := make([]byte, len(data))
	copy(result, data)
	return result, nil
}

// UnmarshalJSON unmarshals JSON into the message.
func (pm *PooledMessage) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, pm.Message)
}

// reset resets the pooled response for reuse.
func (pr *PooledResponse) reset() {
	// Clear the map but keep the underlying storage
	for k := range pr.Response {
		delete(pr.Response, k)
	}
	pr.Buffer.Reset()
}

// Release returns the pooled response to the pool.
func (pr *PooledResponse) Release() {
	if pr.pool != nil {
		pr.pool.responsePool.Put(pr)
	}
}

// SetStandardFields sets standard JSON-RPC response fields.
func (pr *PooledResponse) SetStandardFields(jsonrpc string, id interface{}) {
	pr.Response["jsonrpc"] = jsonrpc
	pr.Response["id"] = id
}

// SetResult sets the result field.
func (pr *PooledResponse) SetResult(result interface{}) {
	pr.Response["result"] = result
	delete(pr.Response, "error") // Remove error if setting result
}

// SetError sets the error field.
func (pr *PooledResponse) SetError(err *protocol.ErrorObject) {
	pr.Response["error"] = err
	delete(pr.Response, "result") // Remove result if setting error
}

// MarshalJSON marshals the response to JSON using the pooled buffer.
func (pr *PooledResponse) MarshalJSON() ([]byte, error) {
	pr.Buffer.Reset()
	encoder := json.NewEncoder(pr.Buffer)
	if err := encoder.Encode(pr.Response); err != nil {
		return nil, err
	}

	// Remove the trailing newline added by Encode
	data := pr.Buffer.Bytes()
	if len(data) > 0 && data[len(data)-1] == '\n' {
		data = data[:len(data)-1]
	}

	// Return a copy since the buffer will be reused
	result := make([]byte, len(data))
	copy(result, data)
	return result, nil
}

// JSONString returns the JSON string representation.
func (pr *PooledResponse) JSONString() (string, error) {
	data, err := pr.MarshalJSON()
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// CancellationManager manages request cancellations.
type CancellationManager struct {
	cancellations map[protocol.RequestID]chan struct{}
	mu            sync.RWMutex
}

// NewCancellationManager creates a new cancellation manager.
func NewCancellationManager() *CancellationManager {
	return &CancellationManager{
		cancellations: make(map[protocol.RequestID]chan struct{}),
	}
}

// AddCancellation adds a cancellation channel for a request.
func (cm *CancellationManager) AddCancellation(requestID protocol.RequestID) chan struct{} {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cancel := make(chan struct{})
	cm.cancellations[requestID] = cancel
	return cancel
}

// CancelRequest cancels a request.
func (cm *CancellationManager) CancelRequest(requestID protocol.RequestID) bool {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cancel, exists := cm.cancellations[requestID]; exists {
		close(cancel)
		delete(cm.cancellations, requestID)
		return true
	}
	return false
}

// RemoveCancellation removes a cancellation channel.
func (cm *CancellationManager) RemoveCancellation(requestID protocol.RequestID) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cancel, exists := cm.cancellations[requestID]; exists {
		close(cancel)
		delete(cm.cancellations, requestID)
	}
}

// GetCancellation gets a cancellation channel for a request.
func (cm *CancellationManager) GetCancellation(requestID protocol.RequestID) (chan struct{}, bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	cancel, exists := cm.cancellations[requestID]
	return cancel, exists
}

// Clear clears all cancellations.
func (cm *CancellationManager) Clear() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	for requestID, cancel := range cm.cancellations {
		close(cancel)
		delete(cm.cancellations, requestID)
	}
}

// BatchProcessor helps process batch requests efficiently.
type BatchProcessor struct {
	pool *MessagePool
}

// NewBatchProcessor creates a new batch processor.
func NewBatchProcessor(pool *MessagePool) *BatchProcessor {
	return &BatchProcessor{
		pool: pool,
	}
}

// ProcessBatch processes a batch of messages.
func (bp *BatchProcessor) ProcessBatch(batch []protocol.Message) ([]*PooledResponse, error) {
	responses := make([]*PooledResponse, 0, len(batch))

	for _, msg := range batch {
		// Skip notifications (no ID means no response expected)
		if msg.ID == nil {
			continue
		}

		// Create a pooled response for this message
		resp := bp.pool.GetResponse()
		resp.SetStandardFields("2.0", *msg.ID)

		responses = append(responses, resp)
	}

	return responses, nil
}

// ReleaseBatch releases all responses in a batch back to the pool.
func (bp *BatchProcessor) ReleaseBatch(responses []*PooledResponse) {
	for _, resp := range responses {
		if resp != nil {
			resp.Release()
		}
	}
}

// RequestContext provides thread-safe access to request body with sync.Once pattern.
type RequestContext struct {
	bodyOnce sync.Once
	body     []byte
	err      error
	method   string
	isInit   bool
}

// NewRequestContext creates a new request context.
func NewRequestContext() *RequestContext {
	return &RequestContext{}
}

// GetBody returns the request body, reading it only once.
func (rc *RequestContext) GetBody(getBodyFunc func() ([]byte, error)) ([]byte, error) {
	rc.bodyOnce.Do(func() {
		rc.body, rc.err = getBodyFunc()
		if rc.err == nil && len(rc.body) > 0 {
			// Try to determine if this is an initialize request
			var msg protocol.Message
			if json.Unmarshal(rc.body, &msg) == nil {
				rc.method = msg.Method
				rc.isInit = (msg.Method == "initialize")
			}
		}
	})

	return rc.body, rc.err
}

// IsInitialize returns true if this is an initialize request.
func (rc *RequestContext) IsInitialize() bool {
	return rc.isInit
}

// GetMethod returns the request method.
func (rc *RequestContext) GetMethod() string {
	return rc.method
}

// Reset resets the request context for reuse.
func (rc *RequestContext) Reset() {
	rc.bodyOnce = sync.Once{}
	rc.body = nil
	rc.err = nil
	rc.method = ""
	rc.isInit = false
}
