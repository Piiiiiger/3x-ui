package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/snell"
)

const snellBinary = "/usr/local/libexec/pigger-agent/snell-server"

// Only these generated units belong to this manager. Existing independent Snell
// services are never adopted or stopped merely because they share a protocol.
type snellManager struct {
	dir, units, binary string
	run                func(...string) (string, error)
	known              map[int]snell.Instance
	ready              bool
	reconciled         bool
	initErr            error
}

func systemctl(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "systemctl", args...).CombinedOutput()
	// Do not include process output: a failed service may echo its credentials.
	if err != nil {
		return "", fmt.Errorf("systemctl %s failed: %w", args[0], err)
	}
	return strings.TrimSpace(string(out)), nil
}

func newSnellManager(stateDir string) *snellManager {
	m := &snellManager{dir: filepath.Join(stateDir, "snell"), units: "/etc/systemd/system", binary: snellBinary, run: systemctl, known: map[int]snell.Instance{}}
	data, err := os.ReadFile(filepath.Join(m.dir, "manifest.json"))
	if err == nil {
		m.initErr = json.Unmarshal(data, &m.known)
	} else if !errors.Is(err, os.ErrNotExist) {
		m.initErr = err
	}
	if m.known == nil {
		m.known = map[int]snell.Instance{}
	}
	if m.initErr == nil {
		for id, i := range m.known {
			if id != i.ID || i.Validate() != nil {
				m.initErr = fmt.Errorf("invalid Snell manifest")
				break
			}
		}
	}
	if st, err := os.Stat(m.binary); err == nil && st.Mode().IsRegular() && st.Mode().Perm()&0o111 != 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, m.binary, "--version").CombinedOutput()
		_, systemdErr := os.Stat("/run/systemd/system")
		m.ready = err == nil && systemdErr == nil && strings.Contains(string(out), "snell-server v5.")
	}
	return m
}
func snellUnit(id int) string { return "pigger-snell-" + strconv.Itoa(id) + ".service" }
func (m *snellManager) configPath(id int) string {
	return filepath.Join(m.dir, strconv.Itoa(id)+".conf")
}
func (m *snellManager) unitPath(id int) string { return filepath.Join(m.units, snellUnit(id)) }
func (m *snellManager) unit(i snell.Instance) []byte {
	// Paths are agent-local constants, never provided by the panel or an inbound.
	return []byte(fmt.Sprintf(`[Unit]
Description=Panel-managed Snell inbound %d
Wants=network-online.target
After=network-online.target
StartLimitIntervalSec=60
StartLimitBurst=5

[Service]
Type=simple
DynamicUser=yes
LoadCredential=snell.conf:%s
ExecStart=%s -c %%d/snell.conf
Restart=on-failure
RestartSec=3
NoNewPrivileges=yes
PrivateTmp=yes
ProtectSystem=strict
ProtectHome=yes
ProtectKernelTunables=yes
ProtectKernelModules=yes
ProtectControlGroups=yes
RestrictSUIDSGID=yes
RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
AmbientCapabilities=CAP_NET_BIND_SERVICE
UMask=0077
LimitNOFILE=16384

[Install]
WantedBy=multi-user.target
`, i.ID, m.configPath(i.ID), m.binary))
}

func (m *snellManager) validate(want []snell.Instance) error {
	if m.initErr != nil {
		return fmt.Errorf("cannot load managed Snell state: %w", m.initErr)
	}
	if len(want) > 0 && !m.ready {
		return fmt.Errorf("install a Snell v5 binary at %s and run the agent on systemd", m.binary)
	}
	ids := map[int]bool{}
	tags := map[string]bool{}
	for _, i := range want {
		if err := i.Validate(); err != nil {
			return err
		}
		if ids[i.ID] || tags[i.Tag] {
			return fmt.Errorf("duplicate Snell instance")
		}
		ids[i.ID] = true
		tags[i.Tag] = true
	}
	return nil
}

func snellIDs(m map[int]snell.Instance) []int {
	ids := make([]int, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return ids
}

func (m *snellManager) start(i snell.Instance) error {
	if i.ExpiryTime > 0 && i.ExpiryTime <= time.Now().UnixMilli() {
		_, err := m.run("disable", "--now", snellUnit(i.ID))
		return err
	}
	if _, err := m.run("enable", snellUnit(i.ID)); err != nil {
		return err
	}
	if _, err := m.run("restart", snellUnit(i.ID)); err != nil {
		return err
	}
	// Type=simple can report success before Snell rejects the bind/config.
	time.Sleep(150 * time.Millisecond)
	state, err := m.run("show", snellUnit(i.ID), "--property=ActiveState", "--value")
	if err != nil {
		return err
	}
	if state != "active" {
		return fmt.Errorf("Snell inbound %d did not start", i.ID)
	}
	return nil
}

func (m *snellManager) write(i snell.Instance) error {
	if err := writeFileAtomic(m.configPath(i.ID), []byte(i.ServerConfig()), 0o600); err != nil {
		return err
	}
	return writeFileAtomic(m.unitPath(i.ID), m.unit(i), 0o644)
}

// apply reconciles a complete desired set. If any changed process fails, restore
// previous configs and service states before reporting failure to the panel.
func (m *snellManager) apply(want []snell.Instance) error {
	if err := m.validate(want); err != nil {
		return err
	}
	next := map[int]snell.Instance{}
	for _, i := range want {
		next[i.ID] = i
	}
	if reflect.DeepEqual(next, m.known) && (m.reconciled || len(next) == 0) {
		return nil
	}
	if err := os.MkdirAll(m.dir, 0o700); err != nil {
		return err
	}
	touched := map[int]bool{}
	rollback := func(cause error) error {
		var failures []string
		for id := range touched {
			if old, ok := m.known[id]; ok {
				if err := m.write(old); err != nil {
					failures = append(failures, strconv.Itoa(id))
				}
			}
			if _, ok := m.known[id]; !ok {
				if _, err := m.run("disable", "--now", snellUnit(id)); err != nil {
					failures = append(failures, strconv.Itoa(id))
				}
				_ = os.Remove(m.unitPath(id))
				_ = os.Remove(m.configPath(id))
			}
		}
		if _, err := m.run("daemon-reload"); err != nil {
			failures = append(failures, "reload")
		}
		for id := range touched {
			if old, ok := m.known[id]; ok {
				if err := m.start(old); err != nil {
					failures = append(failures, strconv.Itoa(id))
				}
			}
		}
		if len(failures) > 0 {
			return fmt.Errorf("%w; rollback needs attention for %s", cause, strings.Join(failures, ","))
		}
		return cause
	}
	// Stop removed/changed listeners before starting replacements (port swaps).
	for _, id := range snellIDs(m.known) {
		old := m.known[id]
		if n, ok := next[id]; ok && m.reconciled && reflect.DeepEqual(old, n) {
			continue
		}
		touched[id] = true
		if _, err := m.run("disable", "--now", snellUnit(id)); err != nil {
			return rollback(err)
		}
	}
	for _, id := range snellIDs(next) {
		i := next[id]
		if old, ok := m.known[id]; ok && m.reconciled && reflect.DeepEqual(old, i) {
			continue
		}
		touched[id] = true
		if err := m.write(i); err != nil {
			return rollback(err)
		}
	}
	if _, err := m.run("daemon-reload"); err != nil {
		return rollback(err)
	}
	for _, id := range snellIDs(next) {
		if touched[id] {
			if err := m.start(next[id]); err != nil {
				return rollback(err)
			}
		}
	}
	data, err := json.Marshal(next)
	if err != nil {
		return rollback(err)
	}
	if err := writeFileAtomic(filepath.Join(m.dir, "manifest.json"), data, 0o600); err != nil {
		return rollback(err)
	}
	for id := range m.known {
		if _, ok := next[id]; !ok {
			_ = os.Remove(m.unitPath(id))
			_ = os.Remove(m.configPath(id))
		}
	}
	m.known = next
	m.reconciled = true
	return nil
}

func (m *snellManager) statuses(now time.Time) map[string]snell.Status {
	out := map[string]snell.Status{}
	for _, id := range snellIDs(m.known) {
		i := m.known[id]
		if i.ExpiryTime > 0 && i.ExpiryTime <= now.UnixMilli() {
			_, err := m.run("disable", "--now", snellUnit(id))
			if err != nil {
				out[i.Tag] = snell.Status{State: "error", Error: "failed to stop expired Snell service"}
			} else {
				out[i.Tag] = snell.Status{State: "expired"}
			}
			continue
		}
		state, err := m.run("show", snellUnit(id), "--property=ActiveState", "--value")
		st := snell.Status{State: "stopped"}
		if err != nil {
			st = snell.Status{State: "error", Error: "cannot read Snell service state"}
		} else {
			switch state {
			case "active":
				st.State = "running"
			case "activating":
				st.State = "starting"
			case "failed":
				st = snell.Status{State: "error", Error: "Snell process failed; check the host service journal"}
			}
		}
		out[i.Tag] = st
	}
	return out
}
