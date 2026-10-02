package sub

import (
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func seedTunnelSubInbound(t *testing.T, protocol model.Protocol, tag, subID, email, settings string, port int) *model.Inbound {
	t.Helper()
	db := database.GetDB()
	ib := &model.Inbound{
		UserId: 1, Tag: tag, Enable: true, Listen: "203.0.113.5", Port: port,
		Protocol: protocol, Remark: tag, Settings: settings,
	}
	if err := db.Create(ib).Error; err != nil {
		t.Fatalf("create %s: %v", tag, err)
	}
	rec := &model.ClientRecord{Email: email, SubID: subID, Enable: true}
	if err := db.Create(rec).Error; err != nil {
		t.Fatalf("create client: %v", err)
	}
	if err := db.Create(&model.ClientInbound{ClientId: rec.Id, InboundId: ib.Id}).Error; err != nil {
		t.Fatalf("link client: %v", err)
	}
	return ib
}

func setCustomShare(t *testing.T, ib *model.Inbound, addr string, port int) {
	t.Helper()
	updates := map[string]any{"share_addr_strategy": "custom", "share_addr": addr, "share_port": port}
	if err := database.GetDB().Model(&model.Inbound{}).Where("id = ?", ib.Id).Updates(updates).Error; err != nil {
		t.Fatalf("set share address: %v", err)
	}
}

// A WireGuard link fans out over advertised endpoints rather than a stream, so the
// public port must reach it too, leaving its name and keys as they were.
func TestGetSubs_WireGuardAdvertisesThePublicPort(t *testing.T) {
	initSubDB(t)
	serverPriv, _ := mustWireguardKeypair(t)
	clientPriv, _ := mustWireguardKeypair(t)
	const email, subID = "alice@wg", "sub-wg-public"
	settings := fmt.Sprintf(`{"secretKey":%q,"clients":[{"email":%q,"privateKey":%q,"allowedIPs":["10.0.0.2/32"],"enable":true}]}`,
		serverPriv, email, clientPriv)
	ib := seedTunnelSubInbound(t, model.WireGuard, "wg-in", subID, email, settings, 51820)
	setCustomShare(t, ib, "wg.example.com", 0)
	before := wireguardSubLink(t, subID)

	setCustomShare(t, ib, "wg.example.com", 443)
	after := wireguardSubLink(t, subID)

	if after.Host != "wg.example.com:443" || before.Host != "wg.example.com:51820" {
		t.Fatalf("endpoint %s before and %s after, want wg.example.com:51820 then :443", before.Host, after.Host)
	}
	if after.Fragment != before.Fragment || after.User.Username() != clientPriv {
		t.Fatalf("link #%s key %q, want the name %q and the client's key kept", after.Fragment, after.User.Username(), before.Fragment)
	}
}

func wireguardSubLink(t *testing.T, subID string) *url.URL {
	t.Helper()
	links, _, _, _, err := NewSubService("").GetSubs(subID, "sub.example.com")
	if err != nil {
		t.Fatalf("GetSubs: %v", err)
	}
	parts := splitLinkLines(strings.Join(links, "\n"))
	if len(parts) != 1 {
		t.Fatalf("links = %d, want 1: %v", len(parts), parts)
	}
	return parseWireguardSubLink(t, parts[0])
}

// The AmneziaWG vpn:// payload carries the endpoint inside its .conf text, so the
// public port must reach the Endpoint line, not only the URL.
func TestGetSubs_AmneziaWGAdvertisesThePublicPort(t *testing.T) {
	initSubDB(t)
	serverPriv, serverPub := mustWireguardKeypair(t)
	clientPriv, _ := mustWireguardKeypair(t)
	const email, subID = "alice@awg", "sub-awg-public"
	settings := fmt.Sprintf(`{"server":{"privateKey":%q,"publicKey":%q,"mtu":1420},"clients":[{"email":%q,"privateKey":%q,"allowedIPs":["10.8.0.2/32"],"enable":true}]}`,
		serverPriv, serverPub, email, clientPriv)
	ib := seedTunnelSubInbound(t, model.AmneziaWG, "awg-in", subID, email, settings, 51821)
	setCustomShare(t, ib, "awg.example.com", 8443)

	links, _, _, _, err := NewSubService("").GetSubs(subID, "sub.example.com")
	if err != nil {
		t.Fatalf("GetSubs: %v", err)
	}
	parts := splitLinkLines(strings.Join(links, "\n"))
	if len(parts) != 1 {
		t.Fatalf("links = %d, want 1: %v", len(parts), parts)
	}
	conf := decodeAmneziaWGSubLink(t, parts[0])
	for _, line := range []string{"Endpoint = awg.example.com:8443", "PrivateKey = " + clientPriv} {
		if !strings.Contains(conf, line) {
			t.Fatalf("config missing %q\n%s", line, conf)
		}
	}
	if strings.Contains(conf, "51821") {
		t.Fatalf("config still advertises the inbound port\n%s", conf)
	}
}
