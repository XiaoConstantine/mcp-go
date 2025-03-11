package logging

import (
	"fmt"
	"log"
	"os"
)

// Level represents a logging level.
type Level int

const (
	// DebugLevel is for detailed debugging information.
	DebugLevel Level = iota
	// InfoLevel is for general operational information.
	InfoLevel
	// WarnLevel is for warnings that might need attention.
	WarnLevel
	// ErrorLevel is for errors that need immediate attention.
	ErrorLevel
)

// Logger is a minimal logging interface compatible with many logging libraries.
type Logger interface {
	Debug(msg string, keysAndValues ...interface{})
	Info(msg string, keysAndValues ...interface{})
	Warn(msg string, keysAndValues ...interface{})
	Error(msg string, keysAndValues ...interface{})
}

// StdLogger implements Logger using Go's standard library.
type StdLogger struct {
	logger *log.Logger
	level  Level
}

// NewStdLogger creates a new StdLogger with the specified level.
func NewStdLogger(level Level) *StdLogger {
	return &StdLogger{
		logger: log.New(os.Stderr, "", log.LstdFlags),
		level:  level,
	}
}

// Debug logs a debug message.
func (l *StdLogger) Debug(msg string, keysAndValues ...interface{}) {
	if l.level <= DebugLevel {
		l.log("DEBUG", msg, keysAndValues...)
	}
}

// Info logs an info message.
func (l *StdLogger) Info(msg string, keysAndValues ...interface{}) {
	if l.level <= InfoLevel {
		l.log("INFO", msg, keysAndValues...)
	}
}

// Warn logs a warning message.
func (l *StdLogger) Warn(msg string, keysAndValues ...interface{}) {
	if l.level <= WarnLevel {
		l.log("WARN", msg, keysAndValues...)
	}
}

// Error logs an error message.
func (l *StdLogger) Error(msg string, keysAndValues ...interface{}) {
	if l.level <= ErrorLevel {
		l.log("ERROR", msg, keysAndValues...)
	}
}

// log formats and logs a message with the specified level.
func (l *StdLogger) log(level, msg string, keysAndValues ...interface{}) {
	// Format key-value pairs
	var kvStr string
	if len(keysAndValues) > 0 {
		kvStr = " "
		for i := 0; i < len(keysAndValues); i += 2 {
			if i+1 < len(keysAndValues) {
				kvStr += fmt.Sprintf("%v=%v ", keysAndValues[i], keysAndValues[i+1])
			} else {
				kvStr += fmt.Sprintf("%v=? ", keysAndValues[i])
			}
		}
	}

	l.logger.Printf("[%s] %s%s", level, msg, kvStr)
}

// NoopLogger is a logger that discards all messages.
type NoopLogger struct{}

// NewNoopLogger creates a new NoopLogger.
func NewNoopLogger() *NoopLogger {
	return &NoopLogger{}
}

// Debug is a no-op.
func (l *NoopLogger) Debug(msg string, keysAndValues ...interface{}) {}

// Info is a no-op.
func (l *NoopLogger) Info(msg string, keysAndValues ...interface{}) {}

// Warn is a no-op.
func (l *NoopLogger) Warn(msg string, keysAndValues ...interface{}) {}

// Error is a no-op.
func (l *NoopLogger) Error(msg string, keysAndValues ...interface{}) {}

// TestLogger logs to a testing.T object.
type TestLogger struct {
	testingT TestingT
	level    Level
}

// TestingT is an interface that wraps the methods of testing.T that we need.
type TestingT interface {
	Logf(format string, args ...interface{})
}

// NewTestLogger creates a logger that outputs to a testing.T.
func NewTestLogger(t TestingT) *TestLogger {
	return &TestLogger{
		testingT: t,
		level:    DebugLevel, // Set debug level by default for tests
	}
}

// Debug logs a debug message to the test output.
func (l *TestLogger) Debug(msg string, keysAndValues ...interface{}) {
	if l.level <= DebugLevel {
		l.log("DEBUG", msg, keysAndValues...)
	}
}

// Info logs an info message to the test output.
func (l *TestLogger) Info(msg string, keysAndValues ...interface{}) {
	if l.level <= InfoLevel {
		l.log("INFO", msg, keysAndValues...)
	}
}

// Warn logs a warning message to the test output.
func (l *TestLogger) Warn(msg string, keysAndValues ...interface{}) {
	if l.level <= WarnLevel {
		l.log("WARN", msg, keysAndValues...)
	}
}

// Error logs an error message to the test output.
func (l *TestLogger) Error(msg string, keysAndValues ...interface{}) {
	if l.level <= ErrorLevel {
		l.log("ERROR", msg, keysAndValues...)
	}
}

// SetLevel sets the minimum log level for the TestLogger.
func (l *TestLogger) SetLevel(level Level) {
	l.level = level
}

// log formats and logs a message with the specified level to the test output.
func (l *TestLogger) log(level, msg string, keysAndValues ...interface{}) {
	// Format key-value pairs
	var kvStr string
	if len(keysAndValues) > 0 {
		kvStr = " "
		for i := 0; i < len(keysAndValues); i += 2 {
			if i+1 < len(keysAndValues) {
				kvStr += fmt.Sprintf("%v=%v ", keysAndValues[i], keysAndValues[i+1])
			} else {
				kvStr += fmt.Sprintf("%v=? ", keysAndValues[i])
			}
		}
	}

	l.testingT.Logf("[%s] %s%s", level, msg, kvStr)
}
