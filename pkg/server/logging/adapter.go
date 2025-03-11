package logging

import (
	"fmt"

	"github.com/XiaoConstantine/mcp-go/pkg/logging"
	"github.com/XiaoConstantine/mcp-go/pkg/model"
)

// ManagerAdapter adapts a Manager to implement the logging.Logger interface.
type ManagerAdapter struct {
	manager *Manager
	logger  string // Component/module name
}

// NewManagerAdapter creates a new adapter that wraps a Manager and implements logging.Logger.
func NewManagerAdapter(manager *Manager, loggerName string) logging.Logger {
	return &ManagerAdapter{
		manager: manager,
		logger:  loggerName,
	}
}

// Debug logs a debug message.
func (a *ManagerAdapter) Debug(msg string, keysAndValues ...interface{}) {
	a.manager.Log(models.LogLevelDebug, formatLogMessage(msg, keysAndValues...), a.logger)
}

// Info logs an info message.
func (a *ManagerAdapter) Info(msg string, keysAndValues ...interface{}) {
	a.manager.Log(models.LogLevelInfo, formatLogMessage(msg, keysAndValues...), a.logger)
}

// Warn logs a warning message.
func (a *ManagerAdapter) Warn(msg string, keysAndValues ...interface{}) {
	a.manager.Log(models.LogLevelWarning, formatLogMessage(msg, keysAndValues...), a.logger)
}

// Error logs an error message.
func (a *ManagerAdapter) Error(msg string, keysAndValues ...interface{}) {
	a.manager.Log(models.LogLevelError, formatLogMessage(msg, keysAndValues...), a.logger)
}

// formatLogMessage formats a log message with key-value pairs.
func formatLogMessage(msg string, keysAndValues ...interface{}) string {
	if len(keysAndValues) == 0 {
		return msg
	}

	formatted := msg + " {"
	for i := 0; i < len(keysAndValues); i += 2 {
		if i > 0 {
			formatted += ", "
		}

		key := keysAndValues[i]
		if i+1 < len(keysAndValues) {
			value := keysAndValues[i+1]
			formatted += fmt.Sprintf("%v: %v", key, value)
		} else {
			formatted += fmt.Sprintf("%v: ?", key)
		}
	}
	formatted += "}"

	return formatted
}
