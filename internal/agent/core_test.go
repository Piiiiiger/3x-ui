package agent

import (
	"encoding/json"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

func freeTCPPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func vlessUser(email string, n int) map[string]any {
	return map[string]any{"email": email, "id": fmt.Sprintf("a17e367c-2074-4d3e-aaeb-fbef5dfde7%02d", n)}
}

// testConfig is the shape the panel pushes: an API inbound plus one VLESS inbound.
func testConfig(t *testing.T, apiPort, port int, users ...map[string]any) []byte {
	t.Helper()
	if users == nil {
		users = []map[string]any{}
	}
	cfg := map[string]any{
		"log": map[string]any{"loglevel": "error"},
		"api": map[string]any{"services": []string{"HandlerService", "StatsService"}, "tag": "api"},
		"inbounds": []any{
			map[string]any{"listen": "127.0.0.1", "port": apiPort, "protocol": "tunnel", "settings": map[string]any{"rewriteAddress": "127.0.0.1"}, "tag": "api"},
			map[string]any{"listen": "127.0.0.1", "port": port, "protocol": "vless", "settings": map[string]any{"clients": users, "decryption": "none"}, "tag": "n1-in"},
		},
		"outbounds": []any{map[string]any{"protocol": "freedom", "tag": "direct"}},
		"policy":    map[string]any{"levels": map[string]any{"0": map[string]any{"statsUserUplink": true, "statsUserDownlink": true}}},
		"routing":   map[string]any{"rules": []any{map[string]any{"type": "field", "inboundTag": []string{"api"}, "outboundTag": "api"}}},
		"stats":     map[string]any{},
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func startCore(t *testing.T, raw []byte) *xrayCore {
	t.Helper()
	c := newXrayCore()
	if _, err := c.apply(raw, "h0", false); err != nil {
		t.Fatalf("start core: %v", err)
	}
	t.Cleanup(c.stop)
	return c
}

func userOnCore(t *testing.T, apiPort int, email string) bool {
	t.Helper()
	api := xray.XrayAPI{}
	if err := api.Init(apiPort); err != nil {
		t.Fatalf("api init: %v", err)
	}
	defer api.Close()
	probe := map[string]any{"email": email, "id": "0f6ad6a7-5d56-4a0b-9d4b-6e4c5f0a1b2c", "flow": ""}
	err := api.AddUser("vless", "n1-in", probe)
	if err == nil {
		_ = api.RemoveUser("n1-in", email)
		return false
	}
	if xray.IsUserExistsErr(err) {
		return true
	}
	t.Fatalf("probe %s: %v", email, err)
	return false
}

func listening(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// Adding a user must not restart the core: a restart drops every live session.
func TestCore_AddsAUserWithoutRestarting(t *testing.T) {
	apiPort, port := freeTCPPort(t), freeTCPPort(t)
	c := startCore(t, testConfig(t, apiPort, port, vlessUser("alice", 1)))

	restarted, err := c.apply(testConfig(t, apiPort, port, vlessUser("alice", 1), vlessUser("bob", 2)), "h1", false)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if restarted {
		t.Fatal("a user change restarted the core")
	}
	if !userOnCore(t, apiPort, "bob") {
		t.Fatal("bob must be on the running core")
	}
	if got := c.state().Hash; got != "h1" {
		t.Fatalf("hash = %q, want h1", got)
	}
}

func TestCore_RestartsForAChangeTheAPICannotMake(t *testing.T) {
	apiPort, port, newPort := freeTCPPort(t), freeTCPPort(t), freeTCPPort(t)
	c := startCore(t, testConfig(t, apiPort, port, vlessUser("alice", 1)))
	newAPIPort := freeTCPPort(t)

	restarted, err := c.apply(testConfig(t, newAPIPort, newPort, vlessUser("alice", 1)), "h1", false)
	if err != nil || !restarted {
		t.Fatalf("apply = restarted %v, %v; moving the API port needs a restart", restarted, err)
	}
	if !listening(newPort) || listening(port) {
		t.Fatalf("after the restart: new port listening %v, old port listening %v", listening(newPort), listening(port))
	}
	if !userOnCore(t, newAPIPort, "alice") {
		t.Fatal("alice must be on the restarted core")
	}
}

// RemoveUser leaves the user's open connections alive, so a panel that wants a
// cut client gone at once asks for a restart.
func TestCore_RestartsToDropUsersWhenAsked(t *testing.T) {
	apiPort, port := freeTCPPort(t), freeTCPPort(t)
	c := startCore(t, testConfig(t, apiPort, port, vlessUser("alice", 1), vlessUser("bob", 2)))

	restarted, err := c.apply(testConfig(t, apiPort, port, vlessUser("alice", 1)), "h1", true)
	if err != nil || !restarted {
		t.Fatalf("apply dropping bob = restarted %v, %v; want a restart", restarted, err)
	}
	if userOnCore(t, apiPort, "bob") {
		t.Fatal("bob must be gone")
	}
}

// A config the core cannot start must leave users on the one that works.
func TestCore_KeepsTheRunningConfigWhenTheNewOneFails(t *testing.T) {
	apiPort, port := freeTCPPort(t), freeTCPPort(t)
	c := startCore(t, testConfig(t, apiPort, port, vlessUser("alice", 1)))
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer taken.Close()
	takenPort := taken.Addr().(*net.TCPAddr).Port

	if _, err := c.apply(testConfig(t, freeTCPPort(t), takenPort, vlessUser("alice", 1)), "bad", false); err == nil {
		t.Fatal("a config on a port another process holds must fail")
	}
	st := c.state()
	if st.Hash != "h0" || !st.Running || st.Err == "" {
		t.Fatalf("state after the failure = %+v; want h0 still running and the error shown", st)
	}
	if !listening(port) || !userOnCore(t, apiPort, "alice") {
		t.Fatal("the previous config must be serving again")
	}
}

// The panel pushes a config every few seconds until the agent runs it; each
// failed try interrupts every user, so the same failed config waits a while.
func TestCore_DoesNotRetryAFailedConfigRightAway(t *testing.T) {
	apiPort, port := freeTCPPort(t), freeTCPPort(t)
	c := startCore(t, testConfig(t, apiPort, port, vlessUser("alice", 1)))
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	takenPort := taken.Addr().(*net.TCPAddr).Port
	bad := testConfig(t, freeTCPPort(t), takenPort, vlessUser("alice", 1))
	if _, err := c.apply(bad, "bad", false); err == nil {
		t.Fatal("first try must fail")
	}
	taken.Close()

	restarted, err := c.apply(bad, "bad", false)
	if err == nil || restarted {
		t.Fatalf("retry right away = restarted %v, %v; want the earlier refusal without touching the core", restarted, err)
	}

	c.retryRefusedAfter = 0
	if _, err := c.apply(bad, "bad", false); err != nil {
		t.Fatalf("once the wait is over the config must be tried again: %v", err)
	}
	if !listening(takenPort) {
		t.Fatal("the retried config must be running")
	}
}

// The panel's template names log files on the panel's own disk; an agent writes
// Xray's errors to its console instead, so those paths must not stop its core.
func TestCore_IgnoresThePanelsLogFiles(t *testing.T) {
	apiPort, port := freeTCPPort(t), freeTCPPort(t)
	var cfg map[string]any
	if err := json.Unmarshal(testConfig(t, apiPort, port, vlessUser("alice", 1)), &cfg); err != nil {
		t.Fatal(err)
	}
	cfg["log"] = map[string]any{"access": "/var/log/x-ui-missing/access.log", "error": "/var/log/x-ui-missing/error.log", "loglevel": "warning"}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}

	c := newXrayCore()
	t.Cleanup(c.stop)
	if _, err := c.apply(raw, "h0", false); err != nil {
		t.Fatalf("a template with panel-side log paths must still start on the agent: %v", err)
	}
	if !listening(port) {
		t.Fatal("the inbound must be serving")
	}
}
