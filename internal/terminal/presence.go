package terminal

import (
	"fmt"
	"strings"

	"github.com/pipetower/pipetower-cli/internal/api"
	"github.com/pipetower/pipetower-cli/internal/gateway"
)

// presence tracks who is attached to the session and who may type.
type presence struct {
	session      api.Session
	participants map[string]gateway.Participant
	requests     map[string]bool
	known        bool
}

// newPresence creates the presence of a session.
func newPresence(session api.Session) *presence {
	return &presence{session: session, participants: map[string]gateway.Participant{}, requests: map[string]bool{}}
}

// update takes over the participants and describes what changed for the user.
func (p *presence) update(me string, participants []gateway.Participant, requests []string) string {
	next := map[string]gateway.Participant{}
	for _, participant := range participants {
		next[participant.ID] = participant
	}
	nextRequests := map[string]bool{}
	for _, id := range requests {
		nextRequests[id] = true
	}

	var notices []string
	if p.known {
		notices = p.changes(me, next, nextRequests)
	}
	p.participants, p.requests, p.known = next, nextRequests, true

	return strings.Join(notices, ", ")
}

// changes lists joins, leaves, control changes and new requests.
func (p *presence) changes(me string, next map[string]gateway.Participant, requests map[string]bool) []string {
	var notices []string
	for id, participant := range next {
		previous, existed := p.participants[id]
		switch {
		case !existed && id != me:
			notices = append(notices, participant.Name+" joined")
		case existed && previous.Control != participant.Control:
			notices = append(notices, controlNotice(participant, id == me))
		}
		if requests[id] && !p.requests[id] {
			notices = append(notices, participant.Name+" requests control")
		}
	}
	for id, participant := range p.participants {
		if _, still := next[id]; !still && id != me {
			notices = append(notices, participant.Name+" left")
		}
	}
	return notices
}

// controlNotice describes a control change.
func controlNotice(participant gateway.Participant, isMe bool) string {
	name := participant.Name
	if isMe {
		name = "you"
	}
	if participant.Control {
		return name + " got control"
	}
	return name + " lost control"
}

// hasControl tells whether the user may type. Before the first update only the owner may.
func (p *presence) hasControl(me string) bool {
	if !p.known {
		return p.session.Role == "owner"
	}
	return p.participants[me].Control
}

// describe renders the title: server, role and participants.
func (p *presence) describe(me string) string {
	role := "watching (Enter ~c requests control)"
	if p.hasControl(me) {
		role = "control"
	}
	if p.session.Role == "owner" {
		role = "owner"
	}
	return fmt.Sprintf("pt ▸ %s · %s · %d attached", p.session.Server.Name, role, len(p.participants))
}
