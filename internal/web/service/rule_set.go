package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"time"

	"gorm.io/gorm"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
	"github.com/mhsanaei/3x-ui/v3/internal/util/ruleset"
)

// ruleSetVersionsKept is how many saves of each rule set stay restorable.
const ruleSetVersionsKept = 30

// ruleSetFetchLimit bounds an upstream list; the ones followed are a few KB.
const ruleSetFetchLimit = 2 << 20

var ruleSetHTTPClient = &http.Client{Timeout: 30 * time.Second}

// RuleSetService keeps the rule sets templates reference by RULE-SET: the rules a
// review saved, their versions, and the upstream lists they follow.
type RuleSetService struct{}

// RuleSetSummary is a set in the list, without its rules or upstream lists.
type RuleSetSummary struct {
	Name          string `json:"name" example:"ai"`
	UpstreamUrl   string `json:"upstreamUrl" example:"https://example.com/rules.yaml"`
	RuleCount     int    `json:"ruleCount" example:"334"`
	UpdatedAt     int64  `json:"updatedAt" example:"1735689600000"`
	ReviewedAt    int64  `json:"reviewedAt" example:"1735689600000"`
	FetchedAt     int64  `json:"fetchedAt" example:"1735689600000"`
	PendingSince  int64  `json:"pendingSince" example:"0"`
	FetchFailures int    `json:"fetchFailures" example:"0"`
	FetchError    string `json:"fetchError" example:""`
}

// RuleSetDetail is a set with its rules, the upstream list as reviewed and as last
// fetched, and what upstream changed between the two.
type RuleSetDetail struct {
	Name         string         `json:"name" example:"ai"`
	UpstreamUrl  string         `json:"upstreamUrl" example:"https://example.com/rules.yaml"`
	Rules        string         `json:"rules" example:"DOMAIN-SUFFIX,example.com"`
	Reviewed     string         `json:"reviewed" example:"payload:\n  - DOMAIN-SUFFIX,example.com"`
	Latest       string         `json:"latest" example:"payload:\n  - DOMAIN-SUFFIX,example.com"`
	LatestHash   string         `json:"latestHash" example:"a3f1c2"`
	ReviewedAt   int64          `json:"reviewedAt" example:"1735689600000"`
	FetchedAt    int64          `json:"fetchedAt" example:"1735689600000"`
	PendingSince int64          `json:"pendingSince" example:"0"`
	Changes      RuleSetChanges `json:"changes"`
}

// RuleSetInput is a review's result: the rules to serve, and the hash of the upstream
// list it covered; without a hash the set's reviewed list stays as it was.
type RuleSetInput struct {
	Rules        string `json:"rules" example:"DOMAIN-SUFFIX,example.com"`
	ReviewedHash string `json:"reviewedHash" example:"a3f1c2"`
	Note         string `json:"note" example:"Factory added"`
}

// RuleSetVersionView is one kept save, without its lists.
type RuleSetVersionView struct {
	Id        int    `json:"id" example:"7"`
	RuleCount int    `json:"ruleCount" example:"334"`
	Note      string `json:"note" example:"Factory added"`
	SavedAt   int64  `json:"savedAt" example:"1735689600000"`
}

func ruleSetSummary(set *model.RuleSet) *RuleSetSummary {
	return &RuleSetSummary{
		Name: set.Name, UpstreamUrl: set.UpstreamURL, RuleCount: len(ruleset.Parse(set.Rules)),
		UpdatedAt: set.UpdatedAt, ReviewedAt: set.ReviewedAt, FetchedAt: set.FetchedAt,
		PendingSince: set.PendingSince, FetchFailures: set.FetchFailures, FetchError: set.FetchError,
	}
}

func (s *RuleSetService) List() ([]RuleSetSummary, error) {
	var sets []model.RuleSet
	if err := database.GetDB().Order("id").Find(&sets).Error; err != nil {
		return nil, err
	}
	out := make([]RuleSetSummary, 0, len(sets))
	for i := range sets {
		out = append(out, *ruleSetSummary(&sets[i]))
	}
	return out, nil
}

func findRuleSet(tx *gorm.DB, name string) (*model.RuleSet, error) {
	var set model.RuleSet
	if err := tx.Where("name = ?", name).First(&set).Error; err != nil {
		return nil, common.NewError("rule set not found:", name)
	}
	return &set, nil
}

func (s *RuleSetService) Get(name string) (*RuleSetDetail, error) {
	set, err := findRuleSet(database.GetDB(), name)
	if err != nil {
		return nil, err
	}
	return &RuleSetDetail{
		Name: set.Name, UpstreamUrl: set.UpstreamURL, Rules: set.Rules, Reviewed: set.Reviewed, Latest: set.Latest,
		LatestHash: upstreamHash(set.Latest), ReviewedAt: set.ReviewedAt, FetchedAt: set.FetchedAt,
		PendingSince: set.PendingSince, Changes: diffRuleSet(set.Name, ruleset.Parse(set.Reviewed), ruleset.Parse(set.Latest)),
	}, nil
}

func upstreamHash(list string) string {
	if list == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(list))
	return hex.EncodeToString(sum[:])
}

// Save serves a review's rules. A hash naming the latest upstream list marks that
// list reviewed; one naming an older list is refused, its changes unseen.
func (s *RuleSetService) Save(name string, in RuleSetInput) (*RuleSetSummary, error) {
	if err := ruleset.Check(in.Rules); err != nil {
		return nil, common.NewError("rules:", err)
	}
	var out *RuleSetSummary
	err := database.GetDB().Transaction(func(tx *gorm.DB) error {
		set, err := findRuleSet(tx, name)
		if err != nil {
			return err
		}
		set.Rules = in.Rules
		if in.ReviewedHash != "" {
			if in.ReviewedHash != upstreamHash(set.Latest) {
				return common.NewError("the upstream list changed after it was read: read the set again and review what changed")
			}
			set.Reviewed, set.ReviewedAt, set.PendingSince = set.Latest, time.Now().UnixMilli(), 0
		}
		if err := tx.Model(set).Select("rules", "reviewed", "reviewed_at", "pending_since", "updated_at").Updates(set).Error; err != nil {
			return err
		}
		out = ruleSetSummary(set)
		return keepRuleSetVersion(tx, set, in.Note)
	})
	return out, err
}

// CheckProposed refuses a preview's proposed rules that a save would refuse, or
// that name no set.
func (s *RuleSetService) CheckProposed(sets map[string]string) error {
	for name, rules := range sets {
		if _, err := findRuleSet(database.GetDB(), name); err != nil {
			return err
		}
		if err := ruleset.Check(rules); err != nil {
			return common.NewError("rule set "+name+":", err)
		}
	}
	return nil
}

func (s *RuleSetService) Versions(name string) ([]RuleSetVersionView, error) {
	set, err := findRuleSet(database.GetDB(), name)
	if err != nil {
		return nil, err
	}
	var versions []model.RuleSetVersion
	if err := database.GetDB().Where("rule_set_id = ?", set.Id).Order("id DESC").Find(&versions).Error; err != nil {
		return nil, err
	}
	out := make([]RuleSetVersionView, 0, len(versions))
	for _, v := range versions {
		out = append(out, RuleSetVersionView{Id: v.Id, RuleCount: len(ruleset.Parse(v.Rules)), Note: v.Note, SavedAt: v.SavedAt})
	}
	return out, nil
}

// Restore brings back a save's rules and the upstream list it reviewed, itself a
// new save; what upstream changed since that review is pending again.
func (s *RuleSetService) Restore(versionId int) (*RuleSetSummary, error) {
	var out *RuleSetSummary
	err := database.GetDB().Transaction(func(tx *gorm.DB) error {
		var v model.RuleSetVersion
		if err := tx.First(&v, versionId).Error; err != nil {
			return common.NewError("rule set version not found:", versionId)
		}
		var set model.RuleSet
		if err := tx.First(&set, v.RuleSetId).Error; err != nil {
			return common.NewError("rule set not found:", v.RuleSetId)
		}
		set.Rules, set.Reviewed, set.ReviewedAt = v.Rules, v.Reviewed, v.SavedAt
		set.PendingSince = pendingSince(&set, time.Now())
		if err := tx.Model(&set).Select("rules", "reviewed", "reviewed_at", "pending_since", "updated_at").Updates(&set).Error; err != nil {
			return err
		}
		out = ruleSetSummary(&set)
		return keepRuleSetVersion(tx, &set, fmt.Sprintf("restored version %d", v.Id))
	})
	return out, err
}

// pendingSince keeps when the latest list began to differ from the reviewed one,
// starting now if it just did; 0 while their rules are the same.
func pendingSince(set *model.RuleSet, now time.Time) int64 {
	changes := diffRuleSet(set.Name, ruleset.Parse(set.Reviewed), ruleset.Parse(set.Latest))
	switch {
	case set.Latest == "" || len(changes.Added)+len(changes.Removed) == 0:
		return 0
	case set.PendingSince > 0:
		return set.PendingSince
	default:
		return now.UnixMilli()
	}
}

func keepRuleSetVersion(tx *gorm.DB, set *model.RuleSet, note string) error {
	if err := tx.Create(&model.RuleSetVersion{RuleSetId: set.Id, Rules: set.Rules, Reviewed: set.Reviewed, Note: note}).Error; err != nil {
		return err
	}
	var keep []int
	if err := tx.Model(&model.RuleSetVersion{}).Where("rule_set_id = ?", set.Id).
		Order("id DESC").Limit(ruleSetVersionsKept).Pluck("id", &keep).Error; err != nil {
		return err
	}
	return tx.Where("rule_set_id = ? AND id NOT IN ?", set.Id, keep).Delete(&model.RuleSetVersion{}).Error
}

// CheckUpstream fetches each set's upstream list and keeps it for the next review;
// nothing reaches subscriptions until a review saves rules.
func (s *RuleSetService) CheckUpstream(ctx context.Context, now time.Time) error {
	db := database.GetDB()
	var sets []model.RuleSet
	if err := db.Where("upstream_url <> ''").Order("id").Find(&sets).Error; err != nil {
		return err
	}
	for i := range sets {
		set := &sets[i]
		latest, err := fetchUpstreamList(ctx, set.UpstreamURL)
		var columns map[string]any
		if err != nil {
			logger.Warningf("rule set %s: fetching its upstream list failed: %v", set.Name, err)
			since := set.FailingSince
			if since == 0 {
				since = now.UnixMilli()
			}
			columns = map[string]any{"fetch_failures": set.FetchFailures + 1, "failing_since": since, "fetch_error": err.Error()}
		} else {
			set.Latest = latest
			columns = map[string]any{
				"latest": latest, "fetched_at": now.UnixMilli(), "pending_since": pendingSince(set, now),
				"fetch_failures": 0, "failing_since": 0, "fetch_error": "",
			}
		}
		// UpdateColumns leaves updated_at alone: it says when the served rules changed.
		if err := db.Model(&model.RuleSet{}).Where("id = ?", set.Id).UpdateColumns(columns).Error; err != nil {
			return err
		}
	}
	return nil
}

func fetchUpstreamList(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "pigger-rule-sets")
	resp, err := ruleSetHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, ruleSetFetchLimit+1))
	if err != nil {
		return "", err
	}
	if len(body) > ruleSetFetchLimit {
		return "", fmt.Errorf("larger than %d MB", ruleSetFetchLimit>>20)
	}
	rules := ruleset.Parse(string(body))
	if len(rules) == 0 || slices.ContainsFunc(rules, func(r ruleset.Rule) bool { return r.Value == "" }) {
		return "", errors.New("not a rule list: every line should be TYPE,VALUE")
	}
	return string(body), nil
}
