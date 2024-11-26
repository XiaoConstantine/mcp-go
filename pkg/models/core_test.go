package models

import (
	"encoding/json"
	"testing"
)

func TestRole(t *testing.T) {
	// Test both defined roles marshal and unmarshal correctly
	roles := []Role{RoleAssistant, RoleUser}
	expectedStrings := []string{"\"assistant\"", "\"user\""}

	for i, role := range roles {
		// Test marshaling
		data, err := json.Marshal(role)
		if err != nil {
			t.Errorf("Failed to marshal Role %v: %v", role, err)
		}
		if string(data) != expectedStrings[i] {
			t.Errorf("Role marshaled incorrectly. Got %s, want %s", string(data), expectedStrings[i])
		}

		// Test unmarshaling
		var unmarshaled Role
		if err := json.Unmarshal(data, &unmarshaled); err != nil {
			t.Errorf("Failed to unmarshal Role %v: %v", role, err)
		}
		if unmarshaled != role {
			t.Errorf("Role unmarshaled incorrectly. Got %v, want %v", unmarshaled, role)
		}
	}
}

func TestImplementation(t *testing.T) {
	impl := Implementation{
		Name:    "test-implementation",
		Version: "1.0.0",
	}

	// Test marshaling
	data, err := json.Marshal(impl)
	if err != nil {
		t.Fatalf("Failed to marshal Implementation: %v", err)
	}

	// Test unmarshaling
	var decoded Implementation
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal Implementation: %v", err)
	}

	// Verify fields
	if decoded.Name != impl.Name {
		t.Errorf("Name field mismatch. Got %s, want %s", decoded.Name, impl.Name)
	}
	if decoded.Version != impl.Version {
		t.Errorf("Version field mismatch. Got %s, want %s", decoded.Version, impl.Version)
	}
}

func TestAnnotations(t *testing.T) {
	priority := 0.8
	annotations := Annotations{
		Audience: []Role{RoleAssistant, RoleUser},
		Priority: &priority,
	}

	// Test marshaling
	data, err := json.Marshal(annotations)
	if err != nil {
		t.Fatalf("Failed to marshal Annotations: %v", err)
	}

	// Test unmarshaling
	var decoded Annotations
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal Annotations: %v", err)
	}

	// Verify fields
	if len(decoded.Audience) != len(annotations.Audience) {
		t.Errorf("Audience length mismatch. Got %d, want %d", len(decoded.Audience), len(annotations.Audience))
	}
	if *decoded.Priority != *annotations.Priority {
		t.Errorf("Priority mismatch. Got %f, want %f", *decoded.Priority, *annotations.Priority)
	}
}

func TestLoggingLevel(t *testing.T) {
	levels := []LoggingLevel{
		LoggingLevelEmergency,
		LoggingLevelAlert,
		LoggingLevelCritical,
		LoggingLevelError,
		LoggingLevelWarning,
		LoggingLevelNotice,
		LoggingLevelInfo,
		LoggingLevelDebug,
	}

	expectedStrings := []string{
		"\"emergency\"",
		"\"alert\"",
		"\"critical\"",
		"\"error\"",
		"\"warning\"",
		"\"notice\"",
		"\"info\"",
		"\"debug\"",
	}

	for i, level := range levels {
		// Test marshaling
		data, err := json.Marshal(level)
		if err != nil {
			t.Errorf("Failed to marshal LoggingLevel %v: %v", level, err)
		}
		if string(data) != expectedStrings[i] {
			t.Errorf("LoggingLevel marshaled incorrectly. Got %s, want %s", string(data), expectedStrings[i])
		}

		// Test unmarshaling
		var unmarshaled LoggingLevel
		if err := json.Unmarshal(data, &unmarshaled); err != nil {
			t.Errorf("Failed to unmarshal LoggingLevel %v: %v", level, err)
		}
		if unmarshaled != level {
			t.Errorf("LoggingLevel unmarshaled incorrectly. Got %v, want %v", unmarshaled, level)
		}
	}
}

func TestModelPreferences(t *testing.T) {
	costPriority := 0.7
	speedPriority := 0.8
	intelligencePriority := 0.9

	prefs := ModelPreferences{
		CostPriority:         &costPriority,
		SpeedPriority:        &speedPriority,
		IntelligencePriority: &intelligencePriority,
		Hints: []ModelHint{
			{Name: "model1"},
			{Name: "model2"},
		},
	}

	// Test marshaling
	data, err := json.Marshal(prefs)
	if err != nil {
		t.Fatalf("Failed to marshal ModelPreferences: %v", err)
	}

	// Test unmarshaling
	var decoded ModelPreferences
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal ModelPreferences: %v", err)
	}

	// Verify fields
	if *decoded.CostPriority != costPriority {
		t.Errorf("CostPriority mismatch. Got %f, want %f", *decoded.CostPriority, costPriority)
	}
	if *decoded.SpeedPriority != speedPriority {
		t.Errorf("SpeedPriority mismatch. Got %f, want %f", *decoded.SpeedPriority, speedPriority)
	}
	if *decoded.IntelligencePriority != intelligencePriority {
		t.Errorf("IntelligencePriority mismatch. Got %f, want %f", *decoded.IntelligencePriority, intelligencePriority)
	}
	if len(decoded.Hints) != len(prefs.Hints) {
		t.Errorf("Hints length mismatch. Got %d, want %d", len(decoded.Hints), len(prefs.Hints))
	}
}
