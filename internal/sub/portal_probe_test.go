package sub

import (
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service/probetest"
)

const (
	portalProbeNodes = `{
  "uuid-a": {"name":"lite a","region":"🇺🇸","os":"Debian GNU/Linux 13","arch":"amd64","virtualization":"kvm",
    "cpu_cores":2,"price":188,"currency":"¥","expired_at":"2026-10-27T16:00:00Z","group":"secret-group",
    "tags":"secret-tag","public_remark":"secret remark","traffic_limit":1000,"traffic_limit_type":"sum"},
  "uuid-b": {"name":"lite b","region":"🇭🇰"},
  "uuid-c": {"name":"lite c","region":"🇸🇬"}
}`
	portalProbeStatuses = `{
  "uuid-a": {"time":"2026-10-02T07:59:58Z","online":true,"cpu":11,"ram":100,"ram_total":200,"uptime":50,
    "ping":{"3":{"name":"cn","latest":20,"avg":21,"loss":1.5}}},
  "uuid-b": {"time":"2026-10-02T07:59:57Z","online":true,"cpu":22,"ping":{}},
  "uuid-c": {"time":"2026-10-02T07:59:56Z","online":true,"cpu":33,"ping":{}}
}`
)

// seedPortalProbe puts pa@e's inbound on the master and pb@e's on a node, lets
// both sign in, and points the probe at a fake Lite with three servers.
func seedPortalProbe(t *testing.T) (router *gin.Engine, nodeId int) {
	t.Helper()
	router, _ = seedPortal(t)
	db := database.GetDB()
	node := &model.Node{Name: "edge-hk", Address: "203.0.113.11", Kind: model.NodeKindAgent, Enable: true}
	if err := db.Create(node).Error; err != nil {
		t.Fatalf("seed node: %v", err)
	}
	if err := db.Model(&model.Inbound{}).Where("tag = ?", "pb").Update("node_id", node.Id).Error; err != nil {
		t.Fatalf("move pb's inbound to the node: %v", err)
	}
	if err := (&service.ClientPortalService{}).SetPassword("pb@e", "bravo-pass"); err != nil {
		t.Fatalf("set pb's portal password: %v", err)
	}
	lite := probetest.NewLite(t)
	lite.Answer(portalProbeNodes, portalProbeStatuses)
	if _, err := (&service.ProbeService{}).SaveSettings(service.ProbeSettings{URL: lite.URL}); err != nil {
		t.Fatalf("point the probe at the fake Lite: %v", err)
	}
	return router, node.Id
}

func linkProbe(t *testing.T, links ...service.ProbeLinkInput) {
	t.Helper()
	if err := (&service.ProbeService{}).SetLinks(service.ProbeLinksInput{Links: links}); err != nil {
		t.Fatalf("set probe links: %v", err)
	}
}

func sortedKeys(object map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

// The probe names a person's servers, so it sits behind the same session as
// the rest of the portal, and an answer must never be cached by a proxy.
func TestPortalProbeNeedsTheSessionOfThatClient(t *testing.T) {
	router, _ := seedPortalProbe(t)
	linkProbe(t, service.ProbeLinkInput{NodeId: 0, ServerId: "uuid-a"})
	cookie := sessionCookie(t, portalLogin(router, "pa@e", "alpha-pass", "198.51.100.1"))
	forged := *cookie
	paId, pbId := strconv.Itoa(clientIdOf(t, "pa@e")), strconv.Itoa(clientIdOf(t, "pb@e"))
	forged.Value = strings.Replace(cookie.Value, "."+paId+".", "."+pbId+".", 1)
	if forged.Value == cookie.Value {
		t.Fatalf("cookie %q does not carry the client id %s where expected", cookie.Value, paId)
	}

	for name, sent := range map[string]*http.Cookie{"no cookie": nil, "a cookie forged for another client": &forged} {
		t.Run(name, func(t *testing.T) {
			res := portalRequest(router, http.MethodGet, "/sub/portal/probe", "", "198.51.100.1", sent)
			if res.Code != http.StatusUnauthorized || strings.TrimSpace(res.Body.String()) != `{"error":"unauthorized"}` {
				t.Fatalf("answer = %d %s, want 401 {\"error\":\"unauthorized\"}", res.Code, res.Body)
			}
		})
	}

	res := portalRequest(router, http.MethodGet, "/sub/portal/probe", "", "198.51.100.1", cookie)
	if res.Code != http.StatusOK || res.Header().Get("Cache-Control") != "no-cache, no-store, must-revalidate" {
		t.Fatalf("own session = %d with Cache-Control %q, want 200 and no caching", res.Code, res.Header().Get("Cache-Control"))
	}
}

// The privacy whitelist: a client gets the hosts behind its own inbounds with
// exactly these fields, without Lite IDs, addresses, billing or system details.
func TestPortalProbeSendsOnlyTheClientsHostsAndWhitelistedFields(t *testing.T) {
	router, node := seedPortalProbe(t)
	linkProbe(t, service.ProbeLinkInput{NodeId: 0, ServerId: "uuid-a"}, service.ProbeLinkInput{NodeId: node, ServerId: "uuid-b"})
	cookie := sessionCookie(t, portalLogin(router, "pa@e", "alpha-pass", "198.51.100.1"))

	res := portalRequest(router, http.MethodGet, "/sub/portal/probe", "", "198.51.100.1", cookie)
	if res.Code != http.StatusOK {
		t.Fatalf("probe = %d %s, want 200", res.Code, res.Body)
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", res.Body, err)
	}
	if got, want := sortedKeys(body), []string{"enabled", "fetchedAt", "servers", "stale"}; !slices.Equal(got, want) {
		t.Fatalf("answer keys = %v, want %v", got, want)
	}
	var servers []map[string]json.RawMessage
	if err := json.Unmarshal(body["servers"], &servers); err != nil || len(servers) != 1 {
		t.Fatalf("servers = %s (%v), want exactly pa@e's one host", body["servers"], err)
	}
	wantKeys := []string{
		"cpu", "diskTotal", "diskUsed", "id", "load1", "load15", "load5", "memTotal", "memUsed", "name",
		"netIn", "netOut", "netTotalDown", "netTotalUp", "pings", "provider", "region", "status", "updatedAt", "uptime",
	}
	if got := sortedKeys(servers[0]); !slices.Equal(got, wantKeys) {
		t.Fatalf("server keys = %v\nwant       %v", got, wantKeys)
	}
	const wantServer = `{"id":1,"name":"pa","provider":"lite a","status":"online","region":"🇺🇸","updatedAt":1790927998000,"cpu":11,` +
		`"memUsed":100,"memTotal":200,"diskUsed":0,"diskTotal":0,"load1":0,"load5":0,"load15":0,"netIn":0,"netOut":0,` +
		`"netTotalUp":0,"netTotalDown":0,"uptime":50,"pings":[{"id":3,"name":"cn","latency":21,"loss":1.5,"blocks":[]}]}`
	if got := string(body["servers"]); got != "["+wantServer+"]" {
		t.Fatalf("servers =\n %s\nwant\n [%s]", got, wantServer)
	}
	for _, secret := range []string{"uuid-", "lite b", "Debian", "secret", "¥", "2026-10-27", "203.0.113.11", `"pb"`} {
		if strings.Contains(res.Body.String(), secret) {
			t.Fatalf("the answer contains %q:\n%s", secret, res.Body)
		}
	}
}

// The portal parses the answer with a schema that rejects null arrays.
func TestPortalProbeAnswersWithArraysEvenWhenEmpty(t *testing.T) {
	router, _ := seedPortalProbe(t)
	cookie := sessionCookie(t, portalLogin(router, "pa@e", "alpha-pass", "198.51.100.1"))

	unlinked := portalRequest(router, http.MethodGet, "/sub/portal/probe", "", "198.51.100.1", cookie)
	const wantUnlinked = `"servers":[{"id":1,"name":"pa","status":"unmonitored","region":"","updatedAt":0,"cpu":0,` +
		`"memUsed":0,"memTotal":0,"diskUsed":0,"diskTotal":0,"load1":0,"load5":0,"load15":0,"netIn":0,"netOut":0,` +
		`"netTotalUp":0,"netTotalDown":0,"uptime":0,"pings":[]}]`
	if unlinked.Code != http.StatusOK || !strings.Contains(unlinked.Body.String(), wantUnlinked) {
		t.Fatalf("an unlinked host = %d %s\nwant it to contain %s", unlinked.Code, unlinked.Body, wantUnlinked)
	}

	if _, err := (&service.ProbeService{}).SaveSettings(service.ProbeSettings{}); err != nil {
		t.Fatalf("turn the probe off: %v", err)
	}
	off := portalRequest(router, http.MethodGet, "/sub/portal/probe", "", "198.51.100.1", cookie)
	if got := strings.TrimSpace(off.Body.String()); off.Code != http.StatusOK || got != `{"enabled":false,"fetchedAt":0,"stale":false,"servers":[]}` {
		t.Fatalf("with the probe off = %d %s, want enabled false and an empty array", off.Code, got)
	}
}

// The portal shows its Probe switch from this flag, so it must be true only
// for a client who has a host with a link, and must be there for the others.
func TestPortalDataOffersTheProbeOnlyToAClientWithALinkedHost(t *testing.T) {
	router, _ := seedPortalProbe(t)
	probeFlag := func(user, pass string) bool {
		t.Helper()
		cookie := sessionCookie(t, portalLogin(router, user, pass, "198.51.100.1"))
		res := portalRequest(router, http.MethodGet, "/sub/portal/data", "", "198.51.100.1", cookie)
		var data struct {
			Probe *bool `json:"probe"`
		}
		if err := json.Unmarshal(res.Body.Bytes(), &data); err != nil || res.Code != http.StatusOK || data.Probe == nil {
			t.Fatalf("portal data of %s = %d %s, want 200 with a probe flag", user, res.Code, res.Body)
		}
		return *data.Probe
	}

	if probeFlag("pa@e", "alpha-pass") {
		t.Fatal("probe offered to pa@e before any host was linked")
	}
	linkProbe(t, service.ProbeLinkInput{NodeId: 0, ServerId: "uuid-a"})
	if !probeFlag("pa@e", "alpha-pass") {
		t.Fatal("probe not offered to pa@e although the master, where its inbound runs, is linked")
	}
	if probeFlag("pb@e", "bravo-pass") {
		t.Fatal("probe offered to pb@e, whose only host (the node) has no link")
	}
}
