package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConnectURL(t *testing.T) {
	cases := map[string]string{
		"https://panel.example.com/a1b2c3d4/": "wss://panel.example.com/a1b2c3d4/agent/connect",
		"https://panel.example.com/a1b2c3d4":  "wss://panel.example.com/a1b2c3d4/agent/connect",
		"http://127.0.0.1:12890/":             "ws://127.0.0.1:12890/agent/connect",
	}
	for master, want := range cases {
		got, err := Config{Master: master}.connectURL()
		if err != nil || got != want {
			t.Errorf("connectURL(%q) = %q, %v; want %q", master, got, err, want)
		}
	}
	for _, bad := range []string{"panel.example.com", "ftp://panel.example.com/", "https:///nohost"} {
		if _, err := (Config{Master: bad}).connectURL(); err == nil {
			t.Errorf("connectURL(%q) accepted a URL the agent cannot dial", bad)
		}
	}
}

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	cfg, err := LoadConfig(write("ok.json", `{"master":"https://panel.example.com/x/","secret":"s3cret"}`))
	if err != nil {
		t.Fatalf("valid config: %v", err)
	}
	if cfg.StateDir != defaultStateDir {
		t.Fatalf("state dir = %q, want the default %q", cfg.StateDir, defaultStateDir)
	}
	if _, err := LoadConfig(write("nosecret.json", `{"master":"https://panel.example.com/x/","secret":"  "}`)); err == nil {
		t.Fatal("a config without a secret must be refused at start, not by the panel later")
	}
}
