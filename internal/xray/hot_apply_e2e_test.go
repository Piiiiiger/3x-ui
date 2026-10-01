package xray

import (
	"encoding/json"
	"fmt"
	"testing"
)

func vlessClient(email string, n int) map[string]any {
	return map[string]any{"id": fmt.Sprintf("a17e367c-2074-4d3e-aaeb-fbef5dfde7%02d", n), "email": email}
}

func e2eVlessInbound(t *testing.T, tag string, port int, clients ...map[string]any) InboundConfig {
	t.Helper()
	if clients == nil {
		clients = []map[string]any{}
	}
	settings, err := json.Marshal(map[string]any{"clients": clients, "decryption": "none"})
	if err != nil {
		t.Fatal(err)
	}
	return InboundConfig{
		Listen:   []byte(`"127.0.0.1"`),
		Port:     port,
		Protocol: "vless",
		Settings: settings,
		Tag:      tag,
	}
}

func userPresent(t *testing.T, api *XrayAPI, tag, email string) bool {
	t.Helper()
	err := api.AddUser("vless", tag, panelUser(email, map[string]any{"id": "0f6ad6a7-5d56-4a0b-9d4b-6e4c5f0a1b2c"}))
	if err == nil {
		if err := api.RemoveUser(tag, email); err != nil {
			t.Fatalf("undo probe user %s: %v", email, err)
		}
		return false
	}
	if IsUserExistsErr(err) {
		return true
	}
	t.Fatalf("probe user %s on %s: %v", email, tag, err)
	return false
}

// A diff computed against a stale snapshot re-adds users the core already
// holds; ApplyHotDiff must replace them instead of failing the whole apply.
func TestApplyHotDiff_E2E_ReAddedUserIsReplaced(t *testing.T) {
	port := freePort(t)
	c := startE2ECore(t, []any{e2eVlessInbound(t, "in-a", port, vlessClient("alice", 1))})

	oldCfg := &Config{InboundConfigs: []InboundConfig{e2eVlessInbound(t, "in-a", port)}}
	newCfg := &Config{InboundConfigs: []InboundConfig{e2eVlessInbound(t, "in-a", port, vlessClient("alice", 1))}}
	diff, ok := ComputeHotDiff(oldCfg, newCfg)
	if !ok || len(diff.AddedUsers) != 1 {
		t.Fatalf("expected one added user, got ok=%v diff=%+v", ok, diff)
	}

	if err := ApplyHotDiff(c.api, diff); err != nil {
		t.Fatalf("ApplyHotDiff with an already-present user: %v", err)
	}
	if !userPresent(t, c.api, "in-a", "alice") {
		t.Fatal("alice must still be on in-a after the replace")
	}
}

// An edited client arrives as a removal plus an addition of the same email; run
// the other way round, the removal would delete the freshly added user.
func TestApplyHotDiff_E2E_EditedUserSurvives(t *testing.T) {
	port := freePort(t)
	c := startE2ECore(t, []any{e2eVlessInbound(t, "in-a", port, vlessClient("alice", 1), vlessClient("bob", 2))})

	oldCfg := &Config{InboundConfigs: []InboundConfig{e2eVlessInbound(t, "in-a", port, vlessClient("alice", 1), vlessClient("bob", 2))}}
	newCfg := &Config{InboundConfigs: []InboundConfig{e2eVlessInbound(t, "in-a", port, vlessClient("alice", 7))}}
	diff, ok := ComputeHotDiff(oldCfg, newCfg)
	if !ok || len(diff.AddedUsers) != 1 || len(diff.RemovedUsers) != 2 {
		t.Fatalf("expected alice re-added and alice+bob removed, got ok=%v diff=%+v", ok, diff)
	}

	if err := ApplyHotDiff(c.api, diff); err != nil {
		t.Fatalf("ApplyHotDiff with an edited user: %v", err)
	}
	if !userPresent(t, c.api, "in-a", "alice") {
		t.Fatal("alice was edited, not removed, and must still be on in-a")
	}
	if userPresent(t, c.api, "in-a", "bob") {
		t.Fatal("bob was dropped from the config and must not be on in-a")
	}
}
