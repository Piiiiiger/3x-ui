package sub

import (
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func TestSnellClashUsesSharedKeyAndPublicEndpoint(t *testing.T) {
	svc := &SubClashService{SubService: &SubService{}}
	ib := &model.Inbound{Protocol: model.Snell, Listen: "203.0.113.7", Port: 26163, Remark: "HK-Snell", Settings: `{"psk":"0123456789abcdef0123456789abcdef","version":5,"reuse":true}`}
	proxy := svc.buildProxy(svc.SubService, ib, model.Client{Email: "alice", Password: "unrelated-user-password"}, nil, nil)
	for k, want := range map[string]any{"type": "snell", "server": "203.0.113.7", "port": 26163, "version": 5, "psk": "0123456789abcdef0123456789abcdef", "udp": true, "reuse": true} {
		if proxy[k] != want {
			t.Fatalf("wrong %s", k)
		}
	}
	ib.Settings = `{"psk":"bad","version":5}`
	if svc.buildProxy(svc.SubService, ib, model.Client{}, nil, nil) != nil {
		t.Fatal("invalid secret exported")
	}
}

func TestSnellShareLinkUsesTheSharedServerCredential(t *testing.T) {
	svc := &SubService{}
	ib := &model.Inbound{Id: 91, Protocol: model.Snell, Listen: "203.0.113.7", Port: 26163, Remark: "HK-Snell", Settings: `{"psk":"0123456789abcdef0123456789abcdef","version":5,"reuse":true}`}
	svc.primeLinkClients(ib.Id, []model.Client{{Email: "tom"}}, true)
	link := svc.GetLink(ib, "tom")
	for _, want := range []string{"snell://203.0.113.7:26163", "psk=0123456789abcdef0123456789abcdef", "version=5", "reuse=true", "#HK-Snell"} {
		if !strings.Contains(link, want) {
			t.Fatalf("link %q missing %q", link, want)
		}
	}
}
