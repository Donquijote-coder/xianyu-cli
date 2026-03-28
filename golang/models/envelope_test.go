package models

import (
	"encoding/json"
	"testing"
)

func TestOKEnvelope(t *testing.T) {
	env := OK(map[string]interface{}{"key": "value"})
	if !env.OK {
		t.Error("expected ok=true")
	}
	if env.Error != "" {
		t.Error("expected no error")
	}
}

func TestFailEnvelope(t *testing.T) {
	env := Fail("something went wrong")
	if env.OK {
		t.Error("expected ok=false")
	}
	if env.Error != "something went wrong" {
		t.Errorf("unexpected error: %s", env.Error)
	}
}

func TestEnvelopeToJSON(t *testing.T) {
	env := OK(map[string]interface{}{"items": []int{1, 2, 3}})
	j := env.ToJSON()
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(j), &parsed); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if parsed["ok"] != true {
		t.Error("expected ok=true in JSON")
	}
	if parsed["schema_version"] != "1.0.0" {
		t.Errorf("unexpected schema_version: %v", parsed["schema_version"])
	}
}

func TestFailEnvelopeToDict(t *testing.T) {
	env := Fail("error")
	j := env.ToJSON()
	var parsed map[string]interface{}
	json.Unmarshal([]byte(j), &parsed)
	if parsed["ok"] != false {
		t.Error("expected ok=false")
	}
	if parsed["error"] != "error" {
		t.Errorf("unexpected error: %v", parsed["error"])
	}
	if _, ok := parsed["schema_version"]; !ok {
		t.Error("missing schema_version")
	}
}
