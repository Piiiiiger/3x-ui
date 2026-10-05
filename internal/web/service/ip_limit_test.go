package service

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

// setupIpLimitTest gives each test a clean DB, a ban log of its own and no
// addresses on the panel host unless the test adds them.
func setupIpLimitTest(t *testing.T) {
	t.Helper()
	setupSettingTestDB(t)
	t.Setenv("XUI_LOG_FOLDER", t.TempDir())
	prev := ipLimitLocalAddrs
	ipLimitLocalAddrs = func() []netip.Addr { return nil }
	t.Cleanup(func() { ipLimitLocalAddrs = prev })
	resetIpLimitCaches()
}

func seedLimitedClient(t *testing.T, nodeID *int, tag string, port int, email string, limit int) *model.Inbound {
	t.Helper()
	return seedNodeInbound(t, nodeID, tag, port, model.VLESS, true, []model.Client{
		{Email: email, ID: uuidFor(email), LimitIP: limit, Enable: true},
	})
}

func uuidFor(seed string) string {
	h := fmt.Sprintf("%x", sha256.Sum256([]byte(seed)))
	return h[0:8] + "-" + h[8:12] + "-4" + h[13:16] + "-8" + h[17:20] + "-" + h[20:32]
}

func activeBanNetworks(t *testing.T, email string, now time.Time) []string {
	t.Helper()
	bans, err := (&IpLimitService{}).BansForEmail(email, now)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, 0, len(bans))
	for _, b := range bans {
		out = append(out, b.Network)
	}
	slices.Sort(out)
	return out
}

func TestIpLimitBansTheLeastRecentlySeenNetworkAcrossServers(t *testing.T) {
	setupIpLimitTest(t)
	node := seedAgentNodeRow(t, "edge")
	seedLimitedClient(t, &node.Id, "in-a", 443, "alice", 2)
	now := time.Unix(1_800_000_000, 0)

	observed := []IpObservation{
		{Email: "alice", IP: "198.51.100.1", LastSeen: now.Unix() - 300},
		{Email: "alice", IP: "198.51.100.2", LastSeen: now.Unix() - 20},
		{Email: "alice", IP: "198.51.100.3", LastSeen: now.Unix() - 5},
	}
	changed, err := (&IpLimitService{}).Enforce(now, observed)
	if err != nil || !changed {
		t.Fatalf("Enforce = %v, %v; want a new ban", changed, err)
	}
	if got := activeBanNetworks(t, "alice", now); !slices.Equal(got, []string{"198.51.100.1"}) {
		t.Fatalf("banned %v, want only the least recently seen network", got)
	}
	var ban model.ClientIpBan
	if err := database.GetDB().Where("email = ?", "alice").First(&ban).Error; err != nil {
		t.Fatal(err)
	}
	if ban.ExpiresAt != now.Unix()+30*60 {
		t.Fatalf("ban expires at %d, want the default 30 minutes after %d", ban.ExpiresAt, now.Unix())
	}

	// The banned network keeps retrying; it must not count, or each retry would
	// push another of the user's networks out.
	changed, err = (&IpLimitService{}).Enforce(now.Add(10*time.Second), observed)
	if err != nil || changed {
		t.Fatalf("second Enforce = %v, %v; want no further ban", changed, err)
	}
}

func TestIpLimitNeverCountsPiggerServers(t *testing.T) {
	setupIpLimitTest(t)
	relay := seedAgentNodeRow(t, "relay")
	natRelay := seedAgentNodeRow(t, "nat-relay")
	if err := database.GetDB().Model(natRelay).Updates(map[string]any{"address": "relay.example.test", "agent_remote_ip": "192.0.2.44"}).Error; err != nil {
		t.Fatal(err)
	}
	prevLookup := ipLimitLookupHost
	ipLimitLookupHost = func(host string) ([]netip.Addr, error) {
		if host == "relay.example.test" {
			return []netip.Addr{netip.MustParseAddr("192.0.2.33")}, nil
		}
		return nil, os.ErrNotExist
	}
	t.Cleanup(func() { ipLimitLookupHost = prevLookup })
	ipLimitLocalAddrs = func() []netip.Addr {
		return []netip.Addr{netip.MustParseAddr("192.0.2.55"), netip.MustParseAddr("2001:db8:aa::1")}
	}
	seedLimitedClient(t, &relay.Id, "in-r", 443, "bob", 1)
	now := time.Unix(1_800_000_000, 0)

	observed := []IpObservation{
		{Email: "bob", IP: "198.51.100.9", LastSeen: now.Unix() - 1},
		{Email: "bob", IP: relay.Address, LastSeen: now.Unix() - 50},
		{Email: "bob", IP: "192.0.2.33", LastSeen: now.Unix() - 60},
		{Email: "bob", IP: "192.0.2.44", LastSeen: now.Unix() - 70},
		{Email: "bob", IP: "192.0.2.55", LastSeen: now.Unix() - 80},
		{Email: "bob", IP: "2001:db8:aa::99", LastSeen: now.Unix() - 90},
	}
	changed, err := (&IpLimitService{}).Enforce(now, observed)
	if err != nil || changed {
		t.Fatalf("Enforce = %v, %v; server addresses must neither count nor be banned", changed, err)
	}
}

func TestIpLimitCountsRelayedUsersByTheirRealAddress(t *testing.T) {
	setupIpLimitTest(t)
	relayNode, exitNode := seedAgentNodeRow(t, "relay"), seedAgentNodeRow(t, "exit")
	if err := database.GetDB().Model(exitNode).Update("address", "203.0.113.80").Error; err != nil {
		t.Fatal(err)
	}
	user := model.Client{Email: "carol", ID: "33333333-3333-4333-8333-333333333333", LimitIP: 1, Enable: true}
	relay := seedNodeInbound(t, &relayNode.Id, "relay-in", 81, model.VLESS, true, []model.Client{user})
	target := seedNodeInbound(t, &exitNode.Id, "exit-in", 443, model.VLESS, true, []model.Client{user})
	chain := model.ProxyChain{TargetInboundId: target.Id, RelayInboundId: relay.Id, Enabled: true}
	if err := database.GetDB().Create(&chain).Error; err != nil {
		t.Fatal(err)
	}
	_, alias := model.ChainTransitCredential(user.ID, chain.Id)
	now := time.Unix(1_800_000_000, 0)

	observed := []IpObservation{
		{Email: alias, IP: "198.51.100.20", LastSeen: now.Unix() - 40},
		{Email: "carol", IP: relayNode.Address, LastSeen: now.Unix() - 40},
		{Email: "carol", IP: "198.51.100.21", LastSeen: now.Unix() - 2},
	}
	changed, err := (&IpLimitService{}).Enforce(now, observed)
	if err != nil || !changed {
		t.Fatalf("Enforce = %v, %v; the relayed device is a second network", changed, err)
	}
	if got := activeBanNetworks(t, "carol", now); !slices.Equal(got, []string{"198.51.100.20"}) {
		t.Fatalf("banned %v, want the relayed device's own address", got)
	}
}

func TestIpLimitCountsIPv6PerSlash64(t *testing.T) {
	setupIpLimitTest(t)
	node := seedAgentNodeRow(t, "edge")
	seedLimitedClient(t, &node.Id, "in-6", 443, "dave", 1)
	now := time.Unix(1_800_000_000, 0)

	sameLan := []IpObservation{
		{Email: "dave", IP: "2001:db8:1:2::10", LastSeen: now.Unix() - 30},
		{Email: "dave", IP: "2001:db8:1:2:a:b:c:d", LastSeen: now.Unix() - 3},
		{Email: "dave", IP: "::ffff:198.51.100.30", LastSeen: now.Unix() - 3000},
	}
	changed, err := (&IpLimitService{}).Enforce(now, sameLan[:2])
	if err != nil || changed {
		t.Fatalf("Enforce = %v, %v; two addresses in one /64 are one network", changed, err)
	}
	changed, err = (&IpLimitService{}).Enforce(now, sameLan)
	if err != nil || !changed {
		t.Fatalf("Enforce = %v, %v; an IPv4 network next to the /64 is a second one", changed, err)
	}
	if got := activeBanNetworks(t, "dave", now); !slices.Equal(got, []string{"198.51.100.30"}) {
		t.Fatalf("banned %v, want the mapped IPv4 address in plain form", got)
	}

	other := append(sameLan[:2:2], IpObservation{Email: "dave", IP: "2001:db8:9:9::1", LastSeen: now.Unix()})
	if _, err := (&IpLimitService{}).Enforce(now, other); err != nil {
		t.Fatal(err)
	}
	if got := activeBanNetworks(t, "dave", now); !slices.Equal(got, []string{"198.51.100.30", "2001:db8:1:2::/64"}) {
		t.Fatalf("banned %v, want the older /64 banned as a whole", got)
	}
}

func TestIpLimitIgnoresAddressesThatAreNotOnTheInternet(t *testing.T) {
	setupIpLimitTest(t)
	node := seedAgentNodeRow(t, "nat")
	seedLimitedClient(t, &node.Id, "in-n", 443, "erin", 1)
	now := time.Unix(1_800_000_000, 0)

	observed := []IpObservation{
		{Email: "erin", IP: "10.10.16.1", LastSeen: now.Unix() - 10},
		{Email: "erin", IP: "100.64.3.4", LastSeen: now.Unix() - 10},
		{Email: "erin", IP: "fd00::5", LastSeen: now.Unix() - 10},
		{Email: "erin", IP: "198.51.100.40", LastSeen: now.Unix() - 1},
	}
	changed, err := (&IpLimitService{}).Enforce(now, observed)
	if err != nil || changed {
		t.Fatalf("Enforce = %v, %v; a NAT gateway or private address says nothing about devices", changed, err)
	}
}

func TestIpLimitHonoursTheAllowlistAndBanDuration(t *testing.T) {
	setupIpLimitTest(t)
	node := seedAgentNodeRow(t, "edge")
	seedLimitedClient(t, &node.Id, "in-w", 443, "fay", 1)
	s := &SettingService{}
	if err := s.saveSetting("ipLimitAllowlist", "198.51.100.0/28"); err != nil {
		t.Fatal(err)
	}
	if err := s.saveSetting("ipLimitBanMinutes", "5"); err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0)

	observed := []IpObservation{
		{Email: "fay", IP: "198.51.100.3", LastSeen: now.Unix() - 900},
		{Email: "fay", IP: "198.51.100.50", LastSeen: now.Unix() - 600},
		{Email: "fay", IP: "198.51.100.51", LastSeen: now.Unix() - 1},
	}
	if _, err := (&IpLimitService{}).Enforce(now, observed); err != nil {
		t.Fatal(err)
	}
	var bans []model.ClientIpBan
	if err := database.GetDB().Find(&bans).Error; err != nil {
		t.Fatal(err)
	}
	if len(bans) != 1 || bans[0].Network != "198.51.100.50" || bans[0].ExpiresAt != now.Unix()+5*60 {
		t.Fatalf("bans = %+v; want 198.51.100.50 for 5 minutes and the allowlisted address untouched", bans)
	}
}

func TestIpLimitLiftsLapsedBansAndLogsThem(t *testing.T) {
	setupIpLimitTest(t)
	node := seedAgentNodeRow(t, "edge")
	seedLimitedClient(t, &node.Id, "in-x", 443, "gus", 2)
	seedLimitedClient(t, &node.Id, "in-y", 444, "hal", 0)
	now := time.Unix(1_800_000_000, 0)
	for _, b := range []model.ClientIpBan{
		{Email: "gus", Network: "198.51.100.60", BannedAt: now.Unix() - 1800, ExpiresAt: now.Unix() - 1},
		{Email: "gus", Network: "198.51.100.61", BannedAt: now.Unix() - 60, ExpiresAt: now.Unix() + 600},
		{Email: "hal", Network: "198.51.100.62", BannedAt: now.Unix() - 60, ExpiresAt: now.Unix() + 600},
	} {
		if err := database.GetDB().Create(&b).Error; err != nil {
			t.Fatal(err)
		}
	}

	changed, err := (&IpLimitService{}).Enforce(now, nil)
	if err != nil || !changed {
		t.Fatalf("Enforce = %v, %v; an expired ban and a ban without a limit must go", changed, err)
	}
	var left []model.ClientIpBan
	if err := database.GetDB().Order("network").Find(&left).Error; err != nil {
		t.Fatal(err)
	}
	if len(left) != 1 || left[0].Network != "198.51.100.61" {
		t.Fatalf("bans left = %+v; want only gus's unexpired ban", left)
	}
	logged, err := os.ReadFile(filepath.Join(os.Getenv("XUI_LOG_FOLDER"), "3xipl-banned.log"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"UNBAN   [Email] = gus [IP] = 198.51.100.60", "UNBAN   [Email] = hal [IP] = 198.51.100.62"} {
		if !strings.Contains(string(logged), want) {
			t.Fatalf("ban log lacks %q:\n%s", want, logged)
		}
	}
}

func TestIpLimitUnbanAndActiveBans(t *testing.T) {
	setupIpLimitTest(t)
	node := seedAgentNodeRow(t, "edge")
	seedLimitedClient(t, &node.Id, "in-u", 443, "ivy", 1)
	now := time.Unix(1_800_000_000, 0)
	for _, b := range []model.ClientIpBan{
		{Email: "ivy", Network: "198.51.100.70", BannedAt: now.Unix() - 60, ExpiresAt: now.Unix() + 600},
		{Email: "ivy", Network: "198.51.100.71", BannedAt: now.Unix() - 600, ExpiresAt: now.Unix() - 1},
	} {
		if err := database.GetDB().Create(&b).Error; err != nil {
			t.Fatal(err)
		}
	}
	if got := activeBanNetworks(t, "ivy", now); !slices.Equal(got, []string{"198.51.100.70"}) {
		t.Fatalf("active bans %v, want the expired one hidden", got)
	}
	if err := (&IpLimitService{}).Unban("ivy", "198.51.100.70"); err != nil {
		t.Fatal(err)
	}
	if got := activeBanNetworks(t, "ivy", now); len(got) != 0 {
		t.Fatalf("active bans %v after unban", got)
	}
	if err := (&IpLimitService{}).Unban("ivy", "198.51.100.70"); err == nil {
		t.Fatal("unbanning a network that is not banned succeeded")
	}
}

func routingRules(t *testing.T, raw []byte) []map[string]any {
	t.Helper()
	var routing struct {
		Rules []map[string]any `json:"rules"`
	}
	if err := json.Unmarshal(raw, &routing); err != nil {
		t.Fatal(err)
	}
	return routing.Rules
}

func outboundTags(t *testing.T, raw []byte) []string {
	t.Helper()
	var outbounds []struct {
		Tag      string `json:"tag"`
		Protocol string `json:"protocol"`
	}
	if err := json.Unmarshal(raw, &outbounds); err != nil {
		t.Fatal(err)
	}
	var tags []string
	for _, o := range outbounds {
		tags = append(tags, o.Tag+":"+o.Protocol)
	}
	return tags
}

func TestIpLimitBanRulesLeadTheConfigOfServersThatServeTheUser(t *testing.T) {
	setupIpLimitTest(t)
	relayNode, exitNode, otherNode := seedAgentNodeRow(t, "relay"), seedAgentNodeRow(t, "exit"), seedAgentNodeRow(t, "other")
	user := model.Client{Email: "kim", ID: uuidFor("kim"), LimitIP: 1, Enable: true}
	relay := seedNodeInbound(t, &relayNode.Id, "relay-in", 81, model.VLESS, true, []model.Client{user})
	target := seedNodeInbound(t, &exitNode.Id, "exit-in", 443, model.VLESS, true, []model.Client{user})
	seedLimitedClient(t, &otherNode.Id, "other-in", 443, "lee", 1)
	chain := model.ProxyChain{TargetInboundId: target.Id, RelayInboundId: relay.Id, Enabled: true}
	if err := database.GetDB().Create(&chain).Error; err != nil {
		t.Fatal(err)
	}
	_, alias := model.ChainTransitCredential(user.ID, chain.Id)
	now := time.Now()
	for _, network := range []string{"2001:db8:5:6::/64", "198.51.100.80"} {
		ban := model.ClientIpBan{Email: "kim", Network: network, BannedAt: now.Unix(), ExpiresAt: now.Unix() + 600}
		if err := database.GetDB().Create(&ban).Error; err != nil {
			t.Fatal(err)
		}
	}

	for name, nodeID := range map[string]int{"relay": relayNode.Id, "exit": exitNode.Id} {
		cfg, err := (&XrayService{}).GetAgentXrayConfig(nodeID)
		if err != nil {
			t.Fatal(err)
		}
		first := routingRules(t, cfg.RouterConfig)[0]
		wantUsers := []any{"kim"}
		if name == "relay" {
			wantUsers = []any{alias, "kim"}
			slices.SortFunc(wantUsers, func(a, b any) int { return strings.Compare(a.(string), b.(string)) })
		}
		gotUsers, _ := first["user"].([]any)
		gotNets, _ := first["sourceIP"].([]any)
		if first["outboundTag"] != "iplimit-block" || !slices.Equal(gotUsers, wantUsers) ||
			!slices.Equal(gotNets, []any{"198.51.100.80", "2001:db8:5:6::/64"}) {
			t.Fatalf("%s: first rule = %v; want kim's banned networks blocked for %v", name, first, wantUsers)
		}
		if !slices.Contains(outboundTags(t, cfg.OutboundConfigs), "iplimit-block:blackhole") {
			t.Fatalf("%s: no blackhole outbound for the ban rule: %s", name, cfg.OutboundConfigs)
		}
	}

	cfg, err := (&XrayService{}).GetAgentXrayConfig(otherNode.Id)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(cfg.RouterConfig)+string(cfg.OutboundConfigs), "iplimit-block") {
		t.Fatal("a server that does not serve the banned client got its ban rule")
	}

	// A client whose limit was removed loses its bans from every config at once.
	if err := database.GetDB().Model(&model.ClientRecord{}).Where("email = ?", "kim").Update("limit_ip", 0).Error; err != nil {
		t.Fatal(err)
	}
	cfg, err = (&XrayService{}).GetAgentXrayConfig(exitNode.Id)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(cfg.RouterConfig), "iplimit-block") {
		t.Fatal("the ban rule outlived the client's IP limit")
	}
}

// Agents and the panel apply a config through the core API when they can and
// restart otherwise; a restart would drop every user each time a ban starts or ends.
func TestIpLimitBanChangeIsHotApplicable(t *testing.T) {
	setupIpLimitTest(t)
	node := seedAgentNodeRow(t, "edge")
	seedLimitedClient(t, &node.Id, "in-h", 443, "max", 1)
	before, err := (&XrayService{}).GetAgentXrayConfig(node.Id)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	ban := model.ClientIpBan{Email: "max", Network: "198.51.100.90", BannedAt: now.Unix(), ExpiresAt: now.Unix() + 600}
	if err := database.GetDB().Create(&ban).Error; err != nil {
		t.Fatal(err)
	}
	after, err := (&XrayService{}).GetAgentXrayConfig(node.Id)
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct {
		name     string
		from, to *xray.Config
	}{{"ban starts", before, after}, {"ban ends", after, before}} {
		diff, ok := xray.ComputeHotDiff(step.from, step.to)
		if !ok {
			t.Fatalf("%s: the change needs a core restart", step.name)
		}
		if diff.RoutingConfig == nil || len(diff.AddedInbounds)+len(diff.RemovedInboundTags)+len(diff.AddedUsers)+len(diff.RemovedUsers) != 0 {
			t.Fatalf("%s: diff = %+v; want routing (and the block outbound) only", step.name, diff)
		}
	}
}

func TestOnlineIpsListOnlyTheClientsOwnNetworks(t *testing.T) {
	setupIpLimitTest(t)
	relayNode, exitNode := seedAgentNodeRow(t, "relay"), seedAgentNodeRow(t, "exit")
	if err := database.GetDB().Model(exitNode).Update("address", "203.0.113.80").Error; err != nil {
		t.Fatal(err)
	}
	user := model.Client{Email: "uma", ID: uuidFor("uma"), LimitIP: 3, Enable: true}
	relay := seedNodeInbound(t, &relayNode.Id, "relay-in", 81, model.VLESS, true, []model.Client{user})
	target := seedNodeInbound(t, &exitNode.Id, "exit-in", 443, model.VLESS, true, []model.Client{user})
	for id, remark := range map[int]string{relay.Id: "HK relay", target.Id: "SG exit"} {
		if err := database.GetDB().Model(&model.Inbound{}).Where("id = ?", id).Update("remark", remark).Error; err != nil {
			t.Fatal(err)
		}
	}
	chain := model.ProxyChain{TargetInboundId: target.Id, RelayInboundId: relay.Id, Enabled: true}
	if err := database.GetDB().Create(&chain).Error; err != nil {
		t.Fatal(err)
	}
	_, alias := model.ChainTransitCredential(user.ID, chain.Id)
	if err := (&SettingService{}).saveSetting("ipLimitAllowlist", "198.51.100.200"); err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0)
	ban := model.ClientIpBan{Email: "uma", Network: "198.51.100.9", BannedAt: now.Unix() - 60, ExpiresAt: now.Unix() + 600}
	if err := database.GetDB().Create(&ban).Error; err != nil {
		t.Fatal(err)
	}

	observed := []IpObservation{
		{Email: alias, IP: "198.51.100.20", LastSeen: now.Unix() - 30, Server: relayNode.Id},
		{Email: "uma", IP: relayNode.Address, LastSeen: now.Unix() - 30, Server: exitNode.Id},
		{Email: "uma", IP: "10.10.16.1", LastSeen: now.Unix() - 20, Server: exitNode.Id},
		{Email: "uma", IP: "2001:db8:1:2::10", LastSeen: now.Unix() - 15, Server: exitNode.Id},
		{Email: "uma", IP: "2001:db8:1:2::20", LastSeen: now.Unix() - 12, Server: exitNode.Id},
		{Email: "uma", IP: "198.51.100.200", LastSeen: now.Unix() - 5, Server: exitNode.Id},
		{Email: "uma", IP: "198.51.100.9", LastSeen: now.Unix() - 1, Server: exitNode.Id},
	}
	if _, err := (&IpLimitService{}).Enforce(now, observed); err != nil {
		t.Fatal(err)
	}

	got, err := (&IpLimitService{}).OnlineIps("uma", now)
	if err != nil {
		t.Fatal(err)
	}
	want := []OnlineNetwork{
		{Network: "198.51.100.200", Addresses: []string{"198.51.100.200"}, Servers: []string{"SG exit"}, LastSeen: now.Unix() - 5, Counted: false},
		{Network: "2001:db8:1:2::/64", Addresses: []string{"2001:db8:1:2::10", "2001:db8:1:2::20"}, Servers: []string{"SG exit"}, LastSeen: now.Unix() - 12, Counted: true},
		{Network: "198.51.100.20", Addresses: []string{"198.51.100.20"}, Servers: []string{"HK relay"}, LastSeen: now.Unix() - 30, Counted: true},
	}
	if got.Limit != 3 || got.Count != 2 || len(got.Online) != len(want) {
		t.Fatalf("online ips = %+v; want limit 3, count 2 and %d networks", got, len(want))
	}
	for i := range want {
		g, w := got.Online[i], want[i]
		if g.Network != w.Network || !slices.Equal(g.Addresses, w.Addresses) || !slices.Equal(g.Servers, w.Servers) ||
			g.LastSeen != w.LastSeen || g.Counted != w.Counted {
			t.Fatalf("online[%d] = %+v, want %+v", i, g, w)
		}
	}
	if len(got.Bans) != 1 || got.Bans[0].Network != "198.51.100.9" || got.Bans[0].ExpiresAt != now.Unix()+600 {
		t.Fatalf("bans = %+v; want the running ban only", got.Bans)
	}
}

// The panels read the last scan; one older than a few scans means the scan
// stopped, and showing it would claim devices that may be long gone.
func TestOnlineIpsIgnoreAStaleScan(t *testing.T) {
	setupIpLimitTest(t)
	node := seedAgentNodeRow(t, "edge")
	seedLimitedClient(t, &node.Id, "in-s", 443, "val", 3)
	now := time.Unix(1_800_000_000, 0)
	observed := []IpObservation{{Email: "val", IP: "198.51.100.30", LastSeen: now.Unix() - 1, Server: node.Id}}
	if _, err := (&IpLimitService{}).Enforce(now, observed); err != nil {
		t.Fatal(err)
	}
	if fresh, err := (&IpLimitService{}).OnlineIps("val", now.Add(20*time.Second)); err != nil || fresh.Count != 1 {
		t.Fatalf("a 20 s old scan = %+v, %v; want the network counted", fresh, err)
	}
	stale, err := (&IpLimitService{}).OnlineIps("val", now.Add(2*time.Minute))
	if err != nil || stale.Count != 0 || len(stale.Online) != 0 {
		t.Fatalf("a 2 min old scan = %+v, %v; want nothing online", stale, err)
	}
}

// The user list shows "slots in use / limit" per row without opening anything,
// so the counts ride on the rows the list already polls.
func TestClientListRowsCarryOnlineIpCounts(t *testing.T) {
	setupIpLimitTest(t)
	node := seedAgentNodeRow(t, "edge")
	seedLimitedClient(t, &node.Id, "in-w1", 443, "wes", 3)
	seedLimitedClient(t, &node.Id, "in-w2", 444, "xia", 3)
	now := time.Now()
	ban := model.ClientIpBan{Email: "wes", Network: "198.51.100.43", BannedAt: now.Unix(), ExpiresAt: now.Unix() + 600}
	if err := database.GetDB().Create(&ban).Error; err != nil {
		t.Fatal(err)
	}
	observed := []IpObservation{
		{Email: "wes", IP: "198.51.100.41", LastSeen: now.Unix() - 9, Server: node.Id},
		{Email: "wes", IP: "198.51.100.42", LastSeen: now.Unix() - 3, Server: node.Id},
		{Email: "wes", IP: "198.51.100.43", LastSeen: now.Unix() - 1, Server: node.Id},
	}
	if _, err := (&IpLimitService{}).Enforce(now, observed); err != nil {
		t.Fatal(err)
	}

	page, err := (&ClientService{}).ListPaged(&InboundService{}, &SettingService{}, ClientPageParams{Page: 1, PageSize: 25})
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string]ClientSlim{}
	for _, item := range page.Items {
		rows[item.Email] = item
	}
	if r := rows["wes"]; r.OnlineIps != 2 || r.IpBans != 1 {
		t.Fatalf("wes row = online %d, bans %d; want 2 slots in use and 1 ban", r.OnlineIps, r.IpBans)
	}
	if r := rows["xia"]; r.OnlineIps != 0 || r.IpBans != 0 {
		t.Fatalf("xia row = online %d, bans %d; want nothing", r.OnlineIps, r.IpBans)
	}
}
