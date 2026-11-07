// Package definition reads definition files: the portable description of an action, a workflow
// or a template, as the export of Pipetower writes it and the API takes it.
package definition

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/pipetower/pipetower-cli/internal/fault"
)

// Kinds are the kinds of definitions.
var Kinds = []string{"action", "workflow", "template"}

// Definition is a loaded definition file.
type Definition struct {
	// Kind is the kind the file declares.
	Kind string
	// Name is the name of the described object.
	Name string
	// Data is the whole definition, ready to be sent as JSON.
	Data map[string]any
	// Source names where it came from, for messages.
	Source string
}

// Load reads a definition file, "-" reads the standard input. The file must be YAML or JSON
// (which is YAML) with a known kind and a name, like the export writes it.
func Load(path string, stdin io.Reader) (Definition, error) {
	content, source, err := read(path, stdin)
	if err != nil {
		return Definition{}, err
	}
	return Parse(content, source)
}

// Parse parses the content of a definition file.
func Parse(content []byte, source string) (Definition, error) {
	var data map[string]any
	if err := yaml.Unmarshal(content, &data); err != nil {
		return Definition{}, invalid(source, "It is not valid YAML: "+strings.TrimPrefix(err.Error(), "yaml: "))
	}
	if len(bytes.TrimSpace(content)) == 0 || data == nil {
		return Definition{}, invalid(source, "It is empty.")
	}

	kind, _ := data["kind"].(string)
	if !isKind(kind) {
		return Definition{}, invalid(source, "It must say what it is with “kind: "+strings.Join(Kinds, "|")+"”.")
	}
	name, _ := data["name"].(string)
	if name == "" {
		return Definition{}, invalid(source, "It must have a name.")
	}
	if _, err := json.Marshal(data); err != nil {
		return Definition{}, invalid(source, "It cannot be sent as JSON: "+err.Error())
	}

	return Definition{Kind: kind, Name: name, Data: data, Source: source}, nil
}

// Body is the definition as request body, with the given keys (team, dry_run) added.
func (d Definition) Body(extra map[string]any) map[string]any {
	body := make(map[string]any, len(d.Data)+len(extra))
	for key, value := range d.Data {
		body[key] = value
	}
	for key, value := range extra {
		body[key] = value
	}
	return body
}

// read returns the content of the file, or of stdin for "-", and a name for messages.
func read(path string, stdin io.Reader) ([]byte, string, error) {
	if path == "-" {
		content, err := io.ReadAll(stdin)
		if err != nil {
			return nil, "", err
		}
		return content, "The definition on stdin", nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fault.New(fault.Problem{Title: "Cannot read " + path, Detail: err.Error(), Code: fault.ExitUsage})
	}
	return content, path, nil // not trimmed: the final line break ends a literal block at the end of the file
}

// isKind tells whether the kind is known.
func isKind(kind string) bool {
	for _, known := range Kinds {
		if kind == known {
			return true
		}
	}
	return false
}

// invalid explains why the file is not a definition.
func invalid(source, detail string) error {
	return fault.New(fault.Problem{Title: fmt.Sprintf("%s is not a definition", source), Detail: detail, Code: fault.ExitUsage})
}
