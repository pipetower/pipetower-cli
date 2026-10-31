package serverview

import (
	"context"
	"io"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"

	"github.com/pipetower/pipetower-cli/internal/api"
	"github.com/pipetower/pipetower-cli/internal/debounce"
	"github.com/pipetower/pipetower-cli/internal/gateway"
	"github.com/pipetower/pipetower-cli/internal/ui"
)

const (
	refreshDelay   = 300 * time.Millisecond
	refreshMaxWait = 2 * time.Second
	pollInterval   = 15 * time.Second
)

// Snapshot is the data shown on the dashboard.
type Snapshot struct {
	Server api.Server
	Runs   []api.Run
}

// Fetcher loads a fresh snapshot.
type Fetcher func(ctx context.Context) (Snapshot, error)

type (
	refreshMsg  struct{}
	snapshotMsg struct {
		snapshot Snapshot
		err      error
		seq      int
	}
	liveMsg  struct{ state string }
	clockMsg time.Time
)

// watchModel is the live dashboard.
type watchModel struct {
	ctx      context.Context
	fetch    Fetcher
	snapshot Snapshot
	err      error
	state    string
	now      time.Time
	// requested and shown number the fetches, so that a slow older fetch never replaces a newer one
	requested int
	shown     int
}

// Watch shows the dashboard and refreshes it whenever the gateway reports a change.
func Watch(ctx context.Context, initial Snapshot, fetch Fetcher, connect gateway.Connector, out io.Writer, profile colorprofile.Profile) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	m := &watchModel{ctx: ctx, fetch: fetch, snapshot: initial, state: "connecting", now: time.Now()}
	program := tea.NewProgram(m, tea.WithContext(ctx), tea.WithOutput(out), tea.WithColorProfile(profile))

	go listen(ctx, initial.Server.ID, connect, func(msg tea.Msg) { program.Send(msg) })

	_, err := program.Run()
	if ctx.Err() != nil {
		return nil
	}
	return err
}

// listen turns gateway changes of the server and its runs into debounced refreshes,
// it falls back to polling without a gateway.
func listen(ctx context.Context, serverID string, connect gateway.Connector, send func(tea.Msg)) {
	client, err := connect(ctx)
	if err != nil {
		send(liveMsg{"polling"})
		poll(ctx, send)
		return
	}
	defer client.Close()

	// the server topic also announces the changes of runs on the server
	_ = client.Subscribe(gateway.ServerTopic(serverID))
	send(liveMsg{"live"})

	refresh := debounce.New(refreshDelay, refreshMaxWait)
	for {
		select {
		case <-ctx.Done():
			return
		case <-refresh.C():
			refresh.Fired()
			send(refreshMsg{})
		case message, ok := <-client.Messages():
			if !ok {
				return
			}
			if failed := handleMessage(message, refresh, send); failed {
				poll(ctx, send)
				return
			}
		}
	}
}

// handleMessage turns a gateway message into a refresh or a connection state, it tells whether the gateway gave up.
func handleMessage(message gateway.Message, refresh *debounce.Debounce, send func(tea.Msg)) bool {
	switch message.Type {
	case gateway.TypeChanged:
		refresh.Trigger()
	case gateway.TypeReady:
		send(liveMsg{"live"})
		refresh.Trigger()
	case gateway.TypeDisconnected:
		send(liveMsg{"reconnecting"})
	case gateway.TypeFailed:
		send(liveMsg{"polling"})
		return true
	}
	return false
}

// poll requests a refresh in intervals.
func poll(ctx context.Context, send func(tea.Msg)) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			send(refreshMsg{})
		}
	}
}

// Init starts the clock which keeps relative times fresh.
func (m *watchModel) Init() tea.Cmd {
	return tick()
}

// Update handles refreshes, fetched data, the clock and keys.
func (m *watchModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case refreshMsg:
		m.requested++
		return m, m.load(m.requested)
	case snapshotMsg:
		if msg.seq < m.shown {
			return m, nil
		}
		m.shown, m.err = msg.seq, msg.err
		if msg.err == nil {
			m.snapshot = msg.snapshot
		}
	case liveMsg:
		m.state = msg.state
	case clockMsg:
		m.now = time.Time(msg)
		return m, tick()
	case tea.KeyPressMsg:
		if key := msg.String(); key == "q" || key == "ctrl+c" || key == "esc" {
			return m, tea.Quit
		}
	}
	return m, nil
}

// load fetches a fresh snapshot.
func (m *watchModel) load(seq int) tea.Cmd {
	return func() tea.Msg {
		snapshot, err := m.fetch(m.ctx)
		return snapshotMsg{snapshot, err, seq}
	}
}

// View renders the dashboard and the live status line.
func (m *watchModel) View() tea.View {
	return tea.NewView(Dashboard(m.snapshot.Server, m.snapshot.Runs, m.now) + "\n" + m.status() + "\n")
}

// status renders how updates arrive and the key help.
func (m *watchModel) status() string {
	state := map[string]string{
		"live":         ui.Success.Render("● live"),
		"reconnecting": ui.Warning.Render("◌ reconnecting…"),
		"polling":      ui.Muted.Render("◌ polling every 15s"),
		"connecting":   ui.Muted.Render("◌ connecting…"),
	}[m.state]
	if m.err != nil {
		state += ui.Danger.Render("  ·  refresh failed: " + m.err.Error())
	}
	return "  " + state + ui.Faint.Render("  ·  q quit")
}

// tick fires every second.
func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return clockMsg(t) })
}
