package models

import (
	"testing"
)

func TestRole(t *testing.T) {
	tests := []struct {
		name string
		role Role
		want bool
	}{
		{"Valid assistant", RoleAssistant, true},
		{"Valid user", RoleUser, true},
		{"Invalid role", Role("invalid"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.role.IsValid(); got != tt.want {
				t.Errorf("Role.IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestImplementation_Validate(t *testing.T) {
	tests := []struct {
		name    string
		impl    Implementation
		wantErr bool
	}{
		{
			name:    "Valid implementation",
			impl:    Implementation{Name: "test", Version: "1.0.0"},
			wantErr: false,
		},
		{
			name:    "Missing name",
			impl:    Implementation{Version: "1.0.0"},
			wantErr: true,
		},
		{
			name:    "Missing version",
			impl:    Implementation{Name: "test"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.impl.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Implementation.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestAnnotations_Validate(t *testing.T) {
	validPriority := 0.5
	invalidPriority := 1.5

	tests := []struct {
		name    string
		ann     *Annotations
		wantErr bool
	}{
		{
			name: "Valid annotations",
			ann: &Annotations{
				Audience: []Role{RoleAssistant, RoleUser},
				Priority: &validPriority,
			},
			wantErr: false,
		},
		{
			name: "Invalid role",
			ann: &Annotations{
				Audience: []Role{Role("invalid")},
			},
			wantErr: true,
		},
		{
			name: "Invalid priority",
			ann: &Annotations{
				Priority: &invalidPriority,
			},
			wantErr: true,
		},
		{
			name:    "Nil annotations",
			ann:     nil,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.ann.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Annotations.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBaseAnnotated(t *testing.T) {
	// Test GetAnnotations and SetAnnotations
	base := &BaseAnnotated{}
	
	// Test initial state (nil annotations)
	if annotations := base.GetAnnotations(); annotations != nil {
		t.Errorf("Expected nil annotations initially, got %v", annotations)
	}
	
	// Test setting annotations
	testAnnotations := &Annotations{
		Audience: []Role{RoleAssistant},
		Priority: func() *float64 { p := 0.5; return &p }(),
	}
	base.SetAnnotations(testAnnotations)
	
	// Test getting annotations
	retrieved := base.GetAnnotations()
	if retrieved == nil {
		t.Fatal("Expected annotations to be set, got nil")
	}
	if len(retrieved.Audience) != 1 || retrieved.Audience[0] != RoleAssistant {
		t.Errorf("Wrong audience: got %v, want [%s]", retrieved.Audience, RoleAssistant)
	}
	if retrieved.Priority == nil || *retrieved.Priority != 0.5 {
		t.Errorf("Wrong priority: got %v, want 0.5", retrieved.Priority)
	}
	
	// Test setting nil annotations
	base.SetAnnotations(nil)
	if annotations := base.GetAnnotations(); annotations != nil {
		t.Errorf("Expected nil annotations after setting nil, got %v", annotations)
	}
}

func TestLogLevel(t *testing.T) {
	tests := []struct {
		name    string
		level   LogLevel
		isValid bool
		str     string
	}{
		{"Emergency", LogLevelEmergency, true, "emergency"},
		{"Alert", LogLevelAlert, true, "alert"},
		{"Critical", LogLevelCritical, true, "critical"},
		{"Error", LogLevelError, true, "error"},
		{"Warning", LogLevelWarning, true, "warning"},
		{"Notice", LogLevelNotice, true, "notice"},
		{"Info", LogLevelInfo, true, "info"},
		{"Debug", LogLevelDebug, true, "debug"},
		{"Invalid", LogLevel("invalid"), false, "invalid"},
		{"Empty", LogLevel(""), false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test IsValid
			if got := tt.level.IsValid(); got != tt.isValid {
				t.Errorf("LogLevel.IsValid() = %v, want %v", got, tt.isValid)
			}
			
			// Test String
			if got := tt.level.String(); got != tt.str {
				t.Errorf("LogLevel.String() = %v, want %v", got, tt.str)
			}
		})
	}
}

func TestModelPreferences_Validate(t *testing.T) {
	validPriority := 0.5
	invalidPriority := 1.5
	invalidLowPriority := -0.1

	tests := []struct {
		name    string
		mp      *ModelPreferences
		wantErr bool
	}{
		{
			name: "Valid preferences",
			mp: &ModelPreferences{
				CostPriority:         &validPriority,
				SpeedPriority:        &validPriority,
				IntelligencePriority: &validPriority,
				Hints:                []ModelHint{{Name: "test"}},
			},
			wantErr: false,
		},
		{
			name: "Invalid cost priority",
			mp: &ModelPreferences{
				CostPriority: &invalidPriority,
			},
			wantErr: true,
		},
		{
			name: "Invalid speed priority",
			mp: &ModelPreferences{
				SpeedPriority: &invalidPriority,
			},
			wantErr: true,
		},
		{
			name: "Invalid intelligence priority",
			mp: &ModelPreferences{
				IntelligencePriority: &invalidPriority,
			},
			wantErr: true,
		},
		{
			name: "Invalid low cost priority",
			mp: &ModelPreferences{
				CostPriority: &invalidLowPriority,
			},
			wantErr: true,
		},
		{
			name: "Invalid low speed priority",
			mp: &ModelPreferences{
				SpeedPriority: &invalidLowPriority,
			},
			wantErr: true,
		},
		{
			name: "Invalid low intelligence priority",
			mp: &ModelPreferences{
				IntelligencePriority: &invalidLowPriority,
			},
			wantErr: true,
		},
		{
			name: "Invalid hint",
			mp: &ModelPreferences{
				Hints: []ModelHint{{Name: ""}},
			},
			wantErr: true,
		},
		{
			name:    "Nil preferences",
			mp:      nil,
			wantErr: false,
		},
		{
			name: "Edge case priorities (exactly 0 and 1)",
			mp: &ModelPreferences{
				CostPriority:         func() *float64 { p := 0.0; return &p }(),
				SpeedPriority:        func() *float64 { p := 1.0; return &p }(),
				IntelligencePriority: func() *float64 { p := 0.5; return &p }(),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.mp.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("ModelPreferences.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
