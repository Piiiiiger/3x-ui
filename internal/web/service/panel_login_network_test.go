package service

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
)

// The first sign-in from a network is new and later ones are not; an IPv6 address whose
// privacy suffix rotated is the same /64, while another IPv4 address is a new network.
func TestPanelLoginNetworksRememberWhereTheAdminSignedIn(t *testing.T) {
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "x-ui.db"))
	now := time.Unix(1_791_172_800, 0)
	steps := []struct {
		ip        string
		knownWant bool
	}{
		{"198.51.100.7", false},
		{"198.51.100.7", true},
		{"198.51.100.8", false},
		{"2001:db8:1:2::a", false},
		{"2001:db8:1:2:bf4:6f2:9058:5d7", true},
		{"2001:db8:1:3::a", false},
	}
	for i, step := range steps {
		if got := PanelLoginKnown(step.ip); got != step.knownWant {
			t.Fatalf("step %d: PanelLoginKnown(%s) = %v before sign-in, want %v", i, step.ip, got, step.knownWant)
		}
		isNew, err := RecordPanelLogin(step.ip, now.Add(time.Duration(i)*time.Minute))
		if err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		if isNew != !step.knownWant {
			t.Fatalf("step %d: RecordPanelLogin(%s) new = %v, want %v", i, step.ip, isNew, !step.knownWant)
		}
		if !PanelLoginKnown(step.ip) {
			t.Fatalf("step %d: %s unknown after signing in", i, step.ip)
		}
	}
}
