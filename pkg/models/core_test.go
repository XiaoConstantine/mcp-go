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
			name: "Valid implementation",
			impl: Implementation{Name: "test", Version: "1.0.0"},
			wantErr: false,
		},
		{
			name: "Missing name",
			impl: Implementation{Version: "1.0.0"},
			wantErr: true,
		},
		{
			name: "Missing version",
			impl: Implementation{Name: "test"},
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
			name: "Nil annotations",
			ann:  nil,
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

func TestModelPreferences_Validate(t *testing.T) {
	validPriority := 0.5
	invalidPriority := 1.5

	tests := []struct {
		name    string
		mp      *ModelPreferences
		wantErr bool
	}{
		{
			name: "Valid preferences",
			mp: &ModelPreferences{
				CostPriority: &validPriority,
				SpeedPriority: &validPriority,
				IntelligencePriority: &validPriority,
				Hints: []ModelHint{{Name: "test"}},
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
			name: "Invalid hint",
			mp: &ModelPreferences{
				Hints: []ModelHint{{Name: ""}},
			},
			wantErr: true,
		},
		{
			name: "Nil preferences",
			mp:   nil,
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
