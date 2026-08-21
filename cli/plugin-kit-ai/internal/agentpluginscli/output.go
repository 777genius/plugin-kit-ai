package agentpluginscli

import (
	"encoding/json"
	"fmt"
	"io"
)

const outputSchemaVersion = 1

const (
	outputResultSuccess = "success"
	outputResultFailure = "failure"
)

type outputResultProvider interface {
	outputResult() string
}

type outputEnvelope struct {
	SchemaVersion int    `json:"schema_version"`
	Command       string `json:"command"`
	Result        string `json:"result"`
	Data          any    `json:"data"`
}

func writeJSONOutput(writer io.Writer, command string, data any) error {
	result := outputResultSuccess
	if provider, ok := data.(outputResultProvider); ok {
		if provided := provider.outputResult(); provided != "" {
			result = provided
		}
	}
	return writeJSONResult(writer, command, result, data)
}

func writeJSONResult(writer io.Writer, command, result string, data any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(outputEnvelope{SchemaVersion: outputSchemaVersion, Command: command, Result: result, Data: data})
}

func writeProgress(app App, _ string, message string) {
	if message == "" {
		return
	}
	_, _ = fmt.Fprintln(app.errorOutput(), message)
}
