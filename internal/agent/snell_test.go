package agent

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/snell"
)

func TestSnellReconcileRollbackRemovalAndExpiry(t *testing.T) {
	dir := t.TempDir()
	states := map[string]string{}
	restarts := 0
	fail := false
	m := &snellManager{dir: filepath.Join(dir, "state"), units: dir, binary: "/trusted/snell", known: map[int]snell.Instance{}, ready: true}
	m.run = func(args ...string) (string, error) {
		switch args[0] {
		case "restart":
			restarts++
			if fail {
				fail = false
				return "", errors.New("bind failed")
			}
			states[args[1]] = "active"
		case "disable":
			states[args[2]] = "inactive"
		case "show":
			return states[args[1]], nil
		}
		return "", nil
	}
	i := snell.Instance{ID: 11, Tag: "snell-11", Port: 26163, Settings: snell.Settings{PSK: "0123456789abcdef0123456789abcdef", Version: 5}}
	unrelated := filepath.Join(dir, "snell-neburst.service")
	os.WriteFile(unrelated, []byte("original"), 0o600)
	if err := m.apply([]snell.Instance{i}); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(m.configPath(i.ID))
	if st.Mode().Perm() != 0o600 {
		t.Fatal("config exposed")
	}
	unit, _ := os.ReadFile(m.unitPath(i.ID))
	if strings.Contains(string(unit), i.Settings.PSK) {
		t.Fatal("PSK in public unit")
	}
	before := restarts
	if err := m.apply([]snell.Instance{i}); err != nil || restarts != before {
		t.Fatal("unchanged listener restarted")
	}
	changed := i
	changed.Port++
	fail = true
	if m.apply([]snell.Instance{changed}) == nil {
		t.Fatal("failed start acknowledged")
	}
	if m.known[i.ID].Port != i.Port || states[snellUnit(i.ID)] != "active" {
		t.Fatal("old service not restored")
	}
	cfg, _ := os.ReadFile(m.configPath(i.ID))
	if string(cfg) != i.ServerConfig() {
		t.Fatal("rollback config mismatch")
	}
	if m.statuses(time.Now())[i.Tag].State != "running" {
		t.Fatal("missing runtime status")
	}
	expired := i
	expired.ExpiryTime = time.Now().Add(-time.Second).UnixMilli()
	m.known[i.ID] = expired
	if m.statuses(time.Now())[i.Tag].State != "expired" || states[snellUnit(i.ID)] != "inactive" {
		t.Fatal("expiry not enforced")
	}
	if err := m.apply(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(m.unitPath(i.ID)); !os.IsNotExist(err) {
		t.Fatal("managed unit not removed")
	}
	if data, _ := os.ReadFile(unrelated); string(data) != "original" {
		t.Fatal("unmanaged service modified")
	}
}

func TestSnellMissingBinaryRejectedBeforeMutation(t *testing.T) {
	calls := 0
	m := &snellManager{known: map[int]snell.Instance{}, run: func(...string) (string, error) { calls++; return "", nil }}
	if m.apply([]snell.Instance{{ID: 1}}) == nil || calls != 0 {
		t.Fatal("unsupported host mutated")
	}
}
