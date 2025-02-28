package logging

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/XiaoConstantine/mcp-go/pkg/models"
)

func TestNewManager(t *testing.T) {
	manager := NewManager()
	assert.NotNil(t, manager)
	assert.Equal(t, models.LogLevelInfo, manager.level)
	assert.Nil(t, manager.sink)
}

func TestSetLevel(t *testing.T) {
	manager := NewManager()

	// Set level to debug
	manager.SetLevel(models.LogLevelDebug)
	assert.Equal(t, models.LogLevelDebug, manager.GetLevel())

	// Set level to error
	manager.SetLevel(models.LogLevelError)
	assert.Equal(t, models.LogLevelError, manager.GetLevel())
}

func TestSetSink(t *testing.T) {
	manager := NewManager()

	var receivedLevel models.LogLevel
	var receivedData interface{}
	var receivedLogger string
	var wg sync.WaitGroup

	// Set sink function
	manager.SetSink(func(level models.LogLevel, data interface{}, logger string) {
		receivedLevel = level
		receivedData = data
		receivedLogger = logger
		wg.Done()
	})

	// Log a message
	wg.Add(1)
	manager.Log(models.LogLevelWarning, "test message", "test-logger")
	wg.Wait()

	// Verify message was received by sink
	assert.Equal(t, models.LogLevelWarning, receivedLevel)
	assert.Equal(t, "test message", receivedData)
	assert.Equal(t, "test-logger", receivedLogger)
}

func TestLogFiltering(t *testing.T) {
	manager := NewManager()

	var received bool
	var wg sync.WaitGroup

	// Set sink function
	manager.SetSink(func(level models.LogLevel, data interface{}, logger string) {
		received = true
		wg.Done()
	})

	// Set level to warning
	manager.SetLevel(models.LogLevelWarning)

	// Log at info level (should be filtered out)
	manager.Log(models.LogLevelInfo, "info message", "")

	// Log at error level (should pass through)
	wg.Add(1)
	manager.Log(models.LogLevelError, "error message", "")
	wg.Wait()

	// Verify only error message was received
	assert.True(t, received)
}

func TestLogWithoutSink(t *testing.T) {
	manager := NewManager()

	// This should not panic even though no sink is set
	manager.Log(models.LogLevelInfo, "test message", "")
}

func TestIsLevelEnabled(t *testing.T) {
	// Test cases with different level combinations
	testCases := []struct {
		name          string
		currentLevel  models.LogLevel
		messageLevel  models.LogLevel
		shouldProcess bool
	}{
		{"Same level", models.LogLevelInfo, models.LogLevelInfo, true},
		{"More severe", models.LogLevelInfo, models.LogLevelError, true},
		{"Less severe", models.LogLevelError, models.LogLevelInfo, false},
		{"Debug includes all", models.LogLevelDebug, models.LogLevelInfo, true},
		{"Emergency only", models.LogLevelEmergency, models.LogLevelAlert, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isLevelEnabled(tc.currentLevel, tc.messageLevel)
			assert.Equal(t, tc.shouldProcess, result)
		})
	}
}
