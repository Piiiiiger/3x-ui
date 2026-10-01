package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/agentproto"
	"github.com/mhsanaei/3x-ui/v3/internal/agentproto/agenttest"
	"github.com/mhsanaei/3x-ui/v3/internal/web/runtime"

	"github.com/gorilla/websocket"
)

const e2eNode = 1

// proxyConfig is a panel-style config whose client inbound is an HTTP proxy, so
// the test can move real bytes through the agent's core as user alice.
func proxyConfig(t *testing.T, apiPort, proxyPort int) []byte {
	t.Helper()
	cfg := map[string]any{
		"log": map[string]any{"loglevel": "error"},
		"api": map[string]any{"services": []string{"HandlerService", "StatsService"}, "tag": "api"},
		"inbounds": []any{
			map[string]any{"listen": "127.0.0.1", "port": apiPort, "protocol": "tunnel", "settings": map[string]any{"rewriteAddress": "127.0.0.1"}, "tag": "api"},
			map[string]any{
				"listen": "127.0.0.1", "port": proxyPort, "protocol": "http", "tag": "n1-proxy",
				"settings": map[string]any{"accounts": []any{map[string]any{"user": "alice", "pass": "pw"}}},
			},
		},
		"outbounds": []any{map[string]any{"protocol": "freedom", "tag": "direct"}},
		"policy": map[string]any{
			"levels": map[string]any{"0": map[string]any{"statsUserUplink": true, "statsUserDownlink": true, "statsUserOnline": true}},
			"system": map[string]any{"statsInboundUplink": true, "statsInboundDownlink": true},
		},
		"routing": map[string]any{"rules": []any{map[string]any{"type": "field", "inboundTag": []string{"api"}, "outboundTag": "api"}}},
		"stats":   map[string]any{},
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

type panelSide struct {
	hub     *runtime.AgentHub
	master  string
	mu      sync.Mutex
	reports []agentproto.Traffic
}

func startPanel(t *testing.T) *panelSide {
	t.Helper()
	p := &panelSide{hub: runtime.NewAgentHub()}
	p.hub.SetHandlers(runtime.AgentHandlers{Traffic: func(_ int, tr *agentproto.Traffic) error {
		p.mu.Lock()
		defer p.mu.Unlock()
		p.reports = append(p.reports, *tr)
		return nil
	}})
	url := agenttest.Server(t, func(conn *websocket.Conn) { p.hub.Attach(e2eNode, conn) })
	p.master = "http" + strings.TrimPrefix(url, "ws") + "/"
	return p
}

func (p *panelSide) downFor(email string) int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	var total int64
	for _, r := range p.reports {
		for _, c := range r.Clients {
			if c.Name == email {
				total += c.Down
			}
		}
	}
	return total
}

func runAgent(t *testing.T, cfg Config) *Agent {
	t.Helper()
	a, err := New(cfg, "test")
	if err != nil {
		t.Fatal(err)
	}
	a.interval = 100 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		a.Run(ctx)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	return a
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func connected(hub *runtime.AgentHub) func() bool {
	return func() bool { _, ok := hub.Session(e2eNode); return ok }
}

func fetchThroughProxy(t *testing.T, proxyPort, size int) {
	t.Helper()
	backend, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(make([]byte, size))
	})}
	go func() { _ = srv.Serve(backend) }()
	defer srv.Close()
	proxyURL, _ := url.Parse(fmt.Sprintf("http://alice:pw@127.0.0.1:%d", proxyPort))
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}, Timeout: 10 * time.Second}
	resp, err := client.Get("http://" + backend.Addr().String() + "/")
	if err != nil {
		t.Fatalf("request through the agent's proxy: %v", err)
	}
	n, _ := io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if n != int64(size) {
		t.Fatalf("proxied %d bytes, want %d", n, size)
	}
}

// The whole loop: the panel pushes a config, users move bytes through the agent,
// and the panel receives that usage once, acked, plus who is online.
func TestAgent_ReportsRealTrafficToThePanel(t *testing.T) {
	const payload = 256 * 1024
	panel := startPanel(t)
	a := runAgent(t, Config{Master: panel.master, Secret: "s", StateDir: t.TempDir()})
	eventually(t, "the agent to connect", connected(panel.hub))

	apiPort, proxyPort := freeTCPPort(t), freeTCPPort(t)
	res, err := panel.hub.Apply(context.Background(), e2eNode, agentproto.Apply{Config: proxyConfig(t, apiPort, proxyPort), Hash: "h1"})
	if err != nil || !res.OK {
		t.Fatalf("apply = %+v, %v", res, err)
	}

	fetchThroughProxy(t, proxyPort, payload)

	eventually(t, "alice's usage at the panel", func() bool { return panel.downFor("alice") >= payload })
	eventually(t, "the report to be acked", func() bool { return a.outbox.next() == nil })
	if got := panel.downFor("alice"); got > payload+64*1024 {
		t.Fatalf("panel counted %d bytes for a %d byte download; a report was counted twice", got, payload)
	}
	eventually(t, "status with alice online", func() bool {
		st, ok := panel.hub.Session(e2eNode)
		return ok && st.Status.XrayState == "running" && st.Status.ConfigHash == "h1" && slices.Contains(st.Status.Online, "alice")
	})
}

func TestAgent_ReconnectsWhenThePanelDropsIt(t *testing.T) {
	panel := startPanel(t)
	runAgent(t, Config{Master: panel.master, Secret: "s", StateDir: t.TempDir()})
	eventually(t, "the agent to connect", connected(panel.hub))
	first, _ := panel.hub.Session(e2eNode)

	panel.hub.Disconnect(e2eNode)

	eventually(t, "a new session", func() bool {
		st, ok := panel.hub.Session(e2eNode)
		return ok && st.ConnectedAt.After(first.ConnectedAt)
	})
}

// Users must keep working when the agent restarts while the panel is down.
func TestAgent_RestartsOnTheLastConfigWithoutThePanel(t *testing.T) {
	panel := startPanel(t)
	stateDir := t.TempDir()
	apiPort, proxyPort := freeTCPPort(t), freeTCPPort(t)
	a, err := New(Config{Master: panel.master, Secret: "s", StateDir: stateDir}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if res := a.applyConfig(agentproto.Apply{Config: proxyConfig(t, apiPort, proxyPort), Hash: "h1"}); !res.OK {
		t.Fatalf("apply: %+v", res)
	}
	a.core.stop()

	unreachable := fmt.Sprintf("http://127.0.0.1:%d/", freeTCPPort(t))
	runAgent(t, Config{Master: unreachable, Secret: "s", StateDir: stateDir})
	eventually(t, "the saved config to serve", func() bool { return listening(proxyPort) })
	fetchThroughProxy(t, proxyPort, 1024)
}
