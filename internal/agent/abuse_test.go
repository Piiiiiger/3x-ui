package agent

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/abuse"
	"github.com/mhsanaei/3x-ui/v3/internal/agentproto"
)

func (p *panelSide) signals() []abuse.Signal {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []abuse.Signal
	for _, r := range p.reports {
		out = append(out, r.Abuse...)
	}
	return out
}

// sweepRules watch only for one address's ports, at a level a test can reach.
func sweepRules() abuse.Rules {
	return abuse.Rules{ScanPortsOnIP: 3, ScanWindowMin: 5}
}

const abuseUserID = "5f8bd2c4-1c58-4d0e-9a21-6c3a8e47d915"

// vlessConfig is a panel-style config serving alice over plain VLESS, the
// protocol whose connection records name the user.
func vlessConfig(t *testing.T, apiPort, port int) []byte {
	t.Helper()
	return mustJSON(t, map[string]any{
		"log": map[string]any{"loglevel": "error"},
		"api": map[string]any{"services": []string{"HandlerService", "StatsService"}, "tag": "api"},
		"inbounds": []any{
			map[string]any{"listen": "127.0.0.1", "port": apiPort, "protocol": "tunnel", "settings": map[string]any{"rewriteAddress": "127.0.0.1"}, "tag": "api"},
			map[string]any{
				"listen": "127.0.0.1", "port": port, "protocol": "vless", "tag": "n1-vless",
				"settings":       map[string]any{"clients": []any{map[string]any{"id": abuseUserID, "email": "alice"}}, "decryption": "none"},
				"streamSettings": map[string]any{"network": "tcp"},
			},
		},
		"outbounds": []any{map[string]any{"protocol": "freedom", "tag": "direct", "settings": map[string]any{
			"finalRules": []any{map[string]any{"action": "allow", "ip": []string{"127.0.0.0/8"}}},
		}}},
		"policy":  map[string]any{"levels": map[string]any{"0": map[string]any{"statsUserUplink": true, "statsUserDownlink": true}}},
		"routing": map[string]any{"rules": []any{map[string]any{"type": "field", "inboundTag": []string{"api"}, "outboundTag": "api"}}},
		"stats":   map[string]any{},
	})
}

// startVlessClient runs a client core reaching the agent's VLESS inbound as alice.
// It starts first: a process has one Xray logger, and the agent must claim it last.
func startVlessClient(t *testing.T, serverPort int) int {
	t.Helper()
	socksPort := freeTCPPort(t)
	startCore(t, mustJSON(t, map[string]any{
		"log":      map[string]any{"loglevel": "error"},
		"inbounds": []any{map[string]any{"listen": "127.0.0.1", "port": socksPort, "protocol": "socks", "settings": map[string]any{"udp": false}}},
		"outbounds": []any{map[string]any{"protocol": "vless", "settings": map[string]any{"vnext": []any{map[string]any{
			"address": "127.0.0.1", "port": serverPort,
			"users": []any{map[string]any{"id": abuseUserID, "encryption": "none"}},
		}}}}},
	}))
	return socksPort
}

// visitPorts fetches from n local web servers, each on its own port, through socks.
func visitPorts(t *testing.T, socksPort, n int) {
	t.Helper()
	proxyURL, _ := url.Parse(fmt.Sprintf("socks5://127.0.0.1:%d", socksPort))
	client := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	for range n {
		site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "ok") }))
		resp, err := client.Get(site.URL)
		if err != nil {
			site.Close()
			t.Fatalf("request through the agent's VLESS inbound: %v", err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		site.Close()
	}
}

func applyTo(t *testing.T, panel *panelSide, raw []byte) {
	t.Helper()
	if res, err := panel.hub.Apply(context.Background(), e2eNode, agentproto.Apply{Config: raw, Hash: "h1"}); err != nil || !res.OK {
		t.Fatalf("apply = %+v, %v", res, err)
	}
}

// The whole path: the hook sees each VLESS connection inside the running core,
// the detector trips, and the signal reaches the panel in a traffic report.
func TestAgent_ReportsAbuseItSeesInTheCore(t *testing.T) {
	panel := startPanel(t)
	apiPort, serverPort := freeTCPPort(t), freeTCPPort(t)
	socksPort := startVlessClient(t, serverPort)
	a := runAgent(t, Config{Master: panel.master, Secret: "s", StateDir: t.TempDir()})
	eventually(t, "the agent to connect", connected(panel.hub))
	raw, err := abuse.JoinConfig(vlessConfig(t, apiPort, serverPort), sweepRules())
	if err != nil {
		t.Fatal(err)
	}
	applyTo(t, panel, raw)

	visitPorts(t, socksPort, 3)

	eventually(t, "a scan signal at the panel", func() bool { return len(panel.signals()) > 0 })
	s := panel.signals()[0]
	if s.Email != "alice" || s.Rule != abuse.RuleScan || s.Measure != abuse.MeasurePorts || s.Count < 3 {
		t.Fatalf("signal = %+v, want alice's port sweep", s)
	}
	eventually(t, "the report to be acked", func() bool { return a.outbox.next() == nil })
}

// A host the panel turns detection off for stops watching at once, though the
// same traffic would trip the rules it had a moment ago.
func TestAgent_StopsDetectingWhenThePanelTurnsItOff(t *testing.T) {
	panel := startPanel(t)
	apiPort, serverPort := freeTCPPort(t), freeTCPPort(t)
	socksPort := startVlessClient(t, serverPort)
	a := runAgent(t, Config{Master: panel.master, Secret: "s", StateDir: t.TempDir()})
	eventually(t, "the agent to connect", connected(panel.hub))
	watched, err := abuse.JoinConfig(vlessConfig(t, apiPort, serverPort), sweepRules())
	if err != nil {
		t.Fatal(err)
	}
	applyTo(t, panel, watched)
	if res, err := panel.hub.Apply(context.Background(), e2eNode, agentproto.Apply{Config: vlessConfig(t, apiPort, serverPort), Hash: "h2"}); err != nil || !res.OK {
		t.Fatalf("apply without detection = %+v, %v", res, err)
	}

	visitPorts(t, socksPort, 3)

	eventually(t, "alice's usage at the panel", func() bool { return panel.downFor("alice") > 0 })
	eventually(t, "the report to be acked", func() bool { return a.outbox.next() == nil })
	if got := panel.signals(); len(got) != 0 {
		t.Fatalf("signals with detection off: %+v", got)
	}
}

// A signal waiting for the panel survives an agent restart and goes once.
func TestOutbox_KeepsSignalsUntilAcked(t *testing.T) {
	path := filepath.Join(t.TempDir(), "outbox.json")
	box, err := openOutbox(path)
	if err != nil {
		t.Fatal(err)
	}
	box.addSignals([]abuse.Signal{{Email: "alice", Rule: abuse.RuleSpam}})
	first := box.next()
	if first == nil || len(first.Abuse) != 1 {
		t.Fatalf("report = %+v, want the signal", first)
	}

	reopened, err := openOutbox(path)
	if err != nil {
		t.Fatal(err)
	}
	again := reopened.next()
	if again == nil || again.Seq != first.Seq || len(again.Abuse) != 1 {
		t.Fatalf("after a restart the report is %+v, want the same unacked one", again)
	}
	reopened.ack(again.Instance, again.Seq)
	if left := reopened.next(); left != nil {
		t.Fatalf("after the ack %+v is still queued", left)
	}
}
