package sub

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

func TestSubscriptionProbeRejectsUnknownTokensAndHandlesNoMonitor(t *testing.T) {
	router, _ := seedPortal(t)
	for _, tc := range []struct {
		token  string
		status int
	}{{"unknown", http.StatusNotFound}, {"s1", http.StatusOK}} {
		res := portalRequest(router, http.MethodGet, "/sub/"+tc.token+"/probe", "", "198.51.100.8", nil)
		if res.Code != tc.status || res.Header().Get("Cache-Control") != "no-cache, no-store, must-revalidate" {
			t.Fatalf("%s = %d %s", tc.token, res.Code, res.Body)
		}
		if res.Code == http.StatusOK {
			var probe service.PortalProbe
			if err := json.Unmarshal(res.Body.Bytes(), &probe); err != nil || probe.Enabled || len(probe.Servers) != 0 {
				t.Fatalf("no monitor = %s (%v)", res.Body, err)
			}
		}
	}
}

func TestSubscriptionDetailsUsesOnlyTheTokensOwnHostsAndReset(t *testing.T) {
	seedPortal(t)
	db := database.GetDB()
	if _, err := (&service.ProbeService{}).SaveSettings(service.ProbeSettings{URL: "http://127.0.0.1:18180"}); err != nil {
		t.Fatal(err)
	}
	node := &model.Node{Name: "edge", Address: "203.0.113.12", Kind: model.NodeKindAgent, Enable: true}
	if err := db.Create(node).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.Inbound{}).Where("tag = ?", "pb").Update("node_id", node.Id).Error; err != nil {
		t.Fatal(err)
	}
	if err := (&service.ProbeService{}).SetLinks(service.ProbeLinksInput{Links: []service.ProbeLinkInput{{NodeId: node.Id, ServerId: "invented-probe"}}}); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.ClientRecord{}).Where("sub_id = ?", "s1").Updates(map[string]any{"traffic_reset": "daily"}).Error; err != nil {
		t.Fatal(err)
	}
	controller := &SUBController{subPath: "/sub/"}
	loc, _ := (&service.SettingService{}).GetTimeLocation()
	now := time.Now().In(loc)
	tomorrow := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, loc).UnixMilli()
	if base, reset := controller.subscriptionDetails("s1"); base != "" || reset != tomorrow {
		t.Fatalf("pa details = %q/%d, want no probe and tomorrow", base, reset)
	}
	if base, reset := controller.subscriptionDetails("s2"); base != "/sub/s2" || reset != 0 {
		t.Fatalf("pb details = %q/%d, want its probe and no reset", base, reset)
	}
}

func TestSubscriptionProbeFiltersOtherUsersHosts(t *testing.T) {
	router, node := seedPortalProbe(t)
	linkProbe(t, service.ProbeLinkInput{NodeId: 0, ServerId: "uuid-a"}, service.ProbeLinkInput{NodeId: node, ServerId: "uuid-b"})
	for _, tc := range []struct {
		token string
		node  int
		cpu   float64
	}{{"s1", 0, 11}, {"s2", node, 22}} {
		res := portalRequest(router, http.MethodGet, "/sub/"+tc.token+"/probe", "", "198.51.100.8", nil)
		var probe service.PortalProbe
		if err := json.Unmarshal(res.Body.Bytes(), &probe); err != nil || res.Code != http.StatusOK || len(probe.Servers) != 1 || probe.Servers[0].Id != tc.node || probe.Servers[0].Cpu != tc.cpu {
			t.Fatalf("%s sees %s (%v)", tc.token, res.Body, err)
		}
	}
}
