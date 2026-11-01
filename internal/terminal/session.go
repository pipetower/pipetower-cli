package terminal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/pipetower/pipetower-cli/internal/api"
	"github.com/pipetower/pipetower-cli/internal/gateway"
	"github.com/pipetower/pipetower-cli/internal/ui"
)

// Reasons why attaching ended, besides the end reasons of the gateway.
const (
	ReasonDetached = "detached"
)

// Options describes how to open the session and the local terminal.
type Options struct {
	// Open opens or joins the session with its first ticket, only once the gateway is connected.
	Open func(ctx context.Context) (api.Session, error)
	// Reattach issues a fresh ticket for the session after the gateway reconnected.
	Reattach func(ctx context.Context, sessionID string) (api.Session, error)
	Connect  gateway.Connector
	In       *os.File
	Out      *os.File
}

// Result tells how the session ended for the local user.
type Result struct {
	Session api.Session
	Reason  string
}

// reattachDelays are the waits between attempts to reattach after a temporary error.
var reattachDelays = []time.Duration{time.Second, 2 * time.Second, 5 * time.Second, 10 * time.Second, 15 * time.Second, 30 * time.Second}

// client mirrors one session into the local terminal.
type client struct {
	opts     Options
	session  api.Session
	gw       *gateway.Client
	me       gateway.User
	escaper  *Escaper
	presence *presence
	attached bool
	// attachSent tells whether the first attach message reached the gateway.
	attachSent bool
	// connected is set by the first ready message, later ones mean a reconnect.
	connected bool
	// reattach fires when a failed reattach is tried again.
	reattach *time.Timer
	attempts int
}

// Attach connects to the gateway, opens the session and mirrors it into the terminal until it
// ends or the user detaches. Without a gateway no session is opened.
func Attach(ctx context.Context, opts Options) (Result, error) {
	gw, err := opts.Connect(ctx)
	if err != nil {
		return Result{}, err
	}
	defer gw.Close()

	session, err := opts.Open(ctx)
	if err != nil {
		return Result{}, err
	}

	// enabled first and restored last (deferred calls run in reverse), so that Windows consoles
	// also interpret the sequences which save and restore the terminal state
	defer ui.EnableVirtualTerminal(opts.Out)()
	restore, err := rawMode(opts.In, opts.Out)
	if err != nil {
		return Result{}, err
	}
	defer restore()

	c := &client{opts: opts, session: session, gw: gw, escaper: NewEscaper(), presence: newPresence(session), reattach: stoppedTimer()}
	if ready := gw.Ready(); ready.User != nil {
		c.me = *ready.User
	}
	// if the connection dropped meanwhile, the next ready message attaches with a fresh ticket
	c.attachSent = c.attach(session.Ticket) == nil

	result, err := c.loop(ctx)
	result.Session = session
	return result, err
}

// stoppedTimer creates a timer which fires only once it is reset.
func stoppedTimer() *time.Timer {
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	return timer
}

// loop dispatches gateway messages, keystrokes and resizes until the session is left.
func (c *client) loop(ctx context.Context) (Result, error) {
	input := readInput(c.opts.In)
	resizes, stop := watchResize(c.opts.Out)
	defer stop()

	for {
		select {
		case <-ctx.Done():
			return Result{Reason: ReasonDetached}, nil
		case <-c.reattach.C:
			if result, done, err := c.tryReattach(ctx); done {
				return result, err
			}
		case message, ok := <-c.gw.Messages():
			if !ok {
				return Result{}, errors.New("the gateway connection closed")
			}
			if result, done, err := c.receive(ctx, message); done {
				return result, err
			}
		case chunk, ok := <-input:
			if !ok {
				return Result{Reason: ReasonDetached}, nil
			}
			if result, done := c.keys(chunk); done {
				return result, nil
			}
		case <-resizes:
			c.sendResize()
		}
	}
}

// receive handles one gateway message and tells whether the session is over.
func (c *client) receive(ctx context.Context, m gateway.Message) (Result, bool, error) {
	switch m.Type {
	case gateway.TypeReady:
		if m.User != nil {
			c.me = *m.User
		}
		if !c.connected && c.attachSent {
			c.connected = true
			return Result{}, false, nil
		}
		// a reconnect (or a first attach which could not be sent): attach with a fresh ticket
		c.connected = true
		c.reattach.Stop()
		c.attempts = 0
		return c.tryReattach(ctx)
	case gateway.TypeDisconnected:
		c.title("reconnecting…")
	case gateway.TypeFailed:
		return Result{}, true, errors.New("lost the connection to the gateway: " + m.Text)
	case gateway.TypeError:
		// errors without a session, e.g. a rejected attach, end a session which never started
		if m.SessionID() == "" && !c.attached {
			return Result{}, true, errors.New(m.Text)
		}
	}
	if m.SessionID() != c.session.ID {
		return Result{}, false, nil
	}

	switch m.Type {
	case gateway.TypeAttached:
		c.attached = true
		c.write("\x1b[2J\x1b[H" + m.Snapshot)
		c.participants(m.Participants, m.Requests)
	case gateway.TypeOutput:
		c.write(m.Data)
	case gateway.TypeParticipants:
		c.participants(m.Participants, m.Requests)
	case gateway.TypeEnded:
		return Result{Reason: m.Reason}, true, nil
	case gateway.TypeError:
		if !c.attached {
			return Result{}, true, errors.New(m.Text)
		}
		c.title("error: " + m.Text)
	}
	return Result{}, false, nil
}

// keys handles local keystrokes: input for the session or escape commands.
func (c *client) keys(chunk []byte) (Result, bool) {
	for _, action := range c.escaper.Feed(chunk) {
		if action.Command == CommandNone {
			c.sendInput(action.Input)
			continue
		}
		if done := c.command(action.Command); done {
			c.send(map[string]any{"type": "detach", "session": c.session.ID})
			return Result{Reason: ReasonDetached}, true
		}
	}
	return Result{}, false
}

// command executes an escape command and tells whether to detach.
func (c *client) command(command Command) bool {
	switch command {
	case CommandDetach:
		return true
	case CommandHelp:
		c.write("\r\n" + strings.ReplaceAll(HelpText, "\n", "\r\n") + "\r\n")
	case CommandRequestControl:
		c.control("request", "")
		c.title("control requested")
	case CommandReleaseControl:
		c.control("release", "")
	case CommandEnd:
		c.send(map[string]any{"type": "end", "session": c.session.ID})
	}
	return false
}

// attach sends the attach message with the terminal size.
func (c *client) attach(ticket string) error {
	cols, rows := size(c.opts.Out)
	return c.gw.Send(map[string]any{"type": "attach", "session": c.session.ID, "ticket": ticket, "cols": cols, "rows": rows})
}

// tryReattach attaches again with a fresh ticket after the gateway reconnected. A temporary error
// is tried again later, a session which cannot be attached anymore has ended in the meantime.
func (c *client) tryReattach(ctx context.Context) (Result, bool, error) {
	session, err := c.opts.Reattach(ctx, c.session.ID)
	switch {
	case err == nil:
		c.title("")
		// if the gateway dropped again meanwhile, its next ready message tries again
		_ = c.attach(session.Ticket)
		return Result{}, false, nil
	case isGone(err):
		return Result{Reason: "gone"}, true, nil
	case api.IsTransient(err):
		c.title("reattaching…")
		c.reattach.Reset(reattachDelays[min(c.attempts, len(reattachDelays)-1)])
		c.attempts++
		return Result{}, false, nil
	default:
		return Result{}, true, err
	}
}

// isGone tells whether the session cannot be attached anymore (ended or not accessible).
func isGone(err error) bool {
	var apiErr *api.Error
	return errors.As(err, &apiErr) && (apiErr.Status == http.StatusNotFound || apiErr.Status == http.StatusForbidden)
}

// sendInput forwards keystrokes. A bell tells that they were not sent: guests without
// control and while reconnecting (the window title shows why).
func (c *client) sendInput(data []byte) {
	if !c.presence.hasControl(c.me.ID) {
		c.write("\a")
		return
	}
	if err := c.gw.Send(map[string]any{"type": "input", "session": c.session.ID, "data": string(data)}); err != nil {
		c.write("\a")
	}
}

// sendResize tells the session about the new terminal size.
func (c *client) sendResize() {
	cols, rows := size(c.opts.Out)
	c.send(map[string]any{"type": "resize", "session": c.session.ID, "cols": cols, "rows": rows})
}

// control sends a control action.
func (c *client) control(action, user string) {
	c.send(map[string]any{"type": "control", "session": c.session.ID, "action": action, "user": user})
}

// participants updates the presence and shows what changed in the title.
func (c *client) participants(participants []gateway.Participant, requests []string) {
	notice := c.presence.update(c.me.ID, participants, requests)
	c.title(notice)
}

// title shows the session state and a notice in the window title.
func (c *client) title(notice string) {
	text := c.presence.describe(c.me.ID)
	if notice != "" {
		text += " · " + notice
	}
	c.write("\x1b]0;" + text + "\x07")
}

// send writes a message to the gateway, failures surface as reconnects.
func (c *client) send(message map[string]any) {
	_ = c.gw.Send(message)
}

// write writes to the local terminal.
func (c *client) write(text string) {
	_, _ = io.WriteString(c.opts.Out, text)
}

// resetModes turns off what a full-screen program in the session may have turned on: the alternate
// screen, mouse tracking, bracketed paste and application cursor and keypad keys. Then colors,
// the cursor and the saved window title are restored.
const resetModes = "\x1b[?1049l\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l\x1b[?2004l\x1b[?1l\x1b>" + "\x1b[0m\x1b[?25h\x1b[23;0t"

// rawMode switches the terminal to raw mode and saves the window title.
func rawMode(in, out *os.File) (func(), error) {
	state, err := term.MakeRaw(int(in.Fd()))
	if err != nil {
		return nil, fmt.Errorf("the terminal cannot be switched to raw mode: %w", err)
	}
	_, _ = io.WriteString(out, "\x1b[22;0t")

	return func() {
		_, _ = io.WriteString(out, resetModes)
		_ = term.Restore(int(in.Fd()), state)
	}, nil
}

// readInput forwards stdin chunks until it fails.
func readInput(in io.Reader) <-chan []byte {
	chunks := make(chan []byte)
	go func() {
		defer close(chunks)
		buffer := make([]byte, 4096)
		for {
			n, err := in.Read(buffer)
			if n > 0 {
				chunks <- append([]byte(nil), buffer[:n]...)
			}
			if err != nil {
				return
			}
		}
	}()
	return chunks
}

// size returns the terminal size with a fallback.
func size(out *os.File) (int, int) {
	cols, rows, err := term.GetSize(int(out.Fd()))
	if err != nil || cols <= 0 || rows <= 0 {
		return 120, 30
	}
	return cols, rows
}
