package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/abuse"
	"github.com/mhsanaei/3x-ui/v3/internal/config"
	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func banFor(t *testing.T, email string, bannedAt, expiresAt int64) *model.BanRecord {
	t.Helper()
	rec := &model.BanRecord{Email: email, Kind: BanKindAbuse, Rule: abuse.RuleScan, BannedAt: bannedAt, ExpiresAt: expiresAt}
	if err := database.GetDB().Create(rec).Error; err != nil {
		t.Fatal(err)
	}
	return rec
}

func blockedUsers(t *testing.T, cfg []byte) []any {
	t.Helper()
	for _, r := range routingRules(t, cfg) {
		if r["outboundTag"] == abuse.TagBlock {
			users, _ := r["user"].([]any)
			return users
		}
	}
	return nil
}

// A ban holds on every server serving the account, detection there or not, and
// ends with its time or when an admin lifts it.
func TestAbuseBanBlocksTheAccountOnEveryServerServingIt(t *testing.T) {
	setupIpLimitTest(t)
	edge, other := seedAgentNodeRow(t, "edge"), seedAgentNodeRow(t, "other")
	seedLimitedClient(t, &edge.Id, "edge-in", 443, "alice", 3)
	seedLimitedClient(t, &other.Id, "other-in", 443, "bob", 3)
	now := time.Now()
	ban := banFor(t, "alice", now.Unix(), now.Add(30*time.Minute).Unix())

	cfg, err := (&XrayService{}).GetAgentXrayConfig(edge.Id)
	if err != nil {
		t.Fatal(err)
	}
	if first := routingRules(t, cfg.RouterConfig)[0]; first["outboundTag"] != abuse.TagBlock || !slices.Equal(blockedUsers(t, cfg.RouterConfig), []any{"alice"}) {
		t.Fatalf("first rule = %v, want alice blocked", first)
	}
	if !slices.Contains(outboundTags(t, cfg.OutboundConfigs), abuse.TagBlock+":blackhole") {
		t.Fatalf("no blackhole for the ban: %s", cfg.OutboundConfigs)
	}
	if cfg, _ = (&XrayService{}).GetAgentXrayConfig(other.Id); blockedUsers(t, cfg.RouterConfig) != nil {
		t.Fatal("a server not serving alice got her ban")
	}

	if err := database.GetDB().Model(ban).Update("lifted_at", now.Unix()).Error; err != nil {
		t.Fatal(err)
	}
	if cfg, _ = (&XrayService{}).GetAgentXrayConfig(edge.Id); blockedUsers(t, cfg.RouterConfig) != nil {
		t.Fatal("a lifted ban still blocks")
	}
	banFor(t, "alice", now.Add(-time.Hour).Unix(), now.Add(-30*time.Minute).Unix())
	if cfg, _ = (&XrayService{}).GetAgentXrayConfig(edge.Id); blockedUsers(t, cfg.RouterConfig) != nil {
		t.Fatal("an expired ban still blocks")
	}
	banFor(t, "alice", now.Unix(), 0)
	if cfg, _ = (&XrayService{}).GetAgentXrayConfig(edge.Id); blockedUsers(t, cfg.RouterConfig) == nil {
		t.Fatal("a lock does not block")
	}
}

// Only a server that detects gets the classification rules and the thresholds.
func TestAbuseChecksGoOnlyToDetectingServers(t *testing.T) {
	setupIpLimitTest(t)
	watched, plain := seedAgentNodeRow(t, "watched"), seedAgentNodeRow(t, "plain")
	seedLimitedClient(t, &watched.Id, "watched-in", 443, "alice", 3)
	seedLimitedClient(t, &plain.Id, "plain-in", 443, "bob", 3)
	if err := (&AbuseService{}).SetMode(watched.Id, AbuseModeObserve); err != nil {
		t.Fatal(err)
	}

	raw, _, err := (&AgentService{}).AgentConfig(watched.Id)
	if err != nil {
		t.Fatal(err)
	}
	core, rules, err := abuse.SplitConfig(raw)
	if err != nil || rules == nil || rules.ScanIPs != abuse.DefaultRules().ScanIPs {
		t.Fatalf("the detecting server's thresholds = %+v, %v", rules, err)
	}
	var cfg struct {
		Routing   json.RawMessage `json:"routing"`
		Outbounds json.RawMessage `json:"outbounds"`
	}
	if err := json.Unmarshal(core, &cfg); err != nil {
		t.Fatal(err)
	}
	var tags []string
	for _, r := range routingRules(t, cfg.Routing) {
		if tag, _ := r["outboundTag"].(string); strings.HasPrefix(tag, "abuse-") {
			tags = append(tags, tag)
		}
	}
	if !slices.Equal(tags, []string{abuse.TagSMTP, abuse.TagBT, abuse.TagSpeedTest}) {
		t.Fatalf("classification rules = %v", tags)
	}

	raw, _, err = (&AgentService{}).AgentConfig(plain.Id)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "abuse-") || strings.Contains(string(raw), abuse.ConfigKey) {
		t.Fatal("a server with detection off got the checks")
	}
}

// The panel's own core logs its connections only while it detects.
func TestLocalCoreLogsConnectionsOnlyWhileDetecting(t *testing.T) {
	setupIpLimitTest(t)
	access := func() string {
		cfg, err := (&XrayService{}).GetXrayConfig()
		if err != nil {
			t.Fatal(err)
		}
		var log struct {
			Access string `json:"access"`
		}
		_ = json.Unmarshal(cfg.LogConfig, &log)
		return log.Access
	}
	if got := access(); got != "none" {
		t.Fatalf("access log with detection off = %q, want none", got)
	}
	if err := (&AbuseService{}).SetMode(0, AbuseModeEnforce); err != nil {
		t.Fatal(err)
	}
	if got := access(); got != filepath.Join(config.GetLogFolder(), AbuseAccessLogName) {
		t.Fatalf("access log while detecting = %q", got)
	}
}

func appendLog(t *testing.T, path, text string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(text); err != nil {
		t.Fatal(err)
	}
}

func accessLine(port int) string {
	return fmt.Sprintf("2026/10/06 08:00:00.000000 from tcp:203.0.113.5:40000 accepted tcp:198.51.100.7:%d [in-443 >> direct] email: alice\n", port)
}

// The panel's own core is checked from its access log: what was logged before
// detection began is skipped, and a line written in two pieces counts once.
func TestLocalAbuseChecksReadTheCoresLog(t *testing.T) {
	setupIpLimitTest(t)
	seedLimitedClient(t, nil, "local-in", 47501, "alice", 3)
	s := &AbuseService{}
	settings := s.Settings()
	settings.Thresholds = abuse.Rules{ScanPortsOnIP: 3, ScanWindowMin: 5}
	if err := s.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	if err := s.SetMode(0, AbuseModeEnforce); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.SetMode(0, AbuseModeOff); _, _ = s.RunAbuseChecks(time.Now()) })
	path := filepath.Join(config.GetLogFolder(), AbuseAccessLogName)
	appendLog(t, path, accessLine(999))
	now := time.Now()

	if _, err := s.RunAbuseChecks(now); err != nil {
		t.Fatal(err)
	}
	appendLog(t, path, accessLine(1001)+accessLine(1002))
	half := accessLine(1003)
	cut := len(half) - 5
	appendLog(t, path, half[:cut])
	if _, err := s.RunAbuseChecks(now); err != nil {
		t.Fatal(err)
	}
	if recs := banRecords(t); len(recs) != 0 {
		t.Fatalf("banned on %+v: an old line or half a line was counted", recs)
	}
	appendLog(t, path, half[cut:])
	changed, err := s.RunAbuseChecks(now)
	if err != nil {
		t.Fatal(err)
	}
	if recs := banRecords(t); !changed || len(recs) != 1 || recs[0].Email != "alice" {
		t.Fatalf("after the third port: changed=%v records=%+v, want alice banned", changed, recs)
	}
}

// A ban ending by itself must reach the servers too, so the account comes back.
func TestAbuseChecksRefreshTheConfigsWhenABanEnds(t *testing.T) {
	setupIpLimitTest(t)
	s := &AbuseService{}
	now := time.Now()
	banFor(t, "alice", now.Unix(), now.Add(30*time.Minute).Unix())
	if _, err := s.RunAbuseChecks(now); err != nil {
		t.Fatal(err)
	}
	if changed, _ := s.RunAbuseChecks(now.Add(time.Minute)); changed {
		t.Fatal("nothing changed, yet the configs were refreshed")
	}
	if changed, _ := s.RunAbuseChecks(now.Add(31 * time.Minute)); !changed {
		t.Fatal("the ban ended and the configs were not refreshed")
	}
}
