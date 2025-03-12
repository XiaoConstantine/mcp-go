package logging

import (
	"testing"
	"sync"

	"github.com/XiaoConstantine/mcp-go/pkg/model"
	"github.com/stretchr/testify/assert"
)

func TestFormatLogMessage(t *testing.T) {
	// Test with no key-value pairs
	msg := formatLogMessage("Simple message")
	assert.Equal(t, "Simple message", msg)

	// Test with key-value pairs
	msg = formatLogMessage("Message with keys", "key1", "value1", "key2", 42)
	assert.Equal(t, "Message with keys {key1: value1, key2: 42}", msg)

	// Test with odd number of key-value pairs
	msg = formatLogMessage("Odd pairs", "key1", "value1", "orphaned")
	assert.Equal(t, "Odd pairs {key1: value1, orphaned: ?}", msg)
}

func TestManagerAdapter(t *testing.T) {
	manager := NewManager()
	// Explicitly set to Debug level to ensure all messages pass through
	manager.SetLevel(models.LogLevelDebug)

	type logRecord struct {
		level  models.LogLevel
		data   interface{}
		logger string
	}
	
	records := make([]logRecord, 0, 4)
	mut := sync.Mutex{}
	waiter := make(chan struct{}, 1)

	// Set sink function to capture log messages
	manager.SetSink(func(level models.LogLevel, data interface{}, logger string) {
		mut.Lock()
		records = append(records, logRecord{level, data, logger})
		mut.Unlock()
		waiter <- struct{}{}
	})

	// Create adapter
	adapter := NewManagerAdapter(manager, "test-component")

	expectedRecords := []logRecord{
		{models.LogLevelDebug, "Debug message {key: value}", "test-component"},
		{models.LogLevelInfo, "Info message {key: value}", "test-component"},
		{models.LogLevelWarning, "Warning message {key: value}", "test-component"},
		{models.LogLevelError, "Error message {key: value}", "test-component"},
	}

	// Test Debug level
	adapter.Debug("Debug message", "key", "value")
	<-waiter

	// Test Info level
	adapter.Info("Info message", "key", "value")
	<-waiter

	// Test Warn level
	adapter.Warn("Warning message", "key", "value")
	<-waiter

	// Test Error level
	adapter.Error("Error message", "key", "value")
	<-waiter

	// Verify results
	mut.Lock()
	defer mut.Unlock()
	assert.Equal(t, len(expectedRecords), len(records))
	for i, expected := range expectedRecords {
		if i < len(records) {
			assert.Equal(t, expected.level, records[i].level)
			assert.Equal(t, expected.data, records[i].data)
			assert.Equal(t, expected.logger, records[i].logger)
		}
	}
}

func TestManagerAdapterWithoutKeyValues(t *testing.T) {
	manager := NewManager()
	// Explicitly set to Debug level to ensure all messages pass through
	manager.SetLevel(models.LogLevelDebug)

	var receivedLevel models.LogLevel
	var receivedData interface{}
	var receivedLogger string
	waiter := make(chan struct{}, 1)

	// Set sink function to capture log messages
	manager.SetSink(func(level models.LogLevel, data interface{}, logger string) {
		receivedLevel = level
		receivedData = data
		receivedLogger = logger
		waiter <- struct{}{}
	})

	// Create adapter
	adapter := NewManagerAdapter(manager, "test-component")

	// Test without key-value pairs
	adapter.Info("Simple message")
	<-waiter

	assert.Equal(t, models.LogLevelInfo, receivedLevel)
	assert.Equal(t, "Simple message", receivedData)
	assert.Equal(t, "test-component", receivedLogger)
}
