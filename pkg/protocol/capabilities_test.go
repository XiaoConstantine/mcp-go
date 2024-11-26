package protocol

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// SamplingConfig provides strong typing for sampling configuration
type SamplingConfig struct {
	MaxTokens int      `json:"maxTokens"`
	Models    []string `json:"models"`
}

func TestClientCapabilities(t *testing.T) {
	sampling := SamplingConfig{
		MaxTokens: 1000,
		Models:    []string{"model1", "model2"},
	}
	samplingJSON, err := json.Marshal(sampling)
	if err != nil {
		t.Fatalf("Failed to marshal sampling config: %v", err)
	}
	var samplingMap map[string]interface{}
	if err := json.Unmarshal(samplingJSON, &samplingMap); err != nil {
		t.Fatalf("Failed to unmarshal sampling config: %v", err)
	}
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
		Sampling: samplingMap,
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

	// Use go-cmp to compare the structs. This will automatically handle pointer
	// comparisons and provide a detailed diff if values don't match
	if diff := cmp.Diff(capabilities, decoded); diff != "" {
		t.Errorf("Capabilities mismatch (-want +got):\n%s", diff)
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

	// Compare using go-cmp, which will show exactly what differs if the test fails
	if diff := cmp.Diff(capabilities, decoded); diff != "" {
		t.Errorf("Capabilities mismatch (-want +got):\n%s", diff)
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
