// Package terminal attaches the local terminal to a shared Pipetower terminal session.
package terminal

// Command is a local action triggered by an escape sequence.
type Command int

// Escape commands, typed as Enter ~ <key> like in ssh.
const (
	CommandNone Command = iota
	CommandDetach
	CommandHelp
	CommandRequestControl
	CommandReleaseControl
	CommandEnd
)

const escapeChar = '~'

var escapeKeys = map[byte]Command{
	'.': CommandDetach,
	'?': CommandHelp,
	'c': CommandRequestControl,
	'r': CommandReleaseControl,
	'e': CommandEnd,
}

// Escaper separates escape sequences from the keystrokes sent to the session.
// The escape character is only recognized at the beginning of a line.
type Escaper struct {
	lineStart bool
	pending   bool
}

// NewEscaper creates an escaper; the first keystroke counts as the start of a line.
func NewEscaper() *Escaper {
	return &Escaper{lineStart: true}
}

// Action is either input for the session or a command.
type Action struct {
	Input   []byte
	Command Command
}

// Feed splits a chunk of keystrokes into input and commands, in order.
func (e *Escaper) Feed(chunk []byte) []Action {
	var actions []Action
	var input []byte
	flush := func() {
		if len(input) > 0 {
			actions = append(actions, Action{Input: input})
			input = nil
		}
	}

	for _, b := range chunk {
		send, command := e.key(b)
		input = append(input, send...)
		if command != CommandNone {
			flush()
			actions = append(actions, Action{Command: command})
		}
	}
	flush()

	return actions
}

// key processes one byte and returns the bytes to send and a possible command.
func (e *Escaper) key(b byte) ([]byte, Command) {
	if e.pending {
		e.pending = false
		if command, ok := escapeKeys[b]; ok {
			return nil, command
		}
		e.lineStart = isLineEnd(b)
		if b == escapeChar {
			return []byte{escapeChar}, CommandNone
		}
		return []byte{escapeChar, b}, CommandNone
	}

	if e.lineStart && b == escapeChar {
		e.pending = true
		return nil, CommandNone
	}

	e.lineStart = isLineEnd(b)
	return []byte{b}, CommandNone
}

// isLineEnd tells whether the key ends a line.
func isLineEnd(b byte) bool {
	return b == '\r' || b == '\n'
}

// HelpText lists the escape sequences.
const HelpText = `Supported escape sequences (type them after Enter):
  ~.  detach (the session keeps running)
  ~c  request control (guests)
  ~r  release control (guests)
  ~e  end the session for everybody (owner)
  ~?  this help
  ~~  send a literal ~`
