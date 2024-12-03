// Package models provides core model definitions for MCP
package models

import (
	"fmt"
)

// Role represents the sender or recipient in a conversation
type Role string

const (
	// RoleAssistant represents an AI assistant in the conversation
	RoleAssistant Role = "assistant"
	// RoleUser represents a human user in the conversation
	RoleUser Role = "user"
)

// String returns the string representation of the Role
func (r Role) String() string {
	return string(r)
}

// IsValid checks if the Role is a valid value
func (r Role) IsValid() bool {
	switch r {
	case RoleAssistant, RoleUser:
		return true
	default:
		return false
	}
}

// Implementation describes the name and version of an MCP implementation
type Implementation struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Validate performs validation on Implementation fields
func (i Implementation) Validate() error {
	if i.Name == "" {
		return fmt.Errorf("implementation name cannot be empty")
	}
	if i.Version == "" {
		return fmt.Errorf("implementation version cannot be empty")
	}
	return nil
}

// Annotatable defines the interface for objects that can have annotations
type Annotatable interface {
	GetAnnotations() *Annotations
	SetAnnotations(*Annotations)
}

// Annotations provides metadata about how objects should be used or displayed
type Annotations struct {
	Audience []Role    `json:"audience,omitempty"`
	Priority *float64 `json:"priority,omitempty"`
}

// Validate ensures all annotations are valid
func (a *Annotations) Validate() error {
	if a == nil {
		return nil
	}

	for _, role := range a.Audience {
		if !role.IsValid() {
			return fmt.Errorf("invalid role in audience: %s", role)
		}
	}

	if a.Priority != nil {
		if *a.Priority < 0 || *a.Priority > 1 {
			return fmt.Errorf("priority must be between 0 and 1, got %f", *a.Priority)
		}
	}

	return nil
}

// BaseAnnotated provides a base implementation of Annotatable
type BaseAnnotated struct {
	Annotations *Annotations `json:"annotations,omitempty"`
}

// GetAnnotations implements Annotatable
func (b *BaseAnnotated) GetAnnotations() *Annotations {
	return b.Annotations
}

// SetAnnotations implements Annotatable
func (b *BaseAnnotated) SetAnnotations(a *Annotations) {
	b.Annotations = a
}

// LogLevel represents the severity of a log message
type LogLevel string

const (
	LogLevelEmergency LogLevel = "emergency"
	LogLevelAlert     LogLevel = "alert"
	LogLevelCritical  LogLevel = "critical"
	LogLevelError     LogLevel = "error"
	LogLevelWarning   LogLevel = "warning"
	LogLevelNotice    LogLevel = "notice"
	LogLevelInfo      LogLevel = "info"
	LogLevelDebug     LogLevel = "debug"
)

// IsValid checks if the LogLevel is a valid value
func (l LogLevel) IsValid() bool {
	switch l {
	case LogLevelEmergency, LogLevelAlert, LogLevelCritical,
		LogLevelError, LogLevelWarning, LogLevelNotice,
		LogLevelInfo, LogLevelDebug:
		return true
	default:
		return false
	}
}

// String returns the string representation of the LogLevel
func (l LogLevel) String() string {
	return string(l)
}

// ModelHint provides hints for model selection
type ModelHint struct {
	Name string `json:"name,omitempty"`
}

// Validate ensures the ModelHint is valid
func (m ModelHint) Validate() error {
	if m.Name == "" {
		return fmt.Errorf("model hint name cannot be empty")
	}
	return nil
}

// ModelPreferences expresses priorities for model selection during sampling
type ModelPreferences struct {
	CostPriority         *float64    `json:"costPriority,omitempty"`
	SpeedPriority        *float64    `json:"speedPriority,omitempty"`
	IntelligencePriority *float64    `json:"intelligencePriority,omitempty"`
	Hints                []ModelHint `json:"hints,omitempty"`
}

// Validate ensures all ModelPreferences are valid
func (m *ModelPreferences) Validate() error {
	if m == nil {
		return nil
	}

	// Helper function to validate priority values
	validatePriority := func(name string, value *float64) error {
		if value != nil && (*value < 0 || *value > 1) {
			return fmt.Errorf("%s must be between 0 and 1, got %f", name, *value)
		}
		return nil
	}

	if err := validatePriority("CostPriority", m.CostPriority); err != nil {
		return err
	}
	if err := validatePriority("SpeedPriority", m.SpeedPriority); err != nil {
		return err
	}
	if err := validatePriority("IntelligencePriority", m.IntelligencePriority); err != nil {
		return err
	}

	for _, hint := range m.Hints {
		if err := hint.Validate(); err != nil {
			return fmt.Errorf("invalid model hint: %w", err)
		}
	}

	return nil
}

// Cursor represents an opaque token used for pagination
type Cursor string
