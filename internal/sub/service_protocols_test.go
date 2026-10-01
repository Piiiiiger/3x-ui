package sub

import (
	"slices"
	"strconv"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// A subscription lists only the protocols that have a share link. The probe
// reuses this list to decide which servers a client owns, so it must not drift.
func TestGetInboundsBySubIdKeepsOnlyLinkBearingProtocols(t *testing.T) {
	initSubDB(t)
	db := database.GetDB()
	rec := &model.ClientRecord{Email: "u@proto", SubID: "subproto", Enable: true}
	if err := db.Create(rec).Error; err != nil {
		t.Fatalf("create client: %v", err)
	}
	every := []model.Protocol{
		model.VMESS, model.VLESS, model.Tunnel, model.HTTP, model.Trojan, model.Shadowsocks,
		model.Mixed, model.WireGuard, model.Hysteria, model.MTProto, model.AmneziaWG, model.TUIC,
	}
	for i, protocol := range every {
		in := &model.Inbound{
			Port: 30100 + i, Protocol: protocol, Enable: true,
			Tag: "proto-" + strconv.Itoa(i), Settings: `{"clients":[]}`,
		}
		if err := db.Create(in).Error; err != nil {
			t.Fatalf("create %s inbound: %v", protocol, err)
		}
		if err := db.Create(&model.ClientInbound{ClientId: rec.Id, InboundId: in.Id}).Error; err != nil {
			t.Fatalf("attach %s inbound: %v", protocol, err)
		}
	}

	inbounds, err := (&SubService{}).getInboundsBySubId("subproto")
	if err != nil {
		t.Fatalf("getInboundsBySubId: %v", err)
	}
	got := make([]string, 0, len(inbounds))
	for _, in := range inbounds {
		got = append(got, string(in.Protocol))
	}
	slices.Sort(got)
	want := []string{"amneziawg", "hysteria", "mtproto", "shadowsocks", "trojan", "tuic", "vless", "vmess", "wireguard"}
	if !slices.Equal(got, want) {
		t.Fatalf("subscription protocols = %v, want %v", got, want)
	}
}
