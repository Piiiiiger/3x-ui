// Package agenttest plays a pigger-agent against a panel's agent endpoint in
// tests, over a real WebSocket.
package agenttest

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/agentproto"

	"github.com/gorilla/websocket"
)

// Server upgrades every request and hands the connection to serve, the way the
// panel's agent route does; it returns the ws:// URL to dial.
func Server(t testing.TB, serve func(*websocket.Conn)) string {
	t.Helper()
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if conn, err := upgrader.Upgrade(w, r, nil); err == nil {
			serve(conn)
		}
	}))
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http")
}

// Agent is the agent side of one connection.
type Agent struct {
	T    testing.TB
	Conn *websocket.Conn
}

// Dial connects to url and sends hello, as an agent does first.
func Dial(t testing.TB, url string, hello agentproto.Hello) *Agent {
	t.Helper()
	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		t.Fatalf("dial agent endpoint: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	a := &Agent{T: t, Conn: conn}
	a.Send(agentproto.Message{Type: agentproto.TypeHello, Hello: &hello})
	return a
}

func (a *Agent) Send(m agentproto.Message) {
	a.T.Helper()
	if err := a.Conn.WriteJSON(m); err != nil {
		a.T.Fatalf("agent send %s: %v", m.Type, err)
	}
}

func (a *Agent) Recv(timeout time.Duration) (agentproto.Message, error) {
	var m agentproto.Message
	_ = a.Conn.SetReadDeadline(time.Now().Add(timeout))
	err := a.Conn.ReadJSON(&m)
	return m, err
}

// ExpectClosed fails unless the panel closed the connection; a read that merely
// times out means it is still open.
func (a *Agent) ExpectClosed(why string) {
	a.T.Helper()
	_, err := a.Recv(2 * time.Second)
	var netErr net.Error
	if err == nil || (errors.As(err, &netErr) && netErr.Timeout()) {
		a.T.Fatalf("%s: connection still open (read returned %v)", why, err)
	}
}

// AnswerApplies answers every apply with ok, in the background, and hands each
// apply it received to the returned channel. Call it instead of reading Conn.
func (a *Agent) AnswerApplies(ok bool) <-chan agentproto.Apply {
	got := make(chan agentproto.Apply, 16)
	go func() {
		for {
			var msg agentproto.Message
			if err := a.Conn.ReadJSON(&msg); err != nil {
				return
			}
			if msg.Type != agentproto.TypeApply || msg.Apply == nil {
				continue
			}
			got <- *msg.Apply
			res := &agentproto.Result{OK: ok, ConfigHash: msg.Apply.Hash}
			if !ok {
				res.Error = "refused"
			}
			_ = a.Conn.WriteJSON(agentproto.Message{Type: agentproto.TypeResult, ID: msg.ID, Result: res})
		}
	}()
	return got
}
