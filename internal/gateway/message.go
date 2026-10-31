// Package gateway is the client of the Pipetower real-time gateway: one WebSocket
// per process, authenticated with a single-use ticket, which delivers run output,
// change notifications and terminal sessions.
package gateway

import "encoding/json"

// Message types of the gateway protocol.
const (
	TypeReady        = "ready"
	TypeSubscribed   = "subscribed"
	TypeChanged      = "changed"
	TypeRunOutput    = "run.output"
	TypeAttached     = "attached"
	TypeOutput       = "output"
	TypeParticipants = "participants"
	TypeEnded        = "ended"
	TypeError        = "error"

	// TypeDisconnected is emitted by the client when the connection dropped, it reconnects by itself.
	TypeDisconnected = "gateway.disconnected"
	// TypeFailed is emitted by the client when it gave up reconnecting.
	TypeFailed = "gateway.failed"
)

// Topic names.
const (
	TopicRuns = "runs"
)

// RunTopic is the topic of one run.
func RunTopic(id string) string {
	return "run." + id
}

// ServerTopic is the topic of one server.
func ServerTopic(id string) string {
	return "server." + id
}

// Message is any message of the gateway. Only the fields of its type are set.
type Message struct {
	Type   string `json:"type"`
	Topic  string `json:"topic,omitempty"`
	ID     string `json:"id,omitempty"`
	Run    string `json:"run,omitempty"`
	Stream string `json:"stream,omitempty"`
	Data   string `json:"data,omitempty"`
	Replay bool   `json:"replay,omitempty"`
	Offset *int   `json:"offset,omitempty"`
	Text   string `json:"message,omitempty"`
	Reason string `json:"reason,omitempty"`

	// Session is the session id of terminal messages (lobby messages carry an object instead).
	Session json.RawMessage `json:"session,omitempty"`

	User     *User          `json:"user,omitempty"`
	Sessions []LobbySession `json:"sessions,omitempty"`

	Participants []Participant `json:"participants,omitempty"`
	Requests     []string      `json:"requests,omitempty"`
	Snapshot     string        `json:"snapshot,omitempty"`
}

// OutputOffset returns the position of run output in the whole output, or -1 if unknown.
func (m Message) OutputOffset() int {
	if m.Offset == nil {
		return -1
	}
	return *m.Offset
}

// SessionID returns the session id of a terminal message.
func (m Message) SessionID() string {
	var id string
	if json.Unmarshal(m.Session, &id) == nil {
		return id
	}
	return ""
}

// User is the authenticated user of the connection.
type User struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Initials string `json:"initials"`
}

// Participant is a user attached to a terminal session.
type Participant struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Initials string `json:"initials"`
	Control  bool   `json:"control"`
}

// LobbySession is a running terminal session of one of the user's teams.
type LobbySession struct {
	ID           string        `json:"id"`
	Team         string        `json:"team"`
	Server       ServerInfo    `json:"server"`
	Owner        User          `json:"owner"`
	Participants []Participant `json:"participants"`
	Requests     []string      `json:"requests"`
	StartedAt    string        `json:"startedAt"`
}

// ServerInfo is the short description of a server.
type ServerInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Host string `json:"host"`
}
