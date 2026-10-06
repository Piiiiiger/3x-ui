package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

const reviewedUpstream = `# Example rules
payload:
  # Example AI / Core
  - DOMAIN-SUFFIX,example-ai.com
  - PROCESS-NAME,example
  # Example AI / Network
  - IP-CIDR,192.0.2.0/24,no-resolve
`

// upstreamServer serves whatever list the test sets, or the status it sets.
type upstreamServer struct {
	mu     sync.Mutex
	body   string
	status int
	*httptest.Server
}

func newUpstream(t *testing.T, body string) *upstreamServer {
	t.Helper()
	u := &upstreamServer{body: body, status: http.StatusOK}
	u.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		u.mu.Lock()
		defer u.mu.Unlock()
		w.WriteHeader(u.status)
		_, _ = w.Write([]byte(u.body))
	}))
	t.Cleanup(u.Close)
	return u
}

func (u *upstreamServer) serve(status int, body string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.status, u.body = status, body
}

// ruleSetFollowing replaces the seeded sets with one named ai following url.
func ruleSetFollowing(t *testing.T, url string) {
	t.Helper()
	setupSettingTestDB(t)
	db := database.GetDB()
	if err := db.Where("1 = 1").Delete(&model.RuleSet{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.RuleSet{Name: "ai", UpstreamURL: url}).Error; err != nil {
		t.Fatal(err)
	}
}

func ruleSetRow(t *testing.T) model.RuleSet {
	t.Helper()
	var set model.RuleSet
	if err := database.GetDB().Where("name = ?", "ai").First(&set).Error; err != nil {
		t.Fatal(err)
	}
	return set
}

// reviewLatest saves rules as a review of the list last fetched.
func reviewLatest(t *testing.T, rules string) {
	t.Helper()
	detail, err := (&RuleSetService{}).Get("ai")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (&RuleSetService{}).Save("ai", RuleSetInput{Rules: rules, ReviewedHash: detail.LatestHash, Note: "review"}); err != nil {
		t.Fatal(err)
	}
}

func checkUpstream(t *testing.T, at time.Time) {
	t.Helper()
	if err := (&RuleSetService{}).CheckUpstream(context.Background(), at); err != nil {
		t.Fatal(err)
	}
}

var day0 = time.Date(2026, 10, 6, 2, 0, 0, 0, time.UTC)

// Fetching only keeps the upstream list for a review: what subscriptions get
// changes when a review saves rules, never on a fetch.
func TestFetchingUpstreamLeavesTheServedRules(t *testing.T) {
	up := newUpstream(t, reviewedUpstream)
	ruleSetFollowing(t, up.URL)
	checkUpstream(t, day0)
	reviewLatest(t, "DOMAIN-SUFFIX,example-ai.com\n")
	if err := database.GetDB().Model(&model.RuleSet{}).Where("name = ?", "ai").UpdateColumn("updated_at", 1).Error; err != nil {
		t.Fatal(err)
	}

	up.serve(http.StatusOK, reviewedUpstream+"  # Other AI / Core\n  - DOMAIN-SUFFIX,other-ai.com\n")
	checkUpstream(t, day0.Add(24*time.Hour))

	set := ruleSetRow(t)
	if set.Rules != "DOMAIN-SUFFIX,example-ai.com\n" || set.UpdatedAt != 1 {
		t.Fatalf("a fetch changed the served rules to %q, updated at %d", set.Rules, set.UpdatedAt)
	}
	if !strings.Contains(set.Latest, "other-ai.com") || set.Reviewed != reviewedUpstream {
		t.Fatalf("latest/reviewed = %q / %q, want the new list kept beside the reviewed one", set.Latest, set.Reviewed)
	}
}

// A change is pending from the first fetch that brought it until a review, however
// often upstream changes meanwhile; upstream undoing it ends it too.
func TestUpstreamChangesArePendingFromTheirFirstFetch(t *testing.T) {
	up := newUpstream(t, reviewedUpstream)
	ruleSetFollowing(t, up.URL)
	checkUpstream(t, day0)
	reviewLatest(t, "DOMAIN-SUFFIX,example-ai.com\n")
	if got := ruleSetRow(t).PendingSince; got != 0 {
		t.Fatalf("pending since %d right after the review", got)
	}

	up.serve(http.StatusOK, "# moved\n"+reviewedUpstream)
	checkUpstream(t, day0.Add(24*time.Hour))
	if got := ruleSetRow(t).PendingSince; got != 0 {
		t.Fatalf("a change of comments only is pending since %d", got)
	}

	day2 := day0.Add(48 * time.Hour)
	up.serve(http.StatusOK, reviewedUpstream+"  - DOMAIN,api.example-ai.com\n")
	checkUpstream(t, day2)
	up.serve(http.StatusOK, reviewedUpstream+"  - DOMAIN,api.example-ai.com\n  - DOMAIN,cdn.example-ai.com\n")
	checkUpstream(t, day2.Add(24*time.Hour))
	if got := ruleSetRow(t).PendingSince; got != day2.UnixMilli() {
		t.Fatalf("pending since %d, want the first fetch with the change (%d)", got, day2.UnixMilli())
	}

	up.serve(http.StatusOK, reviewedUpstream)
	checkUpstream(t, day2.Add(48*time.Hour))
	if got := ruleSetRow(t).PendingSince; got != 0 {
		t.Fatalf("pending since %d after upstream went back to the reviewed list", got)
	}
}

// A failed fetch keeps the list fetched before; the failures in a row and since
// when are counted until a fetch succeeds.
func TestFailedFetchesAreCountedAndKeepTheLastList(t *testing.T) {
	up := newUpstream(t, reviewedUpstream)
	ruleSetFollowing(t, up.URL)
	checkUpstream(t, day0)

	failures := []struct {
		status int
		body   string
		want   string
	}{
		{http.StatusBadGateway, "", "HTTP 502"},
		{http.StatusOK, "<html>maintenance</html>\n", "not a rule list"},
		{http.StatusOK, "payload:\n" + strings.Repeat("  - DOMAIN,a.example\n", 1<<17), "larger than"},
	}
	for i, f := range failures {
		up.serve(f.status, f.body)
		checkUpstream(t, day0.Add(time.Duration(i+1)*24*time.Hour))
		set := ruleSetRow(t)
		if set.FetchFailures != i+1 || set.FailingSince != day0.Add(24*time.Hour).UnixMilli() || !strings.Contains(set.FetchError, f.want) {
			t.Fatalf("after failure %d: failures=%d since=%d error=%q, want %d since day 1 and %q",
				i+1, set.FetchFailures, set.FailingSince, set.FetchError, i+1, f.want)
		}
		if set.Latest != reviewedUpstream {
			t.Fatalf("a failed fetch replaced the latest list with %q", set.Latest)
		}
	}

	up.serve(http.StatusOK, reviewedUpstream)
	checkUpstream(t, day0.Add(5*24*time.Hour))
	if set := ruleSetRow(t); set.FetchFailures != 0 || set.FailingSince != 0 || set.FetchError != "" {
		t.Fatalf("a good fetch left failures=%d since=%d error=%q", set.FetchFailures, set.FailingSince, set.FetchError)
	}
}

// A save names the upstream list it reviewed by hash: a list fetched after the
// review was read is not marked reviewed, and nothing is saved.
func TestSaveRefusesAReviewOfAnOlderList(t *testing.T) {
	up := newUpstream(t, reviewedUpstream)
	ruleSetFollowing(t, up.URL)
	checkUpstream(t, day0)
	detail, err := (&RuleSetService{}).Get("ai")
	if err != nil {
		t.Fatal(err)
	}

	up.serve(http.StatusOK, reviewedUpstream+"  - DOMAIN,api.example-ai.com\n")
	checkUpstream(t, day0.Add(time.Hour))
	_, err = (&RuleSetService{}).Save("ai", RuleSetInput{Rules: "DOMAIN-SUFFIX,example-ai.com\n", ReviewedHash: detail.LatestHash})
	if err == nil || !strings.Contains(err.Error(), "changed after it was read") {
		t.Fatalf("Save = %v, want the review refused", err)
	}
	if set := ruleSetRow(t); set.Rules != "" || set.Reviewed != "" {
		t.Fatalf("the refused save left rules %q, reviewed %q", set.Rules, set.Reviewed)
	}
}

// Rules a client could not use never reach the set.
func TestSaveRefusesRulesClientsCannotUse(t *testing.T) {
	ruleSetFollowing(t, "")
	_, err := (&RuleSetService{}).Save("ai", RuleSetInput{Rules: "DOMAIN-SUFFIX,example-ai.com\nIP-ASN,64496\n"})
	if err == nil || !strings.Contains(err.Error(), "line 2: IP-ASN") {
		t.Fatalf("Save = %v, want line 2 refused", err)
	}
	if set := ruleSetRow(t); set.Rules != "" {
		t.Fatalf("refused rules were saved: %q", set.Rules)
	}
}

// Saving without a hash changes the rules only: what upstream changed since the
// last review stays pending.
func TestSaveWithoutAHashKeepsTheReviewedList(t *testing.T) {
	up := newUpstream(t, reviewedUpstream)
	ruleSetFollowing(t, up.URL)
	checkUpstream(t, day0)
	reviewLatest(t, "DOMAIN-SUFFIX,example-ai.com\n")
	up.serve(http.StatusOK, reviewedUpstream+"  - DOMAIN,api.example-ai.com\n")
	checkUpstream(t, day0.Add(24*time.Hour))

	if _, err := (&RuleSetService{}).Save("ai", RuleSetInput{Rules: "DOMAIN-SUFFIX,example-ai.com\nPROCESS-NAME,droid\n"}); err != nil {
		t.Fatal(err)
	}
	detail, err := (&RuleSetService{}).Get("ai")
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Changes.Added) != 1 || detail.Changes.Added[0].Rule != "DOMAIN,api.example-ai.com" || detail.PendingSince == 0 {
		t.Fatalf("changes = %+v, pending since %d; want the upstream addition still pending", detail.Changes.Added, detail.PendingSince)
	}
}

// A restore brings back a save's rules and the list it reviewed, so what upstream
// changed since then is pending again; the restore is itself a version.
func TestRestoreBringsBackTheRulesAndTheirReview(t *testing.T) {
	up := newUpstream(t, reviewedUpstream)
	ruleSetFollowing(t, up.URL)
	checkUpstream(t, day0)
	reviewLatest(t, "DOMAIN-SUFFIX,example-ai.com\n")
	first, err := (&RuleSetService{}).Versions("ai")
	if err != nil {
		t.Fatal(err)
	}

	up.serve(http.StatusOK, reviewedUpstream+"  - DOMAIN,api.example-ai.com\n")
	checkUpstream(t, day0.Add(24*time.Hour))
	reviewLatest(t, "DOMAIN-SUFFIX,example-ai.com\nDOMAIN,api.example-ai.com\n")

	if _, err := (&RuleSetService{}).Restore(first[0].Id); err != nil {
		t.Fatal(err)
	}
	set := ruleSetRow(t)
	if set.Rules != "DOMAIN-SUFFIX,example-ai.com\n" || set.Reviewed != reviewedUpstream || set.PendingSince == 0 {
		t.Fatalf("after the restore rules=%q reviewed=%q pending=%d", set.Rules, set.Reviewed, set.PendingSince)
	}
	versions, err := (&RuleSetService{}).Versions("ai")
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 3 || versions[0].RuleCount != 1 || versions[1].RuleCount != 2 {
		t.Fatalf("versions = %+v, want the restore newest, then the two reviews", versions)
	}
}

// Only the newest saves stay restorable.
func TestOnlyTheNewestRuleSetVersionsAreKept(t *testing.T) {
	ruleSetFollowing(t, "")
	for i := range ruleSetVersionsKept + 2 {
		rules := fmt.Sprintf("DOMAIN-SUFFIX,example-ai.com\nDOMAIN,host%d.example\n", i)
		if _, err := (&RuleSetService{}).Save("ai", RuleSetInput{Rules: rules, Note: "save"}); err != nil {
			t.Fatal(err)
		}
	}
	versions, err := (&RuleSetService{}).Versions("ai")
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != ruleSetVersionsKept {
		t.Fatalf("%d versions kept, want %d", len(versions), ruleSetVersionsKept)
	}
}

// watchAfter scores upstream's list against the reviewed one after pending days.
func watchAfter(t *testing.T, latest string, pendingDays int) *RuleSetWatch {
	t.Helper()
	up := newUpstream(t, reviewedUpstream)
	ruleSetFollowing(t, up.URL)
	checkUpstream(t, day0)
	reviewLatest(t, "DOMAIN-SUFFIX,example-ai.com\n")
	up.serve(http.StatusOK, latest)
	checkUpstream(t, day0)
	w, err := (&RuleSetService{}).Watch(day0.Add(time.Duration(pendingDays) * 24 * time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	return w
}

// The score weighs what upstream changed by how much it matters to people: a new
// service most, a rule reaching beyond one service is flagged and weighs more.
func TestUpstreamChangesAreScoredByWhatTheyMean(t *testing.T) {
	cases := []struct {
		name   string
		latest string
		score  float64
		risk   string
	}{
		{"a third-party rule", reviewedUpstream + "  # Example AI / Third-Party\n  - DOMAIN,telemetry.example-ai.net\n", 1, ""},
		{"a process rule", reviewedUpstream + "  - PROCESS-NAME,example-cli\n", 2, ""},
		{"a core rule", reviewedUpstream + "  # Example AI / Core\n  - DOMAIN,api.example-ai.com\n", 3, ""},
		{"a new service", reviewedUpstream + "  # New AI / Core\n  - DOMAIN-SUFFIX,new-ai.com\n  - DOMAIN-SUFFIX,new-ai.dev\n", 14, ""},
		{"a keyword", reviewedUpstream + "  # Example AI / Third-Party\n  - DOMAIN-KEYWORD,sentry\n", 6, "keyword"},
		{"a regex", reviewedUpstream + "  # Example AI / Third-Party\n  - DOMAIN-REGEX,^ws-\\d+\\.example\\.net$\n", 6, "regex"},
		{"an ASN", reviewedUpstream + "  # Example AI / Network\n  - IP-ASN,64496,no-resolve\n", 6, "asn"},
		{"a wide IPv4 range", reviewedUpstream + "  # Example AI / Network\n  - IP-CIDR,198.18.0.0/15,no-resolve\n", 6, "wide"},
		{"a wide IPv6 range", reviewedUpstream + "  # Example AI / Network\n  - IP-CIDR6,2001:db8::/31,no-resolve\n", 6, "wide"},
		{"a whole platform", reviewedUpstream + "  # Example AI / Third-Party\n  - DOMAIN-SUFFIX,googleapis.com\n", 6, "shared"},
		{"a whole top-level domain", reviewedUpstream + "  # Example AI / Core\n  - DOMAIN-SUFFIX,ai\n", 8, "tld"},
		{"a removed core rule", strings.Replace(reviewedUpstream, "  - PROCESS-NAME,example\n", "", 1), 1, ""},
		{"a removed network rule", strings.Replace(reviewedUpstream, "  - IP-CIDR,192.0.2.0/24,no-resolve\n", "", 1), 0.5, ""},
		{"a service gone", "payload:\n  # Other AI / Core\n  - DOMAIN-SUFFIX,other-ai.com\n", 8 + 3 + 4 + 1 + 1 + 0.5, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := watchAfter(t, c.latest, 0)
			if w.Score != c.score {
				t.Fatalf("score = %v, want %v (%+v)", w.Score, c.score, w.Sets)
			}
			var risks []string
			for _, a := range w.Sets[0].Added {
				if a.Risk != "" {
					risks = append(risks, a.Risk)
				}
			}
			if strings.Join(risks, ",") != c.risk {
				t.Fatalf("risks = %v, want %q", risks, c.risk)
			}
		})
	}
}

// A new service names itself, and a service upstream dropped is named as gone.
func TestTheScoreNamesNewAndGoneServices(t *testing.T) {
	w := watchAfter(t, "payload:\n  # Other AI / Core\n  - DOMAIN-SUFFIX,other-ai.com\n", 0)
	set := w.Sets[0]
	if strings.Join(set.NewProviders, ",") != "Other AI" || strings.Join(set.GoneProviders, ",") != "Example AI" {
		t.Fatalf("new %v, gone %v", set.NewProviders, set.GoneProviders)
	}
}

// Small changes add up over time: a point pending 90 days reaches the threshold,
// a review clears it.
func TestPendingChangesGainWeightEachDay(t *testing.T) {
	latest := reviewedUpstream + "  # Example AI / Third-Party\n  - DOMAIN,telemetry.example-ai.net\n"
	if w := watchAfter(t, latest, 89); w.Score != 9.9 || w.Due {
		t.Fatalf("after 89 days score = %v due = %v, want 9.9 and not due", w.Score, w.Due)
	}
	w := watchAfter(t, latest, 90)
	if w.Score != 10 || !w.Due || w.PendingSince != day0.UnixMilli() {
		t.Fatalf("after 90 days score = %v due = %v since %d, want 10, due, since day 0", w.Score, w.Due, w.PendingSince)
	}
	reviewLatest(t, "DOMAIN-SUFFIX,example-ai.com\n")
	if w, _ := (&RuleSetService{}).Watch(day0.Add(91 * 24 * time.Hour)); w.Score != 0 || w.Due {
		t.Fatalf("after the review score = %v due = %v", w.Score, w.Due)
	}
}

// A preview only tries rules a save would take, for sets that exist.
func TestProposedRulesAreCheckedLikeASave(t *testing.T) {
	ruleSetFollowing(t, "")
	s := &RuleSetService{}
	if err := s.CheckProposed(map[string]string{"ai": "DOMAIN-SUFFIX,example-ai.com\n"}); err != nil {
		t.Fatalf("good rules refused: %v", err)
	}
	if err := s.CheckProposed(map[string]string{"ai": "IP-ASN,64496\n"}); err == nil || !strings.Contains(err.Error(), "rule set ai: line 1: IP-ASN") {
		t.Fatalf("CheckProposed(ASN) = %v", err)
	}
	if err := s.CheckProposed(map[string]string{"elsewhere": "DOMAIN,a.example\n"}); err == nil || !strings.Contains(err.Error(), "rule set not found: elsewhere") {
		t.Fatalf("CheckProposed(unknown set) = %v", err)
	}
}

// The earliest pending change dates the wait: a second set changing later neither
// shortens it nor starts a new one.
func TestTheEarliestPendingChangeDatesTheWait(t *testing.T) {
	setupSettingTestDB(t)
	day3 := day0.Add(72 * time.Hour)
	for name, since := range map[string]time.Time{"ai": day0, "ai-cn": day3} {
		res := database.GetDB().Model(&model.RuleSet{}).Where("name = ?", name).Updates(map[string]any{
			"reviewed": reviewedUpstream, "latest": reviewedUpstream + "  - DOMAIN,api.example-ai.com\n", "pending_since": since.UnixMilli(),
		})
		if res.Error != nil || res.RowsAffected != 1 {
			t.Fatalf("seed %s: %v", name, res.Error)
		}
	}
	w, err := (&RuleSetService{}).Watch(day3)
	if err != nil {
		t.Fatal(err)
	}
	if w.PendingSince != day0.UnixMilli() || w.Score != 2.3 {
		t.Fatalf("pending since %d, score %v; want day 0 and 1+1 points plus 3 days", w.PendingSince, w.Score)
	}
}
