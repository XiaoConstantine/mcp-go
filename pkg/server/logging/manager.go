package logging

import (
	"sync"

	"github.com/XiaoConstantine/mcp-go/pkg/model"
)

// Manager handles logging operations and level control.
type Manager struct {
	mu    sync.RWMutex
	level models.LogLevel
	sink  func(level models.LogLevel, data interface{}, logger string)
}

// NewManager creates a new logging manager with default settings.
func NewManager() *Manager {
	return &Manager{
		level: models.LogLevelInfo, // Set default level to Info
	}
}

// SetLevel sets the current logging level.
func (m *Manager) SetLevel(level models.LogLevel) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.level = level
}

// GetLevel returns the current logging level.
func (m *Manager) GetLevel() models.LogLevel {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.level
}

// SetSink sets the function that will receive log messages.
func (m *Manager) SetSink(sink func(level models.LogLevel, data interface{}, logger string)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sink = sink
}

// Log logs a message if the level is sufficient.
func (m *Manager) Log(level models.LogLevel, data interface{}, logger string) {
	m.mu.RLock()
	currentLevel := m.level
	sink := m.sink
	m.mu.RUnlock()

	// Check if this log level should be processed
	if !isLevelEnabled(currentLevel, level) {
		return
	}

	if sink != nil {
		sink(level, data, logger)
	}
}

// isLevelEnabled checks if a log level should be processed given the current level.
// Returns true if messageLevel is equal to or more severe than currentLevel.
func isLevelEnabled(currentLevel, messageLevel models.LogLevel) bool {
	// Order of severity (from most to least severe)
	levels := []models.LogLevel{
		models.LogLevelEmergency,
		models.LogLevelAlert,
		models.LogLevelCritical,
		models.LogLevelError,
		models.LogLevelWarning,
		models.LogLevelNotice,
		models.LogLevelInfo,
		models.LogLevelDebug,
	}

	// Find indices
	var currentIdx, messageIdx int
	for i, l := range levels {
		if l == currentLevel {
			currentIdx = i
		}
		if l == messageLevel {
			messageIdx = i
		}
	}

	// Message level is enabled if it's less than or equal to current level
	// (Lower index means higher severity)
	return messageIdx <= currentIdx
}
