package logging

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

// testWriter is a simple io.Writer that captures log output.
type testWriter struct {
	buf bytes.Buffer
}

func (w *testWriter) Write(p []byte) (n int, err error) {
	return w.buf.Write(p)
}

func (w *testWriter) String() string {
	return w.buf.String()
}

func TestStdLogger(t *testing.T) {
	// Create a test writer to capture log output
	writer := &testWriter{}
	
	// Create a logger with a custom writer
	stdLog := NewStdLogger(DebugLevel)
	stdLog.logger = log.New(writer, "", 0) // No timestamps to make testing easier
	
	// Test Debug level
	stdLog.Debug("Debug message", "key1", "value1")
	if !strings.Contains(writer.String(), "[DEBUG] Debug message key1=value1") {
		t.Errorf("Expected Debug message to be logged, got: %s", writer.String())
	}
	
	// Clear the buffer
	writer.buf.Reset()
	
	// Test Info level
	stdLog.Info("Info message", "key2", "value2")
	if !strings.Contains(writer.String(), "[INFO] Info message key2=value2") {
		t.Errorf("Expected Info message to be logged, got: %s", writer.String())
	}
	
	// Clear the buffer
	writer.buf.Reset()
	
	// Test Warn level
	stdLog.Warn("Warning message", "key3", "value3")
	if !strings.Contains(writer.String(), "[WARN] Warning message key3=value3") {
		t.Errorf("Expected Warning message to be logged, got: %s", writer.String())
	}
	
	// Clear the buffer
	writer.buf.Reset()
	
	// Test Error level
	stdLog.Error("Error message", "key4", "value4")
	if !strings.Contains(writer.String(), "[ERROR] Error message key4=value4") {
		t.Errorf("Expected Error message to be logged, got: %s", writer.String())
	}
}

func TestStdLoggerLevels(t *testing.T) {
	// Create a test writer to capture log output
	writer := &testWriter{}
	
	// Create a logger with Info level
	stdLog := NewStdLogger(InfoLevel)
	stdLog.logger = log.New(writer, "", 0) // No timestamps to make testing easier
	
	// Debug messages should not be logged
	stdLog.Debug("Debug message")
	if writer.String() != "" {
		t.Errorf("Expected no output for Debug at InfoLevel, got: %s", writer.String())
	}
	
	// Info messages should be logged
	stdLog.Info("Info message")
	if !strings.Contains(writer.String(), "[INFO] Info message") {
		t.Errorf("Expected Info message to be logged, got: %s", writer.String())
	}
	
	// Clear the buffer
	writer.buf.Reset()
	
	// Create a logger with Error level
	stdLog = NewStdLogger(ErrorLevel)
	stdLog.logger = log.New(writer, "", 0)
	
	// Debug, Info, and Warn messages should not be logged
	stdLog.Debug("Debug message")
	stdLog.Info("Info message")
	stdLog.Warn("Warning message")
	if writer.String() != "" {
		t.Errorf("Expected no output for Debug/Info/Warn at ErrorLevel, got: %s", writer.String())
	}
	
	// Error messages should be logged
	stdLog.Error("Error message")
	if !strings.Contains(writer.String(), "[ERROR] Error message") {
		t.Errorf("Expected Error message to be logged, got: %s", writer.String())
	}
}

func TestLogKeyValueFormatting(t *testing.T) {
	// Create a test writer to capture log output
	writer := &testWriter{}
	
	// Create a logger
	stdLog := NewStdLogger(DebugLevel)
	stdLog.logger = log.New(writer, "", 0)
	
	// Test with multiple key-value pairs
	stdLog.Info("Multi KV", "key1", "value1", "key2", 42, "key3", true)
	logOutput := writer.String()
	
	// Check that all key-value pairs are present
	if !strings.Contains(logOutput, "key1=value1") {
		t.Errorf("Expected 'key1=value1' in log output, got: %s", logOutput)
	}
	if !strings.Contains(logOutput, "key2=42") {
		t.Errorf("Expected 'key2=42' in log output, got: %s", logOutput)
	}
	if !strings.Contains(logOutput, "key3=true") {
		t.Errorf("Expected 'key3=true' in log output, got: %s", logOutput)
	}
	
	// Clear the buffer
	writer.buf.Reset()
	
	// Test with odd number of key-value pairs
	stdLog.Info("Odd KV", "key1", "value1", "orphaned")
	logOutput = writer.String()
	
	// Check that the orphaned key is handled correctly
	if !strings.Contains(logOutput, "orphaned=?") {
		t.Errorf("Expected 'orphaned=?' in log output, got: %s", logOutput)
	}
}

func TestNoopLogger(t *testing.T) {
	// Create a NoopLogger
	logger := NewNoopLogger()
	
	// All these should be no-ops, test that they don't panic
	logger.Debug("Debug message", "key", "value")
	logger.Info("Info message", "key", "value")
	logger.Warn("Warning message", "key", "value")
	logger.Error("Error message", "key", "value")
	
	// No way to verify that nothing happened, but the test will fail if there's a panic
}
