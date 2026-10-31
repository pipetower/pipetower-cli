// Package runview follows a run in real time and renders it: as a live tree in
// terminals, or as a line-oriented log when the output is redirected.
package runview

import "strings"

// maxLines is the number of complete lines a buffer keeps.
const maxLines = 5000

// escape parser states
const (
	stateText = iota
	stateEscape
	stateEscapeIntermediate
	stateCSI
	stateOSC
	stateOSCEscape
)

// LineBuffer turns raw terminal output into lines. A carriage return lets the
// following text overwrite the line (progress bars), colors (SGR) are kept and
// all other control sequences are dropped. Escape sequences may span writes.
type LineBuffer struct {
	lines   []string
	current strings.Builder
	cr      bool
	colors  string
	state   int
	params  strings.Builder
	dropped int
	emitted int
}

// Write appends raw output.
func (b *LineBuffer) Write(data string) {
	for _, r := range data {
		b.feed(r)
	}
}

// feed processes one rune according to the parser state.
func (b *LineBuffer) feed(r rune) {
	switch b.state {
	case stateEscape:
		b.escape(r)
	case stateEscapeIntermediate:
		b.escapeIntermediate(r)
	case stateCSI:
		b.csi(r)
	case stateOSC:
		b.osc(r)
	case stateOSCEscape:
		b.state = stateText
	default:
		b.text(r)
	}
}

// text handles a rune outside of escape sequences.
func (b *LineBuffer) text(r rune) {
	switch {
	case r == 0x1b:
		b.state = stateEscape
	case r == '\n':
		b.commit()
	case r == '\r':
		b.cr = true
	case r == '\t':
		b.printable("    ")
	case r == '\b' || r < 0x20 || r == 0x7f:
		// other control characters carry no visible text
	default:
		b.printable(string(r))
	}
}

// printable appends visible text, after a carriage return it replaces the line.
func (b *LineBuffer) printable(text string) {
	if b.cr {
		b.current.Reset()
		b.current.WriteString(b.colors)
		b.cr, b.colors = false, ""
	}
	b.current.WriteString(text)
}

// escape handles the rune after ESC: control sequences, string sequences (OSC, DCS, APC, PM, SOS,
// all ended by BEL or ST), sequences with intermediate bytes (e.g. ESC ( B) and two-byte sequences.
func (b *LineBuffer) escape(r rune) {
	switch {
	case r == '[':
		b.state = stateCSI
		b.params.Reset()
	case r == ']' || r == 'P' || r == '_' || r == '^' || r == 'X':
		b.state = stateOSC
	case r >= 0x20 && r <= 0x2f:
		b.state = stateEscapeIntermediate
	default:
		b.state = stateText
	}
}

// escapeIntermediate skips the intermediate bytes and the final byte of an escape sequence.
func (b *LineBuffer) escapeIntermediate(r rune) {
	if r < 0x20 || r > 0x2f {
		b.state = stateText
	}
}

// csi collects a control sequence and keeps it only if it sets colors.
func (b *LineBuffer) csi(r rune) {
	if r < 0x40 || r > 0x7e {
		b.params.WriteRune(r)
		return
	}
	b.state = stateText
	if r != 'm' {
		return
	}
	// after a carriage return the colors belong to the text which overwrites the line
	if b.cr {
		b.colors += "\x1b[" + b.params.String() + "m"
	} else {
		b.current.WriteString("\x1b[" + b.params.String() + "m")
	}
}

// osc skips an operating system command up to BEL or ST.
func (b *LineBuffer) osc(r rune) {
	switch r {
	case 0x07:
		b.state = stateText
	case 0x1b:
		b.state = stateOSCEscape
	}
}

// commit completes the current line.
func (b *LineBuffer) commit() {
	b.lines = append(b.lines, b.current.String())
	b.current.Reset()
	b.cr, b.colors = false, ""

	if len(b.lines) > maxLines {
		trim := len(b.lines) - maxLines
		b.lines = b.lines[trim:]
		b.dropped += trim
	}
}

// Partial returns the incomplete last line.
func (b *LineBuffer) Partial() string {
	return b.current.String()
}

// Tail returns the last n lines including a non-empty partial line.
func (b *LineBuffer) Tail(n int) []string {
	all := b.lines
	if partial := b.Partial(); partial != "" {
		all = append(all[:len(all):len(all)], partial)
	}
	if len(all) > n {
		all = all[len(all)-n:]
	}
	return all
}

// Lines returns all kept complete lines and the partial one.
func (b *LineBuffer) Lines() []string {
	return b.Tail(len(b.lines) + 1)
}

// Flush returns the lines completed since the last flush.
func (b *LineBuffer) Flush() []string {
	start := max(0, b.emitted-b.dropped)
	lines := b.lines[start:]
	b.emitted = b.dropped + len(b.lines)
	return lines
}

// FlushAll returns the remaining complete lines and the partial line, for a finished stream.
func (b *LineBuffer) FlushAll() []string {
	lines := b.Flush()
	if partial := b.Partial(); partial != "" {
		lines = append(lines, partial)
		b.current.Reset()
	}
	return lines
}

// Empty tells whether nothing was written.
func (b *LineBuffer) Empty() bool {
	return len(b.lines) == 0 && b.dropped == 0 && b.current.Len() == 0
}
