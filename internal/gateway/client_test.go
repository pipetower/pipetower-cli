package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// fakeGateway accepts connections, checks the ticket and echoes subscriptions.
type fakeGateway struct {
	t           *testing.T
	connections atomic.Int32
	dropFirst   bool
	garbage     bool
	flood       int
	closeCode   atomic.Int32
}

func (g *fakeGateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	n := g.connections.Add(1)
	ctx := r.Context()

	var auth map[string]string
	if err := readJSON(ctx, conn, &auth); err != nil || auth["type"] != "auth" {
		return
	}
	if auth["ticket"] != "good" {
		_ = conn.Close(CloseAuthFailed, "Authentication failed")
		return
	}
	writeJSON(ctx, conn, map[string]any{"type": "ready", "user": map[string]string{"id": "u1"}, "sessions": []any{}})

	for {
		var message map[string]string
		if err := readJSON(ctx, conn, &message); err != nil {
			g.closeCode.Store(int32(websocket.CloseStatus(err)))
			return
		}
		if message["type"] == "subscribe" {
			// flooded concurrently, the server keeps reading and answering pings like a real one
			go func(topic string) {
				for i := range g.flood {
					writeJSON(ctx, conn, map[string]any{"type": "run.output", "topic": topic, "stream": "s1", "data": "x", "offset": i})
				}
			}(message["topic"])
			if g.garbage {
				_ = conn.Write(ctx, websocket.MessageText, []byte(`{"type":"output","cols":"not a number"}`))
				_ = conn.Write(ctx, websocket.MessageText, []byte(`not json`))
			}
			writeJSON(ctx, conn, map[string]any{"type": "run.output", "topic": message["topic"], "stream": "s1", "data": "hi", "replay": true})
			writeJSON(ctx, conn, map[string]any{"type": "subscribed", "topic": message["topic"]})
			if g.dropFirst && n == 1 {
				_ = conn.CloseNow()
				return
			}
		}
	}
}

func readJSON(ctx context.Context, conn *websocket.Conn, v any) error {
	_, data, err := conn.Read(ctx)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

func writeJSON(ctx context.Context, conn *websocket.Conn, v any) {
	data, _ := json.Marshal(v)
	_ = conn.Write(ctx, websocket.MessageText, data)
}

func tickets(url, ticket string) TicketFunc {
	return func(context.Context) (string, string, error) { return url, ticket, nil }
}

func next(t *testing.T, c *Client, types ...string) Message {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case m := <-c.Messages():
			for _, want := range types {
				if m.Type == want {
					return m
				}
			}
		case <-timeout:
			t.Fatalf("timed out waiting for %v", types)
		}
	}
}

func TestSubscribeReceivesReplay(t *testing.T) {
	server := httptest.NewServer(&fakeGateway{t: t})
	defer server.Close()

	client, err := Connect(context.Background(), tickets("ws"+strings.TrimPrefix(server.URL, "http"), "good"))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	if ready := next(t, client, TypeReady); ready.User == nil || ready.User.ID != "u1" {
		t.Errorf("ready = %+v", ready)
	}
	if err := client.Subscribe(RunTopic("r1")); err != nil {
		t.Fatal(err)
	}
	output := next(t, client, TypeRunOutput)
	if output.Topic != "run.r1" || output.Data != "hi" || !output.Replay {
		t.Errorf("output = %+v", output)
	}
}

func TestRejectedTicket(t *testing.T) {
	server := httptest.NewServer(&fakeGateway{t: t})
	defer server.Close()

	_, err := Connect(context.Background(), tickets(server.URL, "bad"))

	var authErr *AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("err = %v", err)
	}
}

func TestReconnectResubscribes(t *testing.T) {
	gateway := &fakeGateway{t: t, dropFirst: true}
	server := httptest.NewServer(gateway)
	defer server.Close()

	client, err := Connect(context.Background(), tickets(server.URL, "good"))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	_ = client.Subscribe("runs")
	next(t, client, TypeSubscribed)
	next(t, client, TypeDisconnected)
	next(t, client, TypeReady)
	if m := next(t, client, TypeSubscribed); m.Topic != "runs" {
		t.Errorf("resubscribed = %+v", m)
	}
	if gateway.connections.Load() != 2 {
		t.Errorf("connections = %d", gateway.connections.Load())
	}
}

func TestSessionID(t *testing.T) {
	var message Message
	_ = json.Unmarshal([]byte(`{"type":"output","session":"s1","data":"x"}`), &message)

	if message.SessionID() != "s1" {
		t.Errorf("id = %q", message.SessionID())
	}
}

func TestUndecodableMessagesAreSkippedWithoutReconnecting(t *testing.T) {
	gateway := &fakeGateway{t: t, garbage: true}
	server := httptest.NewServer(gateway)
	defer server.Close()

	client, err := Connect(context.Background(), tickets(server.URL, "good"))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	_ = client.Subscribe("runs")
	if m := next(t, client, TypeSubscribed, TypeDisconnected); m.Type != TypeSubscribed {
		t.Fatalf("got %q, the connection must survive undecodable messages", m.Type)
	}
	if gateway.connections.Load() != 1 {
		t.Errorf("connections = %d, want 1", gateway.connections.Load())
	}
}

func TestCloseSaysGoodbye(t *testing.T) {
	gateway := &fakeGateway{t: t}
	server := httptest.NewServer(gateway)
	defer server.Close()

	client, err := Connect(context.Background(), tickets(server.URL, "good"))
	if err != nil {
		t.Fatal(err)
	}
	client.Close()

	deadline := time.Now().Add(2 * time.Second)
	for gateway.closeCode.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if code := websocket.StatusCode(gateway.closeCode.Load()); code != websocket.StatusNormalClosure {
		t.Errorf("close code = %v, want a normal closure", code)
	}
	for range client.Messages() {
		// the channel is closed after the buffered messages
	}
}

func TestUnansweredPingsCloseTheConnection(t *testing.T) {
	interval, timeout := pingInterval, pingTimeout
	pingInterval, pingTimeout = 20*time.Millisecond, 50*time.Millisecond
	t.Cleanup(func() { pingInterval, pingTimeout = interval, timeout })

	// a gateway which authenticates and then never reads again, so it answers no pings
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		var auth map[string]string
		_ = readJSON(r.Context(), conn, &auth)
		writeJSON(r.Context(), conn, map[string]any{"type": "ready", "sessions": []any{}})
		<-r.Context().Done()
	}))
	defer server.Close()

	client, err := Connect(context.Background(), tickets(server.URL, "good"))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	next(t, client, TypeDisconnected)
}

func TestPausedConsumerKeepsTheConnection(t *testing.T) {
	interval, timeout := pingInterval, pingTimeout
	pingInterval, pingTimeout = 20*time.Millisecond, 50*time.Millisecond
	t.Cleanup(func() { pingInterval, pingTimeout = interval, timeout })

	gateway := &fakeGateway{t: t, flood: 2000}
	server := httptest.NewServer(gateway)
	defer server.Close()

	client, err := Connect(context.Background(), tickets(server.URL, "good"))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	_ = client.Subscribe("runs")
	time.Sleep(300 * time.Millisecond) // the consumer is busy while lots of output arrives and pings are due

	received, quiet := 0, time.After(500*time.Millisecond)
	for done := false; !done; {
		select {
		case m := <-client.Messages():
			received++
			if m.Type == TypeDisconnected {
				t.Fatalf("disconnected after %d messages, a paused consumer must not cost the connection", received)
			}
		case <-quiet:
			done = true
		}
	}
	if received < 2000 || gateway.connections.Load() != 1 {
		t.Errorf("received %d messages over %d connections", received, gateway.connections.Load())
	}
}

func TestCloseAnnouncesNothingAnymore(t *testing.T) {
	server := httptest.NewServer(&fakeGateway{t: t})
	defer server.Close()

	client, err := Connect(context.Background(), tickets(server.URL, "good"))
	if err != nil {
		t.Fatal(err)
	}
	client.Close()

	for message := range client.Messages() {
		if message.Type == TypeDisconnected || message.Type == TypeFailed {
			t.Errorf("announced %q after Close", message.Type)
		}
	}
}
