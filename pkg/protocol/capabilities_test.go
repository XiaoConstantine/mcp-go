package protocol

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestClientCapabilities(t *testing.T) {
	// Create a comprehensive test case that covers all capability fields
	capabilities := ClientCapabilities{
		Experimental: map[string]map[string]interface{}{
			"customFeature": {
				"enabled": true,
				"config":  "test",
			},
		},
		Roots: &RootsCapability{
			ListChanged: true,
		},
		Sampling: map[string]interface{}{
			"maxTokens": 1000,
			"models":    []string{"model1", "model2"},
		},
	}

	// Test marshaling to JSON
	data, err := json.Marshal(capabilities)
	if err != nil {
		t.Fatalf("Failed to marshal ClientCapabilities: %v", err)
	}

	// Test unmarshaling from JSON
	var decoded ClientCapabilities
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal ClientCapabilities: %v", err)
	}

	// Verify all fields were preserved
	if !reflect.DeepEqual(capabilities, decoded) {
		t.Errorf("Decoded capabilities don't match original:\nGot: %+v\nWant: %+v", decoded, capabilities)
	}
}

func TestServerCapabilities(t *testing.T) {
	// Create a comprehensive test case that covers all server capability fields
	capabilities := ServerCapabilities{
		Experimental: map[string]map[string]interface{}{
			"customFeature": {
				"enabled": true,
				"config":  "test",
			},
		},
		Logging: map[string]interface{}{
			"level": "debug",
		},
		Prompts: &PromptsCapability{
			ListChanged: true,
		},
		Resources: &ResourcesCapability{
			ListChanged: true,
			Subscribe:   true,
		},
		Tools: &ToolsCapability{
			ListChanged: true,
		},
	}

	// Test marshaling to JSON
	data, err := json.Marshal(capabilities)
	if err != nil {
		t.Fatalf("Failed to marshal ServerCapabilities: %v", err)
	}

	// Test unmarshaling from JSON
	var decoded ServerCapabilities
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal ServerCapabilities: %v", err)
	}

	// Verify all fields were preserved
	if !reflect.DeepEqual(capabilities, decoded) {
		t.Errorf("Decoded capabilities don't match original:\nGot: %+v\nWant: %+v", decoded, capabilities)
	}
}

func TestCapabilitiesOmitempty(t *testing.T) {
	// Test that empty fields are properly omitted from JSON
	emptyCapabilities := ServerCapabilities{}

	data, err := json.Marshal(emptyCapabilities)
	if err != nil {
		t.Fatalf("Failed to marshal empty ServerCapabilities: %v", err)
	}

	// The resulting JSON should be an empty object
	if string(data) != "{}" {
		t.Errorf("Empty capabilities didn't marshal to empty object. Got: %s", string(data))
	}
}