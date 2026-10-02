package service

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service/probetest"
)

// The clock of setupProbe stands at 2026-10-02T08:00:00Z = 1790928000000 ms.
const (
	scopeLiteNodes = `{
  "uuid-m":   {"name":"lite master","region":"🇺🇸","os":"Debian","traffic_limit":1000,"traffic_limit_type":"sum"},
  "uuid-1":   {"name":"lite one","region":"🇭🇰"},
  "uuid-2":   {"name":"lite two","region":"🇸🇬"},
  "uuid-off": {"name":"lite off","region":"🇬🇧"},
  "uuid-new": {"name":"lite new","region":"🇯🇵"}
}`
	scopeLiteStatuses = `{
  "uuid-m":   {"time":"2026-10-02T07:59:58Z","online":true,"cpu":11,"ram":100,"ram_total":200,"disk":300,"disk_total":400,
    "load":0.1,"load5":0.2,"load15":0.3,"net_in":10,"net_out":20,"net_total_up":30,"net_total_down":40,"uptime":50,
    "ping":{"3":{"name":"cn","latest":20,"avg":21,"loss":1.5}}},
  "uuid-1":   {"time":"2026-10-02T07:59:57Z","online":true,"cpu":22,"ping":{}},
  "uuid-2":   {"time":"2026-10-02T07:59:56Z","online":true,"cpu":33,"ping":{}},
  "uuid-off": {"time":"2026-10-02T07:40:00Z","online":false,"cpu":44,"ram":5,
    "ping":{"3":{"name":"cn","latest":20,"avg":21,"loss":0}}}
}`
	scopeFetchedAt = int64(1790928000000)
)

// seedProbeInbound puts an enabled VLESS inbound on a host; nodeId 0 is the master.
func seedProbeInbound(t *testing.T, nodeId int, tag, remark string, sortIndex int) *model.Inbound {
	t.Helper()
	db := database.GetDB()
	var count int64
	db.Model(&model.Inbound{}).Count(&count)
	inbound := &model.Inbound{
		UserId: 1, Tag: tag, Remark: remark, Enable: true, Port: 41000 + int(count), Protocol: model.VLESS,
		Settings: `{"clients":[]}`, SubSortIndex: sortIndex,
	}
	if nodeId != 0 {
		inbound.NodeID = &nodeId
	}
	if err := db.Create(inbound).Error; err != nil {
		t.Fatalf("seed inbound %s: %v", tag, err)
	}
	return inbound
}

func seedProbeClient(t *testing.T, email string, inbounds ...*model.Inbound) *model.ClientRecord {
	t.Helper()
	db := database.GetDB()
	client := &model.ClientRecord{Email: email, SubID: "sub-" + email, Enable: true}
	if err := db.Create(client).Error; err != nil {
		t.Fatalf("seed client %s: %v", email, err)
	}
	for _, inbound := range inbounds {
		if err := db.Create(&model.ClientInbound{ClientId: client.Id, InboundId: inbound.Id}).Error; err != nil {
			t.Fatalf("attach %s to %s: %v", email, inbound.Tag, err)
		}
	}
	return client
}

func setInboundColumn(t *testing.T, inbound *model.Inbound, column string, value any) {
	t.Helper()
	if err := database.GetDB().Model(&model.Inbound{}).Where("id = ?", inbound.Id).Update(column, value).Error; err != nil {
		t.Fatalf("set %s on %s: %v", column, inbound.Tag, err)
	}
}

func clientServers(t *testing.T, s *ProbeService, client *model.ClientRecord) PortalProbe {
	t.Helper()
	probe, err := s.ClientServers(context.Background(), client)
	if err != nil {
		t.Fatalf("client servers of %s: %v", client.Email, err)
	}
	return probe
}

func assertPortalProbe(t *testing.T, got, want PortalProbe) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("portal probe =\n %+v\nwant\n %+v", got, want)
	}
}

func scopeLite(t *testing.T, s *ProbeService) *probetest.Lite {
	t.Helper()
	lite := probetest.NewLite(t)
	lite.Answer(scopeLiteNodes, scopeLiteStatuses)
	useLite(t, s, lite)
	return lite
}

// The whole point of the portal view: a client is shown the hosts behind its
// own inbounds, the master's under node id 0, and nothing of anyone else's.
func TestClientServersListsOnlyTheHostsTheClientOwns(t *testing.T) {
	s, _ := setupProbe(t)
	scopeLite(t, s)
	hk := seedProbeNode(t, "edge-hk", "", "203.0.113.11")
	sg := seedProbeNode(t, "edge-sg", "", "203.0.113.12")
	onMaster := seedProbeInbound(t, 0, "in-443", "洛杉矶-Alpha", 1)
	onHK := seedProbeInbound(t, hk, "n1-in-443", "香港-Bravo", 2)
	onSG := seedProbeInbound(t, sg, "n2-in-443", "新加坡-Charlie", 3)
	alice := seedProbeClient(t, "alice", onMaster, onHK)
	bob := seedProbeClient(t, "bob", onSG)
	setProbeLinks(t, s,
		ProbeLinkInput{NodeId: 0, ServerId: "uuid-m"},
		ProbeLinkInput{NodeId: hk, ServerId: "uuid-1"},
		ProbeLinkInput{NodeId: sg, ServerId: "uuid-2"},
	)

	assertPortalProbe(t, clientServers(t, s, alice), PortalProbe{
		Enabled: true, FetchedAt: scopeFetchedAt,
		Servers: []PortalProbeServer{
			{
				Id: 0, Name: "洛杉矶-Alpha", Status: "online", Region: "🇺🇸", UpdatedAt: 1790927998000,
				Cpu: 11, MemUsed: 100, MemTotal: 200, DiskUsed: 300, DiskTotal: 400,
				Load1: 0.1, Load5: 0.2, Load15: 0.3, NetIn: 10, NetOut: 20, NetTotalUp: 30, NetTotalDown: 40, Uptime: 50,
				Pings: []ProbePing{{Id: 3, Name: "cn", Latency: 21, Loss: 1.5, Blocks: []ProbePingBlock{}}},
			},
			{Id: hk, Name: "香港-Bravo", Status: "online", Region: "🇭🇰", UpdatedAt: 1790927997000, Cpu: 22, Pings: []ProbePing{}},
		},
	})
	assertPortalProbe(t, clientServers(t, s, bob), PortalProbe{
		Enabled: true, FetchedAt: scopeFetchedAt,
		Servers: []PortalProbeServer{
			{Id: sg, Name: "新加坡-Charlie", Status: "online", Region: "🇸🇬", UpdatedAt: 1790927996000, Cpu: 33, Pings: []ProbePing{}},
		},
	})
}

// One case per status a client can be shown. Only an online server carries
// figures; a host without a usable link says so instead of looking offline.
func TestClientServersReportsOneStatusPerHost(t *testing.T) {
	tests := []struct {
		name     string
		serverId string
		want     PortalProbeServer
	}{
		{"online", "uuid-1", PortalProbeServer{Status: "online", Region: "🇭🇰", UpdatedAt: 1790927997000, Cpu: 22}},
		{"offline keeps when it was last seen", "uuid-off", PortalProbeServer{Status: "offline", Region: "🇬🇧", UpdatedAt: 1790926800000}},
		{"unknown has not reported since Lite started", "uuid-new", PortalProbeServer{Status: "unknown", Region: "🇯🇵"}},
		{"unmonitored without a link", "", PortalProbeServer{Status: "unmonitored"}},
		{"unmonitored when Lite no longer lists the server", "uuid-removed", PortalProbeServer{Status: "unmonitored"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := setupProbe(t)
			scopeLite(t, s)
			node := seedProbeNode(t, "edge-hk", "", "203.0.113.11")
			client := seedProbeClient(t, "alice", seedProbeInbound(t, node, "n1-in-443", "香港-Bravo", 1))
			setProbeLinks(t, s, ProbeLinkInput{NodeId: node, ServerId: tc.serverId})

			tc.want.Id, tc.want.Name, tc.want.Pings = node, "香港-Bravo", []ProbePing{}
			assertPortalProbe(t, clientServers(t, s, client), PortalProbe{
				Enabled: true, FetchedAt: scopeFetchedAt, Servers: []PortalProbeServer{tc.want},
			})
		})
	}
}

// The master's own inbounds have no node row, so a missing link for node id 0
// must read as "not monitored" while the client's linked node still shows.
func TestClientServersMarksTheUnlinkedMasterUnmonitoredNextToALinkedNode(t *testing.T) {
	s, _ := setupProbe(t)
	scopeLite(t, s)
	node := seedProbeNode(t, "edge-hk", "", "203.0.113.11")
	client := seedProbeClient(t, "alice",
		seedProbeInbound(t, 0, "in-443", "洛杉矶-Alpha", 1), seedProbeInbound(t, node, "n1-in-443", "香港-Bravo", 2))
	setProbeLinks(t, s, ProbeLinkInput{NodeId: node, ServerId: "uuid-1"})

	assertPortalProbe(t, clientServers(t, s, client), PortalProbe{
		Enabled: true, FetchedAt: scopeFetchedAt,
		Servers: []PortalProbeServer{
			{Id: 0, Name: "洛杉矶-Alpha", Status: "unmonitored", Pings: []ProbePing{}},
			{Id: node, Name: "香港-Bravo", Status: "online", Region: "🇭🇰", UpdatedAt: 1790927997000, Cpu: 22, Pings: []ProbePing{}},
		},
	})
}

// Links are read per request: a relink must not wait for the cached Lite
// answer to expire, or the client would keep seeing the old server's figures.
func TestClientServersSeesARelinkWhileTheLiteAnswerIsCached(t *testing.T) {
	s, _ := setupProbe(t)
	lite := scopeLite(t, s)
	node := seedProbeNode(t, "edge-hk", "", "203.0.113.11")
	client := seedProbeClient(t, "alice", seedProbeInbound(t, node, "n1-in-443", "香港-Bravo", 1))
	setProbeLinks(t, s, ProbeLinkInput{NodeId: node, ServerId: "uuid-1"})
	if got := clientServers(t, s, client).Servers; len(got) != 1 || got[0].Cpu != 22 {
		t.Fatalf("before relinking = %+v, want the figures of uuid-1", got)
	}

	setProbeLinks(t, s, ProbeLinkInput{NodeId: node, ServerId: "uuid-2"})
	got := clientServers(t, s, client).Servers
	if len(got) != 1 || got[0].Cpu != 33 || got[0].Region != "🇸🇬" || lite.Requests() != 1 {
		t.Fatalf("after relinking = %+v with %d Lite requests, want the figures of uuid-2 from the cached answer", got, lite.Requests())
	}
}

// A host is one card however many inbounds the client has on it, named and
// ordered the way the subscription lists them.
func TestClientServersGroupsInboundsByHostInSubscriptionOrder(t *testing.T) {
	s, _ := setupProbe(t)
	scopeLite(t, s)
	node := seedProbeNode(t, "edge-hk", "", "203.0.113.11")
	client := seedProbeClient(t, "alice",
		seedProbeInbound(t, 0, "in-443", "洛杉矶-Alpha", 3),
		seedProbeInbound(t, node, "n1-in-443", "香港-B", 5),
		seedProbeInbound(t, node, "n1-in-8443", "香港-A", 2),
		seedProbeInbound(t, node, "n1-in-9443", "", 4),
	)

	got := clientServers(t, s, client).Servers
	type host struct {
		Id   int
		Name string
	}
	var hosts []host
	for _, server := range got {
		hosts = append(hosts, host{server.Id, server.Name})
	}
	want := []host{{node, "香港-A / 香港-B"}, {0, "洛杉矶-Alpha"}}
	if !reflect.DeepEqual(hosts, want) {
		t.Fatalf("hosts = %+v, want %+v", hosts, want)
	}
}

// Fail closed on the subscription's own filter: an inbound the subscription
// does not list must not make its host visible.
func TestClientServersSkipsHostsWhoseInboundsTheSubscriptionSkips(t *testing.T) {
	s, _ := setupProbe(t)
	scopeLite(t, s)
	disabledNode := seedProbeNode(t, "disabled", "", "203.0.113.1")
	excludedNode := seedProbeNode(t, "excluded", "", "203.0.113.2")
	proxyNode := seedProbeNode(t, "proxy", "", "203.0.113.3")
	kept := seedProbeInbound(t, 0, "in-443", "洛杉矶-Alpha", 1)
	disabled := seedProbeInbound(t, disabledNode, "n1-in-443", "disabled inbound", 1)
	excluded := seedProbeInbound(t, excludedNode, "n2-in-443", "hidden from the subscription", 1)
	proxy := seedProbeInbound(t, proxyNode, "n3-in-1080", "no share link", 1)
	setInboundColumn(t, disabled, "enable", false)
	setInboundColumn(t, excluded, "exclude_from_sub", true)
	setInboundColumn(t, proxy, "protocol", string(model.Mixed))
	client := seedProbeClient(t, "alice", kept, disabled, excluded, proxy)
	setProbeLinks(t, s,
		ProbeLinkInput{NodeId: disabledNode, ServerId: "uuid-1"},
		ProbeLinkInput{NodeId: excludedNode, ServerId: "uuid-2"},
		ProbeLinkInput{NodeId: proxyNode, ServerId: "uuid-m"},
	)

	assertPortalProbe(t, clientServers(t, s, client), PortalProbe{
		Enabled: true, FetchedAt: scopeFetchedAt,
		Servers: []PortalProbeServer{{Id: 0, Name: "洛杉矶-Alpha", Status: "unmonitored", Pings: []ProbePing{}}},
	})
	if linked, err := s.ClientHasLinkedHost(client); err != nil || linked {
		t.Fatalf("has a linked host = %v, %v; want false: only skipped inbounds sit on linked hosts", linked, err)
	}
}

// A disabled client no longer has the servers, so it is shown none, and Lite
// is not asked on its behalf.
func TestClientServersShowsADisabledClientNothing(t *testing.T) {
	s, _ := setupProbe(t)
	lite := scopeLite(t, s)
	client := seedProbeClient(t, "alice", seedProbeInbound(t, 0, "in-443", "洛杉矶-Alpha", 1))
	setProbeLinks(t, s, ProbeLinkInput{NodeId: 0, ServerId: "uuid-m"})
	client.Enable = false

	assertPortalProbe(t, clientServers(t, s, client), PortalProbe{Enabled: true, Servers: []PortalProbeServer{}})
	if n := lite.Requests(); n != 0 {
		t.Fatalf("Lite was asked %d times for a disabled client, want 0", n)
	}
	if linked, err := s.ClientHasLinkedHost(client); err != nil || linked {
		t.Fatalf("has a linked host = %v, %v; want false for a disabled client", linked, err)
	}
}

func TestClientServersIsOffUntilALiteAddressIsSet(t *testing.T) {
	s, _ := setupProbe(t)
	client := seedProbeClient(t, "alice", seedProbeInbound(t, 0, "in-443", "洛杉矶-Alpha", 1))
	setProbeLinks(t, s, ProbeLinkInput{NodeId: 0, ServerId: "uuid-m"})

	assertPortalProbe(t, clientServers(t, s, client), PortalProbe{Servers: []PortalProbeServer{}})
	if linked, err := s.ClientHasLinkedHost(client); err != nil || linked {
		t.Fatalf("has a linked host = %v, %v; want false while the probe is not configured", linked, err)
	}
}

// With Lite down and nothing cached the list of the client's own hosts is
// still shown (it is no secret), without figures and flagged as stale.
func TestClientServersNamesTheHostsWhenLiteIsDownWithNothingCached(t *testing.T) {
	s, _ := setupProbe(t)
	lite := scopeLite(t, s)
	lite.Override(brokenLite)
	node := seedProbeNode(t, "edge-hk", "", "203.0.113.11")
	client := seedProbeClient(t, "alice",
		seedProbeInbound(t, 0, "in-443", "洛杉矶-Alpha", 1), seedProbeInbound(t, node, "n1-in-443", "香港-Bravo", 2))
	setProbeLinks(t, s, ProbeLinkInput{NodeId: node, ServerId: "uuid-1"})

	assertPortalProbe(t, clientServers(t, s, client), PortalProbe{
		Enabled: true, Stale: true,
		Servers: []PortalProbeServer{
			{Id: 0, Name: "洛杉矶-Alpha", Status: "unmonitored", Pings: []ProbePing{}},
			{Id: node, Name: "香港-Bravo", Status: "unknown", Pings: []ProbePing{}},
		},
	})
}

// During a short outage the client keeps the last figures, marked stale. A host
// linked to a server that answer does not list stays unmonitored, not unknown.
func TestClientServersKeepsTheLastFiguresDuringAShortOutage(t *testing.T) {
	s, clock := setupProbe(t)
	lite := scopeLite(t, s)
	node := seedProbeNode(t, "edge-hk", "", "203.0.113.11")
	client := seedProbeClient(t, "alice",
		seedProbeInbound(t, 0, "in-443", "洛杉矶-Alpha", 1), seedProbeInbound(t, node, "n1-in-443", "香港-Bravo", 2))
	setProbeLinks(t, s, ProbeLinkInput{NodeId: 0, ServerId: "uuid-removed"}, ProbeLinkInput{NodeId: node, ServerId: "uuid-1"})
	clientServers(t, s, client)

	lite.Override(brokenLite)
	clock.Advance(5 * time.Second)
	assertPortalProbe(t, clientServers(t, s, client), PortalProbe{
		Enabled: true, FetchedAt: scopeFetchedAt, Stale: true,
		Servers: []PortalProbeServer{
			{Id: 0, Name: "洛杉矶-Alpha", Status: "unmonitored", Pings: []ProbePing{}},
			{Id: node, Name: "香港-Bravo", Status: "online", Region: "🇭🇰", UpdatedAt: 1790927997000, Cpu: 22, Pings: []ProbePing{}},
		},
	})
}

// portal/data shows the Probe switch only to a client who would see more than
// "not monitored"; it is asked on every portal load, so it must not call Lite.
func TestClientHasLinkedHostNeedsALinkOnOneOfTheClientsHosts(t *testing.T) {
	s, _ := setupProbe(t)
	lite := scopeLite(t, s)
	node := seedProbeNode(t, "edge-hk", "", "203.0.113.11")
	other := seedProbeNode(t, "edge-sg", "", "203.0.113.12")
	onMaster := seedProbeClient(t, "master-only", seedProbeInbound(t, 0, "in-443", "洛杉矶-Alpha", 1))
	onNode := seedProbeClient(t, "node-only", seedProbeInbound(t, node, "n1-in-443", "香港-Bravo", 1))

	check := func(when string, client *model.ClientRecord, want bool) {
		t.Helper()
		got, err := s.ClientHasLinkedHost(client)
		if err != nil || got != want {
			t.Fatalf("%s: %s has a linked host = %v, %v; want %v", when, client.Email, got, err, want)
		}
	}
	check("no links at all", onMaster, false)

	setProbeLinks(t, s, ProbeLinkInput{NodeId: other, ServerId: "uuid-2"})
	check("only another node is linked", onNode, false)

	setProbeLinks(t, s, ProbeLinkInput{NodeId: 0, ServerId: "uuid-m"})
	check("the master is linked", onMaster, true)
	check("the master is linked", onNode, false)

	setProbeLinks(t, s, ProbeLinkInput{NodeId: node, ServerId: "uuid-1"})
	check("the node is linked", onNode, true)
	check("the node is linked", onMaster, false)

	if n := lite.Requests(); n != 0 {
		t.Fatalf("the check asked Lite %d times, want 0", n)
	}
}
