package ui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/charmbracelet/colorprofile"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"
)

// IO bundles the streams of the CLI and what the terminal can do.
type IO struct {
	In      *os.File
	Stdout  *os.File
	Out     io.Writer
	Err     io.Writer
	RawOut  io.Writer
	Profile colorprofile.Profile
	OutTTY  bool
	ErrTTY  bool
	// Terminal tells whether input and output are a terminal (live views, terminal sessions).
	Terminal bool
	// CanPrompt tells whether the user may be asked: in a terminal and without --no-input.
	CanPrompt bool
}

// NewIO detects the terminal capabilities of the process streams.
func NewIO(noColor, noInput bool) *IO {
	out := colorprofile.NewWriter(os.Stdout, os.Environ())
	errOut := colorprofile.NewWriter(os.Stderr, os.Environ())
	if noColor {
		out.Profile, errOut.Profile = colorprofile.ASCII, colorprofile.ASCII
	}

	stdinTTY := term.IsTerminal(int(os.Stdin.Fd()))
	stdoutTTY := term.IsTerminal(int(os.Stdout.Fd()))
	terminal := stdinTTY && stdoutTTY

	return &IO{
		In:        os.Stdin,
		Stdout:    os.Stdout,
		Out:       out,
		Err:       errOut,
		RawOut:    os.Stdout,
		Profile:   out.Profile,
		OutTTY:    stdoutTTY,
		ErrTTY:    term.IsTerminal(int(os.Stderr.Fd())),
		Terminal:  terminal,
		CanPrompt: terminal && !noInput,
	}
}

// Println writes a line to stdout.
func (o *IO) Println(text ...any) {
	_, _ = fmt.Fprintln(o.Out, text...)
}

// Errorln writes a line to stderr.
func (o *IO) Errorln(text ...any) {
	_, _ = fmt.Fprintln(o.Err, text...)
}

// Width returns the terminal width, or a sensible default.
func (o *IO) Width() int {
	if width, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && width > 0 {
		return width
	}
	return 100
}

// PrintJSON pretty-prints raw JSON, keeping the key order of the API.
func (o *IO) PrintJSON(raw []byte) error {
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, raw, "", "  "); err != nil {
		pretty.Reset()
		pretty.Write(raw)
	}
	pretty.WriteByte('\n')
	_, err := o.RawOut.Write(pretty.Bytes())
	return err
}

// PrintYAML converts raw JSON to YAML, keeping the key order.
func (o *IO) PrintYAML(raw []byte) error {
	var node yaml.Node
	if err := yaml.Unmarshal(raw, &node); err != nil {
		return err
	}
	blockStyle(&node)

	encoder := yaml.NewEncoder(o.RawOut)
	encoder.SetIndent(2)
	defer encoder.Close()
	return encoder.Encode(&node)
}

// blockStyle turns the flow style of parsed JSON into readable block style.
func blockStyle(node *yaml.Node) {
	if node.Kind != yaml.ScalarNode {
		node.Style = 0
	} else if node.Style == yaml.DoubleQuotedStyle {
		node.Style = 0
	}
	for _, child := range node.Content {
		blockStyle(child)
	}
}
