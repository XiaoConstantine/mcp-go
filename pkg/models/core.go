package models

// Role represents the sender or recipient in a conversation
type Role string

const (
	RoleAssistant Role = "assistant"
	RoleUser      Role = "user"
)

// Implementation describes the name and version of an MCP implementation
type Implementation struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Annotated is a base for objects that include optional annotations
type Annotated struct {
	Annotations *Annotations `json:"annotations,omitempty"`
}

// Annotations provides metadata about how objects should be used or displayed
type Annotations struct {
	Audience []Role   `json:"audience,omitempty"`
	Priority *float64 `json:"priority,omitempty"`
}

// Cursor represents an opaque token used for pagination
type Cursor string

// LoggingLevel represents the severity of a log message
type LoggingLevel string

const (
	LoggingLevelEmergency LoggingLevel = "emergency"
	LoggingLevelAlert     LoggingLevel = "alert"
	LoggingLevelCritical  LoggingLevel = "critical"
	LoggingLevelError     LoggingLevel = "error"
	LoggingLevelWarning   LoggingLevel = "warning"
	LoggingLevelNotice    LoggingLevel = "notice"
	LoggingLevelInfo      LoggingLevel = "info"
	LoggingLevelDebug     LoggingLevel = "debug"
)

// ModelHint provides hints for model selection
type ModelHint struct {
	Name string `json:"name,omitempty"`
}

// ModelPreferences expresses priorities for model selection during sampling
type ModelPreferences struct {
	CostPriority         *float64    `json:"costPriority,omitempty"`
	SpeedPriority        *float64    `json:"speedPriority,omitempty"`
	IntelligencePriority *float64    `json:"intelligencePriority,omitempty"`
	Hints                []ModelHint `json:"hints,omitempty"`
}
