package agent

import (
	"path/filepath"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/agentproto"
)

func counters(name string, up, down int64) []agentproto.Counter {
	return []agentproto.Counter{{Name: name, Up: up, Down: down}}
}

func clientUp(t *testing.T, r *agentproto.Traffic, email string) int64 {
	t.Helper()
	for _, c := range r.Clients {
		if c.Name == email {
			return c.Up
		}
	}
	t.Fatalf("report %d has no counter for %s: %+v", r.Seq, email, r.Clients)
	return 0
}

// The panel counts (instance, seq) once. Resending a report under a new number,
// or with usage added since, would count part of it twice.
func TestOutbox_ResendsTheSameReportUntilAcked(t *testing.T) {
	o, err := openOutbox(filepath.Join(t.TempDir(), "outbox.json"))
	if err != nil {
		t.Fatal(err)
	}
	if r := o.next(); r != nil {
		t.Fatalf("an empty outbox produced report %+v", r)
	}
	o.add(counters("in-81", 1, 1), counters("alice", 10, 20))
	first := o.next()
	if first == nil || first.Seq != 1 || clientUp(t, first, "alice") != 10 {
		t.Fatalf("first report = %+v", first)
	}

	o.add(nil, counters("alice", 5, 5))
	again := o.next()
	if again.Seq != first.Seq || again.Instance != first.Instance || clientUp(t, again, "alice") != 10 {
		t.Fatalf("unacked report changed: %+v, want %+v", again, first)
	}

	o.ack(first.Instance, first.Seq+1)
	if r := o.next(); r.Seq != first.Seq {
		t.Fatal("an ack for another report must not release this one")
	}
	o.ack(first.Instance, first.Seq)
	second := o.next()
	if second == nil || second.Seq != 2 || clientUp(t, second, "alice") != 5 {
		t.Fatalf("after the ack, report = %+v; want seq 2 with the 5 bytes that waited", second)
	}
}

// Usage the panel has not acked must survive an agent restart, still under the
// same instance and number so the panel can tell whether it already counted it.
func TestOutbox_SurvivesARestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "outbox.json")
	o, err := openOutbox(path)
	if err != nil {
		t.Fatal(err)
	}
	o.add(nil, counters("alice", 10, 20))
	inFlight := o.next()
	o.add(nil, counters("bob", 3, 4))

	reopened, err := openOutbox(path)
	if err != nil {
		t.Fatal(err)
	}
	r := reopened.next()
	if r.Instance != inFlight.Instance || r.Seq != inFlight.Seq || clientUp(t, r, "alice") != 10 {
		t.Fatalf("after restart, in-flight report = %+v, want %+v", r, inFlight)
	}
	reopened.ack(r.Instance, r.Seq)
	if waiting := reopened.next(); waiting == nil || waiting.Seq != inFlight.Seq+1 || clientUp(t, waiting, "bob") != 3 {
		t.Fatalf("usage queued before the restart = %+v, want bob's 3 bytes", waiting)
	}

	other, err := openOutbox(filepath.Join(t.TempDir(), "outbox.json"))
	if err != nil {
		t.Fatal(err)
	}
	other.add(nil, counters("alice", 1, 1))
	if other.next().Instance == r.Instance {
		t.Fatal("a fresh install must report under a new instance")
	}
}

// panelModel applies each (instance, seq) once, as AddAgentTraffic does.
type panelModel struct {
	instance string
	lastSeq  int64
	counted  map[string]int64
}

func (p *panelModel) drain(o *outbox) {
	for r := o.next(); r != nil; r = o.next() {
		if r.Instance != p.instance || r.Seq > p.lastSeq {
			p.instance, p.lastSeq = r.Instance, r.Seq
			for _, c := range r.Clients {
				p.counted[c.Name] += c.Up
			}
		}
		o.ack(r.Instance, r.Seq)
	}
}

// After a restart the panel may already hold the last report the agent built;
// usage counted while the panel was unreachable must not ride on that number.
func TestOutbox_UsageAfterARestartIsCountedOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "outbox.json")
	panel := &panelModel{counted: map[string]int64{}}
	o, err := openOutbox(path)
	if err != nil {
		t.Fatal(err)
	}
	o.add(nil, counters("alice", 10, 0))
	panel.drain(o)

	restarted, err := openOutbox(path)
	if err != nil {
		t.Fatal(err)
	}
	restarted.add(nil, counters("alice", 5, 0))
	panel.drain(restarted)

	if got := panel.counted["alice"]; got != 15 {
		t.Fatalf("panel counted %d bytes for alice, want 15 (10 before the restart, 5 after)", got)
	}
}

// The panel applied the report but the agent restarted before the ack arrived.
func TestOutbox_UsageAfterALostAckIsCountedOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "outbox.json")
	panel := &panelModel{counted: map[string]int64{}}
	o, err := openOutbox(path)
	if err != nil {
		t.Fatal(err)
	}
	o.add(nil, counters("alice", 10, 0))
	r := o.next()
	panel.instance, panel.lastSeq, panel.counted["alice"] = r.Instance, r.Seq, 10

	restarted, err := openOutbox(path)
	if err != nil {
		t.Fatal(err)
	}
	restarted.add(nil, counters("alice", 5, 0))
	panel.drain(restarted)

	if got := panel.counted["alice"]; got != 15 {
		t.Fatalf("panel counted %d bytes for alice, want 15 (10 before the restart, 5 after)", got)
	}
}
