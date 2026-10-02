package controller

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service/probetest"
)

func TestAddHostLinksOnlyAnUnclaimedLiteServer(t *testing.T) {
	engine := newNodeCredentialTestEngine(t)
	lite := probetest.NewLite(t)
	lite.Answer(`{"example-lite":{"name":"Example host"}}`, `{}`)
	if _, err := (&service.ProbeService{}).SaveSettings(service.ProbeSettings{URL: lite.URL}); err != nil {
		t.Fatal(err)
	}
	add := func(name, server string) apiReply {
		return postJSON(t, engine, "/panel/api/nodes/add", map[string]any{
			"name": name, "kind": "agent", "address": "203.0.113.42", "enable": true, "probeServerId": server,
		})
	}
	if reply := add("first", "example-lite"); !reply.Success {
		t.Fatalf("add probe host: %s", reply.Msg)
	}
	var host model.Node
	if err := database.GetDB().Where("name = ?", "first").First(&host).Error; err != nil {
		t.Fatal(err)
	}
	var link model.ProbeLink
	if err := database.GetDB().Where("node_id = ?", host.Id).First(&link).Error; err != nil || link.ServerId != "example-lite" {
		t.Fatalf("host not linked to chosen server: %+v, %v", link, err)
	}
	for _, server := range []string{"example-lite", "absent-lite"} {
		if reply := add("rejected", server); reply.Success {
			t.Fatalf("accepted unavailable probe server %q", server)
		}
	}
	var count int64
	database.GetDB().Model(&model.Node{}).Count(&count)
	if count != 1 {
		t.Fatalf("failed linking left %d hosts, want 1", count)
	}
}

func TestAgentCanWaitForItsFirstConnectionAddress(t *testing.T) {
	engine := newNodeCredentialTestEngine(t)
	reply := postJSON(t, engine, "/panel/api/nodes/add", map[string]any{
		"name": "waiting", "kind": "agent", "address": "", "enable": true,
	})
	if !reply.Success {
		t.Fatalf("agent without address rejected: %s", reply.Msg)
	}
}

func TestAgentConnectionFillsAddressOnceAfterAuthentication(t *testing.T) {
	srv, hub := agentConnectServer(t)
	host := &model.Node{Name: "waiting", Kind: model.NodeKindAgent, Enable: true}
	if err := database.GetDB().Create(host).Error; err != nil {
		t.Fatal(err)
	}
	secret, err := (&service.NodeService{}).MintAgentSecret(host.Id)
	if err != nil {
		t.Fatal(err)
	}
	for i, address := range []string{"203.0.113.42", "198.51.100.11"} {
		header := http.Header{"Authorization": {"Bearer " + secret}, "X-Real-Ip": {address}}
		conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/base/agent/connect", header)
		if err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(time.Second)
		for len(hub.Connected()) == 0 && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		var stored model.Node
		if err := database.GetDB().First(&stored, host.Id).Error; err != nil {
			t.Fatal(err)
		}
		conn.Close()
		hub.Disconnect(host.Id)
		if stored.Address != "203.0.113.42" {
			t.Fatalf("connection %d: address %q, want first public address", i, stored.Address)
		}
	}
}
