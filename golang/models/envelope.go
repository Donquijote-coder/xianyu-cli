package models

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/fatih/color"
)

const SchemaVersion = "1.0.0"

// Envelope is the unified output wrapper for all CLI commands.
type Envelope struct {
	OK            bool        `json:"ok"`
	Data          interface{} `json:"data,omitempty"`
	Error         string      `json:"error,omitempty"`
	SchemaVersion string      `json:"schema_version"`
}

// OK creates a success envelope.
func OK(data interface{}) *Envelope {
	return &Envelope{OK: true, Data: data, SchemaVersion: SchemaVersion}
}

// Fail creates an error envelope.
func Fail(errMsg string) *Envelope {
	return &Envelope{OK: false, Error: errMsg, SchemaVersion: SchemaVersion}
}

// ToJSON serializes the envelope to JSON.
func (e *Envelope) ToJSON() string {
	data, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"ok":false,"error":"json marshal error: %s","schema_version":"%s"}`, err.Error(), SchemaVersion)
	}
	return string(data)
}

// Emit outputs the envelope in the specified format.
// JSON goes to stdout, Rich goes to stderr.
func (e *Envelope) Emit(outputMode string) {
	if outputMode == "json" {
		fmt.Fprintln(os.Stdout, e.ToJSON())
	} else {
		if e.OK {
			e.emitRichSuccess()
		} else {
			e.emitRichError()
		}
	}
}

func (e *Envelope) emitRichSuccess() {
	if e.Data == nil {
		fmt.Fprintln(os.Stderr, color.GreenString("OK"))
		return
	}
	switch v := e.Data.(type) {
	case string:
		fmt.Fprintln(os.Stderr, v)
	case map[string]interface{}:
		if msg, ok := v["message"]; ok {
			fmt.Fprintln(os.Stderr, color.GreenString("✓")+" "+fmt.Sprint(msg))
		} else {
			data, _ := json.MarshalIndent(v, "", "  ")
			fmt.Fprintln(os.Stderr, string(data))
		}
	default:
		data, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			fmt.Fprintln(os.Stderr, fmt.Sprint(v))
		} else {
			fmt.Fprintln(os.Stderr, string(data))
		}
	}
}

func (e *Envelope) emitRichError() {
	red := color.New(color.FgRed, color.Bold)
	fmt.Fprintf(os.Stderr, "\n%s %s\n\n", red.Sprint("Error:"), e.Error)
}
