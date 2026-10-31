package gateway

import (
	"context"
	"sync"
)

// mailbox is an unbounded queue of messages: reading the connection (which also answers its
// pings) never waits for a slow consumer, e.g. a paused pager.
type mailbox struct {
	mu     sync.Mutex
	queue  []Message
	closed bool
	wake   chan struct{}
}

// newMailbox creates an empty mailbox.
func newMailbox() *mailbox {
	return &mailbox{wake: make(chan struct{}, 1)}
}

// put queues a message, after closing messages are dropped.
func (b *mailbox) put(message Message) {
	b.mu.Lock()
	if !b.closed {
		b.queue = append(b.queue, message)
	}
	b.mu.Unlock()
	b.signal()
}

// close ends the mailbox, the queued messages can still be taken.
func (b *mailbox) close() {
	b.mu.Lock()
	b.closed = true
	b.mu.Unlock()
	b.signal()
}

// take waits for the next message, it reports false once the mailbox is closed and empty or the context ended.
func (b *mailbox) take(ctx context.Context) (Message, bool) {
	for {
		b.mu.Lock()
		if len(b.queue) > 0 {
			message := b.queue[0]
			b.queue = b.queue[1:]
			b.mu.Unlock()
			return message, true
		}
		closed := b.closed
		b.mu.Unlock()

		if closed {
			return Message{}, false
		}
		select {
		case <-b.wake:
		case <-ctx.Done():
			return Message{}, false
		}
	}
}

// signal wakes a waiting take.
func (b *mailbox) signal() {
	select {
	case b.wake <- struct{}{}:
	default:
	}
}
