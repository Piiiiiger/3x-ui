package agent

import (
	"slices"
	"testing"
	"time"
)

func TestActivity_KeepsAClientOnlineThroughTheGraceWindow(t *testing.T) {
	a := newActivity(onlineGrace)
	t0 := time.Unix(1_700_000_000, 0)
	a.observe(t0, []string{"alice"}, []string{"n1-in"})
	a.observe(t0.Add(10*time.Second), []string{"bob"}, nil)

	emails, tags := a.current(t0.Add(19 * time.Second))
	if !slices.Equal(emails, []string{"alice", "bob"}) || !slices.Equal(tags, []string{"n1-in"}) {
		t.Fatalf("19s after alice's last traffic: emails %v tags %v, want both online and n1-in active", emails, tags)
	}
	emails, tags = a.current(t0.Add(21 * time.Second))
	if !slices.Equal(emails, []string{"bob"}) || len(tags) != 0 {
		t.Fatalf("21s after alice's last traffic: emails %v tags %v, want only bob", emails, tags)
	}
}
