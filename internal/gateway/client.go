package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

	"github.com/pipetower/pipetower-cli/internal/fault"
)

const (
	readLimit    = 32 << 20
	dialTimeout  = 15 * time.Second
	authTimeout  = 15 * time.Second
	closeTimeout = time.Second
)

// pingInterval and pingTimeout detect a dead connection, e.g. after a laptop slept
// (variables for the tests).
var (
	pingInterval = 25 * time.Second
	pingTimeout  = 10 * time.Second
)

// errUndecodable marks a message which is no valid JSON, it is skipped.
var errUndecodable = errors.New("undecodable gateway message")

// Close codes of the gateway.
const (
	CloseAuthTimeout = 4001
	CloseAuthFailed  = 4003
)

// backoff are the delays between reconnect attempts, the last one repeats.
var backoff = []time.Duration{500 * time.Millisecond, time.Second, 2 * time.Second, 5 * time.Second, 10 * time.Second, 15 * time.Second}

// TicketFunc issues a fresh connection ticket and returns the gateway URL with it.
type TicketFunc func(ctx context.Context) (url string, ticket string, err error)

// Client is a self-healing connection to the gateway.
type Client struct {
	tickets  TicketFunc
	inbox    *mailbox
	messages chan Message
	ctx      context.Context
	cancel   context.CancelFunc
	done     chan struct{}
	closing  atomic.Bool

	mu     sync.Mutex
	conn   *websocket.Conn
	topics map[string]bool
	ready  Message
}

// Connect opens the first connection synchronously, so its errors are returned,
// and keeps the connection alive in the background afterwards.
func Connect(ctx context.Context, tickets TicketFunc) (*Client, error) {
	ctx, cancel := context.WithCancel(ctx)
	c := &Client{tickets: tickets, inbox: newMailbox(), messages: make(chan Message), ctx: ctx, cancel: cancel, done: make(chan struct{}), topics: map[string]bool{}}

	conn, ready, err := c.dial()
	if err != nil {
		cancel()
		return nil, err
	}

	go c.deliver()
	c.setConn(conn, ready)
	go c.run(conn)

	return c, nil
}

// Messages delivers all messages of the gateway, including the client's own
// TypeReady (after every (re)connect), TypeDisconnected and TypeFailed.
func (c *Client) Messages() <-chan Message {
	return c.messages
}

// Ready returns the ready message of the current connection.
func (c *Client) Ready() Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ready
}

// Send writes a message, it fails while the connection is down.
func (c *Client) Send(message any) error {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()

	if conn == nil {
		return errors.New("not connected to the gateway")
	}
	return write(c.ctx, conn, message)
}

// Subscribe subscribes a topic now and again after every reconnect.
func (c *Client) Subscribe(topic string) error {
	c.mu.Lock()
	c.topics[topic] = true
	c.mu.Unlock()

	return c.Send(map[string]string{"type": "subscribe", "topic": topic})
}

// Unsubscribe stops receiving a topic.
func (c *Client) Unsubscribe(topic string) error {
	c.mu.Lock()
	delete(c.topics, topic)
	c.mu.Unlock()

	return c.Send(map[string]string{"type": "unsubscribe", "topic": topic})
}

// Close ends the connection for good: it says goodbye to the gateway, then stops reconnecting.
// Nothing is announced anymore, not even the closed connection.
func (c *Client) Close() {
	c.closing.Store(true)
	c.mu.Lock()
	conn := c.conn
	c.conn = nil
	c.mu.Unlock()

	if conn != nil {
		closeBriefly(conn)
	}
	c.cancel()
	<-c.done
}

// closeBriefly says goodbye to the gateway, but does not wait long for its answer.
func closeBriefly(conn *websocket.Conn) {
	closed := make(chan struct{})
	go func() {
		_ = conn.Close(websocket.StatusNormalClosure, "bye")
		close(closed)
	}()

	select {
	case <-closed:
	case <-time.After(closeTimeout):
		_ = conn.CloseNow()
	}
}

// run reads the connection and reconnects whenever it drops.
func (c *Client) run(conn *websocket.Conn) {
	defer close(c.done)
	defer c.inbox.close()

	for {
		c.readAlive(conn)
		_ = conn.CloseNow()
		c.setConn(nil, Message{})
		if c.ctx.Err() != nil || c.closing.Load() {
			return
		}
		c.emit(Message{Type: TypeDisconnected})

		next, err := c.reconnect()
		if err != nil {
			c.emit(Message{Type: TypeFailed, Text: err.Error()})
			return
		}
		conn = next
	}
}

// readAlive reads the connection while pinging it, a connection which does not answer is closed.
func (c *Client) readAlive(conn *websocket.Conn) {
	ctx, stop := context.WithCancel(c.ctx)
	defer stop()
	go keepAlive(ctx, conn)

	c.read(conn)
}

// keepAlive pings the connection in intervals and closes it when a ping is not answered.
func keepAlive(ctx context.Context, conn *websocket.Conn) {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
			err := conn.Ping(pingCtx)
			cancel()
			if err != nil && ctx.Err() == nil {
				_ = conn.CloseNow()
				return
			}
		}
	}
}

// read forwards messages until the connection fails, undecodable messages are skipped.
func (c *Client) read(conn *websocket.Conn) {
	for {
		message, err := readMessage(c.ctx, conn)
		switch {
		case errors.Is(err, errUndecodable):
			continue
		case err != nil:
			return
		}
		c.emit(message)
	}
}

// reconnect retries with backoff until connected, canceled or the credentials are rejected.
func (c *Client) reconnect() (*websocket.Conn, error) {
	for attempt := 0; ; attempt++ {
		if err := sleep(c.ctx, backoff[min(attempt, len(backoff)-1)]); err != nil {
			return nil, err
		}

		conn, ready, err := c.dial()
		if err == nil {
			c.setConn(conn, ready)
			c.resubscribe()
			return conn, nil
		}
		if isPermanent(err) {
			return nil, err
		}
	}
}

// dial fetches a ticket, opens the socket and authenticates.
func (c *Client) dial() (*websocket.Conn, Message, error) {
	url, ticket, err := c.tickets(c.ctx)
	if err != nil {
		return nil, Message{}, err
	}

	dialCtx, cancel := context.WithTimeout(c.ctx, dialTimeout)
	defer cancel()

	conn, response, err := websocket.Dial(dialCtx, url, nil)
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	if err != nil {
		return nil, Message{}, &UnreachableError{URL: url, Err: err}
	}
	conn.SetReadLimit(readLimit)

	ready, err := authenticate(c.ctx, conn, ticket)
	if err != nil {
		_ = conn.CloseNow()
		return nil, Message{}, err
	}

	return conn, ready, nil
}

// setConn stores the current connection and announces a new one.
func (c *Client) setConn(conn *websocket.Conn, ready Message) {
	c.mu.Lock()
	c.conn, c.ready = conn, ready
	c.mu.Unlock()

	if conn != nil {
		c.emit(ready)
	}
}

// resubscribe restores the subscriptions after a reconnect.
func (c *Client) resubscribe() {
	c.mu.Lock()
	topics := make([]string, 0, len(c.topics))
	for topic := range c.topics {
		topics = append(topics, topic)
	}
	c.mu.Unlock()

	for _, topic := range topics {
		_ = c.Send(map[string]string{"type": "subscribe", "topic": topic})
	}
}

// emit queues a message for the consumer.
func (c *Client) emit(message Message) {
	c.inbox.put(message)
}

// deliver hands the queued messages to the consumer in order, until the client ends.
func (c *Client) deliver() {
	defer close(c.messages)
	for {
		message, ok := c.inbox.take(c.ctx)
		if !ok {
			return
		}
		select {
		case c.messages <- message:
		case <-c.ctx.Done():
			return
		}
	}
}

// authenticate sends the ticket and waits for the ready message.
func authenticate(ctx context.Context, conn *websocket.Conn, ticket string) (Message, error) {
	ctx, cancel := context.WithTimeout(ctx, authTimeout)
	defer cancel()

	if err := write(ctx, conn, map[string]string{"type": "auth", "ticket": ticket}); err != nil {
		return Message{}, err
	}

	message, err := readMessage(ctx, conn)
	if err != nil {
		return Message{}, authError(err)
	}
	if message.Type != TypeReady {
		return Message{}, fmt.Errorf("unexpected gateway message %q", message.Type)
	}

	return message, nil
}

// authError translates the close code of a rejected authentication.
func authError(err error) error {
	switch websocket.CloseStatus(err) {
	case CloseAuthFailed:
		return &AuthError{Reason: "the ticket was rejected"}
	case CloseAuthTimeout:
		return &AuthError{Reason: "the authentication timed out"}
	}
	return err
}

// readMessage reads and decodes one message.
func readMessage(ctx context.Context, conn *websocket.Conn) (Message, error) {
	_, data, err := conn.Read(ctx)
	if err != nil {
		return Message{}, err
	}

	var message Message
	if err := json.Unmarshal(data, &message); err != nil {
		return Message{}, fmt.Errorf("%w: %w", errUndecodable, err)
	}
	return message, nil
}

// write encodes and sends one message.
func write(ctx context.Context, conn *websocket.Conn, message any) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	return conn.Write(ctx, websocket.MessageText, data)
}

// sleep waits for the duration or until the context ends.
func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// isPermanent tells whether retrying cannot help: a rejected API token, or a fresh ticket
// rejected by the gateway (backend and gateway disagree about the ticket secret).
func isPermanent(err error) bool {
	var explainer fault.Explainer
	var authErr *AuthError
	return errors.As(err, &authErr) || (errors.As(err, &explainer) && explainer.Problem().Code == fault.ExitConfig)
}

// Connector opens a gateway connection, it hides how tickets are obtained.
type Connector func(ctx context.Context) (*Client, error)
