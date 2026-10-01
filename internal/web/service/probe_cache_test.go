package service

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service/probetest"
)

type probeTestClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *probeTestClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *probeTestClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// setupProbe gives a test its own database, an empty probe cache and a clock
// that only moves when the test says so.
func setupProbe(t *testing.T) (*ProbeService, *probeTestClock) {
	t.Helper()
	setupConflictDB(t)
	clock := &probeTestClock{now: time.Date(2026, 10, 2, 8, 0, 0, 0, time.UTC)}
	realNow, realTimeout := probeNow, probeFetchTimeout
	probeNow = clock.Now
	dropProbeCache()
	t.Cleanup(func() {
		probeNow, probeFetchTimeout = realNow, realTimeout
		dropProbeCache()
	})
	return &ProbeService{}, clock
}

func useLite(t *testing.T, s *ProbeService, lite *probetest.Lite) {
	t.Helper()
	if _, err := s.SaveSettings(ProbeSettings{URL: lite.URL}); err != nil {
		t.Fatalf("point the probe at the fake Lite: %v", err)
	}
}

func oneLiteServer(id, name string) (nodes, statuses string) {
	return `{"` + id + `":{"name":"` + name + `","region":"🇭🇰"}}`,
		`{"` + id + `":{"time":"2026-10-02T07:59:58Z","online":true,"cpu":7,"ping":{}}}`
}

func snapshotOf(t *testing.T, s *ProbeService) ProbeSnapshot {
	t.Helper()
	snap, configured, err := s.Snapshot(context.Background())
	if err != nil || !configured {
		t.Fatalf("snapshot: configured=%v err=%v", configured, err)
	}
	return snap
}

func snapshotNames(snap ProbeSnapshot) []string {
	names := make([]string, 0, len(snap.Servers))
	for _, server := range snap.Servers {
		names = append(names, server.Name)
	}
	return names
}

func brokenLite(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusInternalServerError)
}

// awaitRequest fails the test instead of hanging it when the fetch under test
// never reaches the fake Lite.
func awaitRequest(t *testing.T, entered <-chan struct{}) {
	t.Helper()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("no request reached the fake Lite")
	}
}

// Every open admin page and portal polls; without the cache each poll would be
// a request to Lite, and with a cache that never expires the page would freeze.
func TestProbeSnapshotAsksLiteAtMostOnceEveryTwoSeconds(t *testing.T) {
	s, clock := setupProbe(t)
	lite := probetest.NewLite(t)
	lite.Answer(oneLiteServer("a", "first"))
	useLite(t, s, lite)

	first := snapshotOf(t, s)
	if names := snapshotNames(first); len(names) != 1 || names[0] != "first" || first.Stale || first.Error != "" {
		t.Fatalf("first snapshot = %+v, want the one server, fresh", first)
	}
	if want := clock.Now().UnixMilli(); first.FetchedAt != want {
		t.Fatalf("fetchedAt = %d, want the fetch time %d", first.FetchedAt, want)
	}

	lite.Answer(oneLiteServer("a", "renamed"))
	clock.Advance(1900 * time.Millisecond)
	if names := snapshotNames(snapshotOf(t, s)); len(names) != 1 || names[0] != "first" || lite.Requests() != 1 {
		t.Fatalf("1.9 s later: servers %v after %d requests, want the cached answer and 1 request", names, lite.Requests())
	}

	clock.Advance(100 * time.Millisecond)
	if names := snapshotNames(snapshotOf(t, s)); len(names) != 1 || names[0] != "renamed" || lite.Requests() != 2 {
		t.Fatalf("2 s later: servers %v after %d requests, want a new answer and 2 requests", names, lite.Requests())
	}
}

// A Lite that is down must not be retried by every poll of every viewer.
func TestProbeSnapshotRemembersAFailureForTwoSeconds(t *testing.T) {
	s, clock := setupProbe(t)
	lite := probetest.NewLite(t)
	lite.Override(brokenLite)
	useLite(t, s, lite)

	for _, wait := range []time.Duration{0, 1900 * time.Millisecond} {
		clock.Advance(wait)
		snap := snapshotOf(t, s)
		if snap.Error != "Lite answered HTTP 500" || snap.Stale || snap.FetchedAt != 0 || snap.Servers == nil || len(snap.Servers) != 0 {
			t.Fatalf("snapshot of a broken Lite = %+v, want the error and an empty, non-nil server list", snap)
		}
	}
	if n := lite.Requests(); n != 1 {
		t.Fatalf("two calls within 2 s of a failure reached Lite %d times, want 1", n)
	}

	clock.Advance(100 * time.Millisecond)
	snapshotOf(t, s)
	if n := lite.Requests(); n != 2 {
		t.Fatalf("after 2 s Lite was asked %d times in total, want 2", n)
	}
}

// A short Lite outage should show the last figures marked stale, but figures
// older than 90 s would pass off a dead server as alive.
func TestProbeSnapshotServesTheLastGoodAnswerForNinetySecondsOfFailures(t *testing.T) {
	s, clock := setupProbe(t)
	lite := probetest.NewLite(t)
	lite.Answer(oneLiteServer("a", "good"))
	useLite(t, s, lite)
	goodAt := clock.Now().UnixMilli()
	snapshotOf(t, s)

	lite.Override(brokenLite)
	clock.Advance(88 * time.Second)
	stale := snapshotOf(t, s)
	if names := snapshotNames(stale); len(names) != 1 || names[0] != "good" || !stale.Stale ||
		stale.Error != "Lite answered HTTP 500" || stale.FetchedAt != goodAt {
		t.Fatalf("88 s into the outage = %+v, want the last good servers, stale, with the error and the old fetch time", stale)
	}

	clock.Advance(2 * time.Second)
	gone := snapshotOf(t, s)
	if len(gone.Servers) != 0 || gone.Stale || gone.FetchedAt != 0 || gone.Error != "Lite answered HTTP 500" {
		t.Fatalf("90 s into the outage = %+v, want only the error", gone)
	}

	lite.Override(nil)
	clock.Advance(2 * time.Second)
	back := snapshotOf(t, s)
	if names := snapshotNames(back); len(names) != 1 || back.Stale || back.Error != "" || back.FetchedAt != clock.Now().UnixMilli() {
		t.Fatalf("after Lite recovered = %+v, want a fresh answer", back)
	}
}

// The cache belongs to one Lite address: neither a fresh nor a stale answer
// fetched from another address may be served after the address changes.
func TestProbeSnapshotFollowsALiteAddressChangedInTheDatabase(t *testing.T) {
	s, clock := setupProbe(t)
	oldLite, newLite := probetest.NewLite(t), probetest.NewLite(t)
	oldLite.Answer(oneLiteServer("a", "old"))
	newLite.Override(brokenLite)
	useLite(t, s, oldLite)
	if names := snapshotNames(snapshotOf(t, s)); len(names) != 1 || names[0] != "old" {
		t.Fatalf("servers = %v, want the old Lite's", names)
	}

	if err := database.GetDB().Model(&model.Setting{}).Where("key = ?", "probeLiteURL").
		Update("value", newLite.URL).Error; err != nil {
		t.Fatalf("change the address behind the service's back: %v", err)
	}
	failing := snapshotOf(t, s)
	if len(failing.Servers) != 0 || failing.Stale || failing.Error != "Lite answered HTTP 500" || newLite.Requests() != 1 {
		t.Fatalf("right after the change = %+v after %d requests to the new Lite, want its failure and nothing of the old one",
			failing, newLite.Requests())
	}

	newLite.Override(nil)
	newLite.Answer(oneLiteServer("b", "new"))
	clock.Advance(2 * time.Second)
	if names := snapshotNames(snapshotOf(t, s)); len(names) != 1 || names[0] != "new" {
		t.Fatalf("servers once the new Lite answers = %v, want its own", names)
	}
}

func TestSavingProbeSettingsDropsTheCachedAnswer(t *testing.T) {
	s, _ := setupProbe(t)
	lite := probetest.NewLite(t)
	lite.Answer(oneLiteServer("a", "before"))
	useLite(t, s, lite)
	snapshotOf(t, s)

	lite.Override(brokenLite)
	if _, err := s.SaveSettings(ProbeSettings{URL: lite.URL, PublicURL: "https://probe.example.com"}); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	snap := snapshotOf(t, s)
	if lite.Requests() != 2 || len(snap.Servers) != 0 || snap.Stale {
		t.Fatalf("after saving settings: %d requests, snapshot %+v; want a new fetch and nothing kept from before", lite.Requests(), snap)
	}
}

// Without single-flight a slow Lite would get one request per waiting poll.
func TestProbeSnapshotSharesOneFetchBetweenCallers(t *testing.T) {
	s, _ := setupProbe(t)
	probeFetchTimeout = 150 * time.Millisecond
	lite := probetest.NewLite(t)
	entered := make(chan struct{}, 4)
	lite.Override(func(_ http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		<-r.Context().Done()
	})
	useLite(t, s, lite)

	first := make(chan ProbeSnapshot, 1)
	go func() {
		snap, _, _ := s.Snapshot(context.Background())
		first <- snap
	}()
	awaitRequest(t, entered)
	second := snapshotOf(t, s)

	const want = "Lite did not answer in 150ms"
	if got := <-first; got.Error != want || second.Error != want {
		t.Fatalf("errors = %q and %q, want both callers to get %q", got.Error, second.Error, want)
	}
	if n := lite.Requests(); n != 1 {
		t.Fatalf("two callers during one slow fetch reached Lite %d times, want 1", n)
	}
}

// The fetch belongs to everyone waiting for it: a browser tab that closes must
// not turn the answer of the other viewers into an error.
func TestProbeSnapshotKeepsFetchingWhenItsFirstCallerLeaves(t *testing.T) {
	s, _ := setupProbe(t)
	probeFetchTimeout = 10 * time.Second
	lite := probetest.NewLite(t)
	entered, release := make(chan struct{}, 4), make(chan struct{})
	lite.Override(func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		select {
		case <-release:
			_, _ = w.Write([]byte(`[{"jsonrpc":"2.0","id":1,"result":{"a":{"name":"kept"}}},{"jsonrpc":"2.0","id":2,"result":{}}]`))
		case <-r.Context().Done():
		}
	})
	useLite(t, s, lite)

	leaving, leave := context.WithCancel(context.Background())
	left := make(chan error, 1)
	go func() {
		_, _, err := s.Snapshot(leaving)
		left <- err
	}()
	awaitRequest(t, entered)
	leave()
	if err := <-left; !errors.Is(err, context.Canceled) {
		t.Fatalf("the caller that left got %v, want its own context error", err)
	}

	close(release)
	snap := snapshotOf(t, s)
	if names := snapshotNames(snap); len(names) != 1 || names[0] != "kept" || snap.Error != "" {
		t.Fatalf("the caller that stayed got %+v, want the server", snap)
	}
	if n := lite.Requests(); n != 1 {
		t.Fatalf("Lite was asked %d times, want the one fetch to be shared", n)
	}
}

// An answer that was asked for before the settings changed must not become
// the cached answer of the new settings.
func TestSavingProbeSettingsDisownsAFetchInTheAir(t *testing.T) {
	s, _ := setupProbe(t)
	probeFetchTimeout = 10 * time.Second
	lite := probetest.NewLite(t)
	entered, release := make(chan struct{}, 4), make(chan struct{})
	lite.Override(func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		select {
		case <-release:
			_, _ = w.Write([]byte(`[{"jsonrpc":"2.0","id":1,"result":{}},{"jsonrpc":"2.0","id":2,"result":{}}]`))
		case <-r.Context().Done():
		}
	})
	useLite(t, s, lite)

	asked := make(chan ProbeSnapshot, 1)
	go func() {
		snap, _, _ := s.Snapshot(context.Background())
		asked <- snap
	}()
	awaitRequest(t, entered)
	if _, err := s.SaveSettings(ProbeSettings{URL: lite.URL}); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	close(release)
	if snap := <-asked; snap.Error != "" {
		t.Fatalf("the caller that was already waiting got %+v, want its answer", snap)
	}

	snapshotOf(t, s)
	if n := lite.Requests(); n != 2 {
		t.Fatalf("Lite was asked %d times, want a second fetch after the settings were saved", n)
	}
}
