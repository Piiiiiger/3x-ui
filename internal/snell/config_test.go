package snell

import (
	"encoding/json"
	"strings"
	"testing"
)

func testInstance() Instance {
	return Instance{ID: 7, Tag: "n3-in-26163-tcpudp", Listen: "::", Port: 26163, Settings: Settings{PSK: "0123456789abcdef0123456789abcdef", Version: 5, Reuse: true}}
}

func TestSplitKeepsXrayAndRemovesSecrets(t *testing.T) {
	i := testInstance()
	raw, _ := json.Marshal(map[string]any{"inbounds": []any{}, ConfigKey: []Instance{i}})
	core, got, err := Split(raw)
	if err != nil || len(got) != 1 || got[0] != i {
		t.Fatalf("split: %v", err)
	}
	if strings.Contains(string(core), "psk") || strings.Contains(string(core), ConfigKey) {
		t.Fatal("sidecar credentials leaked into core")
	}
	if !strings.Contains(i.ServerConfig(), "listen = [::]:26163\n") {
		t.Fatal("IPv6 listener not encoded correctly")
	}
	_, got, err = Split([]byte(`{"inbounds":[]}`))
	if err != nil || len(got) != 0 {
		t.Fatal("removal must produce empty desired set")
	}
}

func TestRejectUnsafeSnellConfigs(t *testing.T) {
	for _, change := range []func(*Instance){func(i *Instance) { i.Settings.PSK = "secret\nlisten = 0.0.0.0:22" }, func(i *Instance) { i.Settings.PSK = "short" }, func(i *Instance) { i.Settings.Version = 4 }, func(i *Instance) { i.Listen = "/tmp/socket" }, func(i *Instance) { i.Port = 65536 }, func(i *Instance) { i.ID = -1 }} {
		i := testInstance()
		change(&i)
		if i.Validate() == nil {
			t.Fatal("accepted invalid configuration")
		}
	}
	i := testInstance()
	raw, _ := json.Marshal(map[string]any{ConfigKey: []Instance{i, i}})
	if _, _, err := Split(raw); err == nil {
		t.Fatal("accepted duplicate listener")
	}
}
