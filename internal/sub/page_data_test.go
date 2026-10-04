package sub

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

func TestVisibleSubscriptionInboundsHideRelayDependencies(t *testing.T) {
	seedSubDB(t)
	db := database.GetDB()
	relay := seedSubInbound(t, "s1", "香港-Lazycat", 4901, 1, tcpStream)
	target := seedSubInbound(t, "s1", "新加坡-Titan-1", 4902, 2, tcpStream)
	chain := model.ProxyChain{TargetInboundId: target.Id, RelayInboundId: relay.Id, Enabled: true}
	if err := db.Create(&chain).Error; err != nil {
		t.Fatal(err)
	}
	keys, _ := json.Marshal([]string{model.PlanNodeKey(target.Id, false), model.PlanChainKey(target.Id, chain.Id)})
	plan := model.Plan{Name: "visible", NodeKeys: string(keys)}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.ClientRecord{}).Where("sub_id = ?", "s1").Update("plan_id", plan.Id).Error; err != nil {
		t.Fatal(err)
	}
	visible := (&SubService{}).visibleSubscriptionInbounds("s1", []*model.Inbound{relay, target})
	if len(visible) != 1 || visible[0].Id != target.Id {
		t.Fatalf("visible inbounds = %v, want only target %d", visible, target.Id)
	}
}

// A single getSubs entry can hold several links (one per host of an inbound)
// joined by newlines. BuildPageData must split them into one entry per link, with
// the email replicated, so the subpage renders one row per host instead of
// collapsing them onto a single mangled line.
func TestBuildPageData_SplitsMultiHostLinks(t *testing.T) {
	s := &SubService{}
	subs := []string{
		"vless://a@h1:443?type=tcp#DE-john@x\nvless://a@h2:443?type=tcp#DE-john@x\nvless://a@h3:443?type=tcp#DE-john@x",
		"vless://b@h:443?type=tcp#FR-alice@x",
	}
	emails := []string{"john@x", "alice@x"}

	page := s.BuildPageData("s1", "", xray.ClientTraffic{}, 0, subs, emails, "", "", "", "/", "", "")

	if len(page.Result) != 4 {
		t.Fatalf("Result len = %d, want 4 (3 host links + 1 single link)", len(page.Result))
	}
	for i, link := range page.Result {
		if strings.Contains(link, "\n") {
			t.Fatalf("Result[%d] still multi-line: %q", i, link)
		}
	}
	wantEmails := []string{"john@x", "john@x", "john@x", "alice@x"}
	if !reflect.DeepEqual(page.Emails, wantEmails) {
		t.Fatalf("Emails = %v, want %v", page.Emails, wantEmails)
	}
}

func TestSubIsOnline(t *testing.T) {
	tests := []struct {
		name   string
		sub    []string
		online []string
		want   bool
	}{
		{name: "nobody online", sub: []string{"a@x"}, online: nil, want: false},
		{name: "no sub emails", sub: nil, online: []string{"a@x"}, want: false},
		{name: "sub client online", sub: []string{"a@x"}, online: []string{"z@x", "a@x"}, want: true},
		{name: "only other clients online", sub: []string{"a@x"}, online: []string{"z@x"}, want: false},
		{name: "any of several sub entries online", sub: []string{"a@x", "b@x"}, online: []string{"b@x"}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := subIsOnline(tt.sub, tt.online); got != tt.want {
				t.Fatalf("subIsOnline(%v, %v) = %v, want %v", tt.sub, tt.online, got, tt.want)
			}
		})
	}
}

func TestBuildPageData_IsOnlineFalseWithoutLiveConnections(t *testing.T) {
	s := &SubService{}

	page := s.BuildPageData("s1", "", xray.ClientTraffic{}, 0, []string{"vless://a@h1:443?type=tcp#DE-john@x"}, []string{"john@x"}, "", "", "", "/", "", "")

	if page.IsOnline {
		t.Fatal("IsOnline must be false when the subscription's client has no live connection")
	}
}

func TestChainEndpointsUsesPublicIPv4AndNATPort(t *testing.T) {
	seedSubDB(t)
	node := model.Node{Name: "ipv6-host", Address: "2001:db8::1"}
	if err := database.GetDB().Create(&node).Error; err != nil {
		t.Fatal(err)
	}
	inbound := model.Inbound{NodeID: &node.Id, Port: 19336, SharePort: 29336, ShareAddrStrategy: "custom", ShareAddr: "192.0.2.50", StreamSettings: `{}`}
	eps, err := ChainEndpoints(&inbound)
	if err != nil {
		t.Fatal(err)
	}
	if len(eps) != 1 || eps[0].Address != "192.0.2.50" || eps[0].Port != 29336 {
		t.Fatalf("wrong public endpoint: %+v", eps)
	}
	inbound.StreamSettings = `{"externalProxy":[{"dest":"192.0.2.60","port":443}]}`
	eps, err = ChainEndpoints(&inbound)
	if err != nil {
		t.Fatal(err)
	}
	if len(eps) != 1 || eps[0].Address != "192.0.2.60" || eps[0].Port != 443 {
		t.Fatalf("external endpoint ignored: %+v", eps)
	}
}
