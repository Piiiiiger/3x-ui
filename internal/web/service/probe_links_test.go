package service

import (
	"context"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service/probetest"
)

func seedProbeNode(t *testing.T, name, remark, address string) int {
	t.Helper()
	node := &model.Node{Name: name, Remark: remark, Address: address, Kind: model.NodeKindAgent, Enable: true}
	if err := database.GetDB().Create(node).Error; err != nil {
		t.Fatalf("seed node %s: %v", name, err)
	}
	return node.Id
}

func storedProbeLinks(t *testing.T) map[int]string {
	t.Helper()
	var rows []model.ProbeLink
	if err := database.GetDB().Find(&rows).Error; err != nil {
		t.Fatalf("read probe links: %v", err)
	}
	links := make(map[int]string, len(rows))
	for _, row := range rows {
		links[row.NodeId] = row.ServerId
	}
	return links
}

// A link left behind by a deleted node would keep its Lite server claimed, so
// the admin could never link that server to another node.
func TestDeleteNodeRemovesItsProbeLinkAndNoOtherOne(t *testing.T) {
	setupConflictDB(t)
	gone := seedProbeNode(t, "gone", "", "203.0.113.1")
	kept := seedProbeNode(t, "kept", "", "203.0.113.2")
	for _, link := range []model.ProbeLink{
		{NodeId: 0, ServerId: "uuid-master"},
		{NodeId: gone, ServerId: "uuid-gone"},
		{NodeId: kept, ServerId: "uuid-kept"},
	} {
		if err := database.GetDB().Create(&link).Error; err != nil {
			t.Fatalf("seed link for node %d: %v", link.NodeId, err)
		}
	}

	if err := (&NodeService{}).Delete(gone); err != nil {
		t.Fatalf("delete node: %v", err)
	}

	got := storedProbeLinks(t)
	if len(got) != 2 || got[0] != "uuid-master" || got[kept] != "uuid-kept" {
		t.Fatalf("links after deleting node %d = %v, want only the master's and node %d's", gone, got, kept)
	}
}

// Node id 0 is the master's own host in probe_links, not a node: the delete
// route accepts /del/0 and must not unlink the master.
func TestDeleteNodeZeroKeepsTheMasterProbeLink(t *testing.T) {
	setupConflictDB(t)
	if err := database.GetDB().Create(&model.ProbeLink{NodeId: 0, ServerId: "uuid-master"}).Error; err != nil {
		t.Fatalf("seed master link: %v", err)
	}

	if err := (&NodeService{}).Delete(0); err != nil {
		t.Fatalf("delete node 0: %v", err)
	}

	if got := storedProbeLinks(t); got[0] != "uuid-master" {
		t.Fatalf("links after deleting node 0 = %v, want the master's link kept", got)
	}
}

func liteServers(names map[string]string) (nodes, statuses string) {
	list := make(map[string]map[string]string, len(names))
	for id, name := range names {
		list[id] = map[string]string{"name": name}
	}
	raw, _ := json.Marshal(list)
	return string(raw), `{}`
}

func setProbeLinks(t *testing.T, s *ProbeService, links ...ProbeLinkInput) {
	t.Helper()
	if err := s.SetLinks(ProbeLinksInput{Links: links}); err != nil {
		t.Fatalf("set links %+v: %v", links, err)
	}
}

// The link table is the whole mapping between the two systems: the master must
// land on node id 0, and the modal needs every host listed, linked or not.
func TestProbeLinksReadBackExactlyWhatWasSet(t *testing.T) {
	s, _ := setupProbe(t)
	lite := probetest.NewLite(t)
	lite.Answer(liteServers(map[string]string{"uuid-a": "lite a", "uuid-b": "lite b"}))
	useLite(t, s, lite)
	hk := seedProbeNode(t, "edge-hk", "Hong Kong", "203.0.113.11")
	la := seedProbeNode(t, "edge-la", "", "203.0.113.13")

	setProbeLinks(t, s, ProbeLinkInput{NodeId: 0, ServerId: "uuid-a"}, ProbeLinkInput{NodeId: hk, ServerId: " uuid-b "})

	got, err := s.Links(context.Background())
	if err != nil {
		t.Fatalf("links: %v", err)
	}
	want := []ProbeLinkView{
		{NodeId: 0, ServerId: "uuid-a", ServerName: "lite a"},
		{NodeId: hk, NodeName: "Hong Kong", Address: "203.0.113.11", ServerId: "uuid-b", ServerName: "lite b"},
		{NodeId: la, NodeName: "edge-la", Address: "203.0.113.13"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("links =\n %+v\nwant\n %+v", got, want)
	}
	if stored := storedProbeLinks(t); len(stored) != 2 || stored[0] != "uuid-a" || stored[hk] != "uuid-b" {
		t.Fatalf("stored rows = %v, want the master under node id 0 and node %d with the trimmed id", stored, hk)
	}
}

// The admin must be able to see and clear a link whose server left Lite.
func TestProbeLinkToAServerLiteNoLongerListsKeepsItsIdWithoutAName(t *testing.T) {
	s, _ := setupProbe(t)
	lite := probetest.NewLite(t)
	lite.Answer(liteServers(map[string]string{"uuid-a": "lite a"}))
	useLite(t, s, lite)
	setProbeLinks(t, s, ProbeLinkInput{NodeId: 0, ServerId: "uuid-removed"})

	got, err := s.Links(context.Background())
	if err != nil {
		t.Fatalf("links: %v", err)
	}
	if len(got) != 1 || got[0].ServerId != "uuid-removed" || got[0].ServerName != "" {
		t.Fatalf("links = %+v, want the stored id with an empty server name", got)
	}
}

// Saving posts the whole set, so swapping two servers arrives as one save: a
// row-by-row update would trip over the unique server id half-way.
func TestSetProbeLinksReplacesTheWholeSetAtOnce(t *testing.T) {
	s, _ := setupProbe(t)
	node := seedProbeNode(t, "edge-hk", "", "203.0.113.11")
	setProbeLinks(t, s, ProbeLinkInput{NodeId: 0, ServerId: "uuid-a"}, ProbeLinkInput{NodeId: node, ServerId: "uuid-b"})

	setProbeLinks(t, s, ProbeLinkInput{NodeId: 0, ServerId: "uuid-b"}, ProbeLinkInput{NodeId: node, ServerId: "uuid-a"})
	if stored := storedProbeLinks(t); len(stored) != 2 || stored[0] != "uuid-b" || stored[node] != "uuid-a" {
		t.Fatalf("after swapping: %v, want the two servers exchanged", stored)
	}

	setProbeLinks(t, s, ProbeLinkInput{NodeId: 0, ServerId: ""}, ProbeLinkInput{NodeId: node, ServerId: "uuid-a"})
	if stored := storedProbeLinks(t); len(stored) != 1 || stored[node] != "uuid-a" {
		t.Fatalf("after unlinking the master: %v, want only node %d", stored, node)
	}

	setProbeLinks(t, s)
	if stored := storedProbeLinks(t); len(stored) != 0 {
		t.Fatalf("after saving an empty set: %v, want no links", stored)
	}
}

func TestSetProbeLinksRejectsAnInconsistentSetAndKeepsTheOldOne(t *testing.T) {
	s, _ := setupProbe(t)
	node := seedProbeNode(t, "edge-hk", "", "203.0.113.11")
	setProbeLinks(t, s, ProbeLinkInput{NodeId: node, ServerId: "uuid-kept"})
	nodeText := strconv.Itoa(node)

	tests := []struct {
		name  string
		links []ProbeLinkInput
		want  string
	}{
		{"a node that does not exist", []ProbeLinkInput{{NodeId: node + 50, ServerId: "uuid-a"}}, "node " + strconv.Itoa(node+50) + " does not exist"},
		{"a node listed twice", []ProbeLinkInput{{NodeId: node, ServerId: "uuid-a"}, {NodeId: node, ServerId: ""}}, "node " + nodeText + " is listed twice"},
		{"a server linked twice", []ProbeLinkInput{{NodeId: 0, ServerId: "uuid-a"}, {NodeId: node, ServerId: "uuid-a "}}, "server uuid-a is linked to more than one node"},
		{"a server id that is too long", []ProbeLinkInput{{NodeId: 0, ServerId: strings.Repeat("x", 65)}}, "a server id is longer than 64 characters"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := s.SetLinks(ProbeLinksInput{Links: tc.links})
			if err == nil || err.Error() != tc.want {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
			if stored := storedProbeLinks(t); len(stored) != 1 || stored[node] != "uuid-kept" {
				t.Fatalf("a rejected save left %v, want the previous link untouched", stored)
			}
		})
	}
}

// Link data is per request. Written into the cached servers it would be shared
// by concurrent requests and outlive a relink until the next fetch.
func TestProbeOverviewJoinsLinksOntoACopyOfTheCachedServers(t *testing.T) {
	s, _ := setupProbe(t)
	lite := probetest.NewLite(t)
	lite.Answer(liteServers(map[string]string{"uuid-a": "a", "uuid-b": "b", "uuid-c": "c"}))
	useLite(t, s, lite)
	node := seedProbeNode(t, "edge-hk", "Hong Kong", "203.0.113.11")
	setProbeLinks(t, s, ProbeLinkInput{NodeId: 0, ServerId: "uuid-a"}, ProbeLinkInput{NodeId: node, ServerId: "uuid-b"})

	type link struct {
		Linked   bool
		NodeId   int
		NodeName string
	}
	linksOf := func(servers []ProbeServer) map[string]link {
		out := make(map[string]link, len(servers))
		for _, server := range servers {
			out[server.Id] = link{server.Linked, server.NodeId, server.NodeName}
		}
		return out
	}

	overview, err := s.Overview(context.Background())
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	want := map[string]link{"uuid-a": {Linked: true}, "uuid-b": {true, node, "Hong Kong"}, "uuid-c": {}}
	if got := linksOf(overview.Servers); !reflect.DeepEqual(got, want) {
		t.Fatalf("link data = %+v, want %+v", got, want)
	}
	unlinked := map[string]link{"uuid-a": {}, "uuid-b": {}, "uuid-c": {}}
	if got := linksOf(snapshotOf(t, s).Servers); !reflect.DeepEqual(got, unlinked) {
		t.Fatalf("the cached servers carry link data: %+v", got)
	}

	setProbeLinks(t, s, ProbeLinkInput{NodeId: node, ServerId: "uuid-c"})
	overview, err = s.Overview(context.Background())
	if err != nil {
		t.Fatalf("overview after relinking: %v", err)
	}
	want = map[string]link{"uuid-a": {}, "uuid-b": {}, "uuid-c": {true, node, "Hong Kong"}}
	if got := linksOf(overview.Servers); !reflect.DeepEqual(got, want) || lite.Requests() != 1 {
		t.Fatalf("after relinking: %+v with %d Lite requests, want %+v from the cached answer", got, lite.Requests(), want)
	}
}

func TestProbeOverviewReportsTheSnapshotStateWithThePublicURL(t *testing.T) {
	s, clock := setupProbe(t)
	lite := probetest.NewLite(t)
	lite.Answer(liteServers(map[string]string{"uuid-a": "a"}))
	if _, err := s.SaveSettings(ProbeSettings{URL: lite.URL, PublicURL: "https://probe.example.com"}); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	fetchedAt := clock.Now().UnixMilli()
	if _, err := s.Overview(context.Background()); err != nil {
		t.Fatalf("overview: %v", err)
	}

	lite.Override(brokenLite)
	clock.Advance(5 * time.Second)
	got, err := s.Overview(context.Background())
	if err != nil {
		t.Fatalf("overview during the outage: %v", err)
	}
	if !got.Configured || got.PublicURL != "https://probe.example.com" || got.FetchedAt != fetchedAt ||
		!got.Stale || got.Error != "Lite answered HTTP 500" || len(got.Servers) != 1 {
		t.Fatalf("overview during the outage = %+v, want the stale server with the error, fetch time and public URL", got)
	}
}
