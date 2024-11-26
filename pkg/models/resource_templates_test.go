package models

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestListResourceTemplatesRequestSerialization(t *testing.T) {
	cursor := Cursor("test-cursor")
	req := ListResourceTemplatesRequest{
		Method: "resources/templates/list",
		Params: &ListResourceTemplatesParams{
			Cursor: &cursor,
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal ListResourceTemplatesRequest: %v", err)
	}

	var decoded ListResourceTemplatesRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal ListResourceTemplatesRequest: %v", err)
	}

	if diff := cmp.Diff(req, decoded); diff != "" {
		t.Errorf("ListResourceTemplatesRequest mismatch (-want +got):\n%s", diff)
	}
}

func TestListResourceTemplatesWithoutCursor(t *testing.T) {
	req := ListResourceTemplatesRequest{
		Method: "resources/templates/list",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal request without cursor: %v", err)
	}

	expected := `{"method":"resources/templates/list"}`
	if string(data) != expected {
		t.Errorf("Unexpected JSON output:\nGot:  %s\nWant: %s", string(data), expected)
	}
}

func TestListResourceTemplatesResultSerialization(t *testing.T) {
	cursor := Cursor("next-page")
	result := ListResourceTemplatesResult{
		ResourceTemplates: []ResourceTemplate{
			{
				Name:        "template1",
				URITemplate: "test://{param1}/{param2}",
				Description: "Test template 1",
				MimeType:    "application/json",
			},
			{
				Name:        "template2",
				URITemplate: "test://{param3}",
				Description: "Test template 2",
				MimeType:    "text/plain",
			},
		},
		NextCursor: &cursor,
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Failed to marshal ListResourceTemplatesResult: %v", err)
	}

	var decoded ListResourceTemplatesResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal ListResourceTemplatesResult: %v", err)
	}

	if diff := cmp.Diff(result, decoded); diff != "" {
		t.Errorf("ListResourceTemplatesResult mismatch (-want +got):\n%s", diff)
	}
}

func TestListResourceTemplatesResultPagination(t *testing.T) {
	result := ListResourceTemplatesResult{
		ResourceTemplates: []ResourceTemplate{
			{
				Name:        "template1",
				URITemplate: "test://{param}",
			},
		},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Failed to marshal result: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if _, exists := m["nextCursor"]; exists {
		t.Error("nextCursor field should not be present when nil")
	}
}
