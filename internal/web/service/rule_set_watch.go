package service

import (
	"math"
	"net/netip"
	"slices"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/ruleset"
)

// The score of what upstream changed since a review. A review is due at
// ruleSetDueScore: replayed over upstream's history, about every two weeks.
const (
	ruleSetDueScore      = 10.0
	scoreNewProvider     = 8.0
	scoreGoneProvider    = 4.0
	scoreCoreRule        = 3.0
	scoreProcessRule     = 2.0
	scoreOtherRule       = 1.0
	scoreRiskyRule       = 5.0
	scoreRemovedCoreRule = 1.0
	scoreRemovedRule     = 0.5
	scorePendingDay      = 0.1
)

// sharedDomains serve far more than one AI service: a rule for one routes all of it.
var sharedDomains = map[string]bool{
	"google.com": true, "googleapis.com": true, "gstatic.com": true, "googleusercontent.com": true,
	"storage.googleapis.com": true, "microsoft.com": true, "live.com": true, "office.com": true, "msn.com": true,
	"azure.com": true, "windows.net": true, "azureedge.net": true, "amazonaws.com": true, "cloudfront.net": true,
	"cloudflare.com": true, "cloudflare.net": true, "github.com": true, "githubusercontent.com": true,
	"apple.com": true, "icloud.com": true, "akamaized.net": true, "akamaihd.net": true, "fastly.net": true,
}

// RuleSetWatch is what the daily check found: each set's upstream changes since its
// last review, and a score saying whether a review is due.
type RuleSetWatch struct {
	Score         float64          `json:"score" example:"14"`
	Threshold     float64          `json:"threshold" example:"10"`
	Due           bool             `json:"due" example:"true"`
	PendingSince  int64            `json:"pendingSince" example:"1735689600000"`
	Sets          []RuleSetChanges `json:"sets"`
	FetchFailures int              `json:"fetchFailures" example:"0"`
	FailingSince  int64            `json:"failingSince" example:"0"`
	FetchError    string           `json:"fetchError" example:""`
}

// RuleSetChanges is what upstream changed in one set's list since its last review.
type RuleSetChanges struct {
	Name          string          `json:"name" example:"ai"`
	ReviewedAt    int64           `json:"reviewedAt" example:"1735689600000"`
	NewProviders  []string        `json:"newProviders"`
	GoneProviders []string        `json:"goneProviders"`
	Added         []RuleSetChange `json:"added"`
	Removed       []RuleSetChange `json:"removed"`
	Score         float64         `json:"score" example:"14"`
}

// RuleSetChange is one rule upstream added or removed. Risk says why an added rule
// may reach beyond one service: keyword, regex, asn, wide, shared, tld or unknown.
type RuleSetChange struct {
	Rule     string `json:"rule" example:"DOMAIN-SUFFIX,example.com"`
	Provider string `json:"provider" example:"Example AI"`
	Tier     string `json:"tier" example:"Core"`
	Risk     string `json:"risk,omitempty" example:"keyword"`
}

// Watch scores what upstream changed since each set's review. Pending changes gain
// weight each day, so small ones still come up for review.
func (s *RuleSetService) Watch(now time.Time) (*RuleSetWatch, error) {
	var sets []model.RuleSet
	if err := database.GetDB().Where("upstream_url <> ''").Order("id").Find(&sets).Error; err != nil {
		return nil, err
	}
	w := &RuleSetWatch{Threshold: ruleSetDueScore, Sets: []RuleSetChanges{}}
	for _, set := range sets {
		if set.FetchFailures > w.FetchFailures {
			w.FetchFailures, w.FailingSince, w.FetchError = set.FetchFailures, set.FailingSince, set.FetchError
		}
		if set.Latest == "" {
			continue
		}
		changes := diffRuleSet(set.Name, ruleset.Parse(set.Reviewed), ruleset.Parse(set.Latest))
		changes.ReviewedAt = set.ReviewedAt
		w.Sets = append(w.Sets, changes)
		w.Score += changes.Score
		if set.PendingSince > 0 && (w.PendingSince == 0 || set.PendingSince < w.PendingSince) {
			w.PendingSince = set.PendingSince
		}
	}
	if w.Score > 0 && w.PendingSince > 0 {
		w.Score += scorePendingDay * float64(now.UnixMilli()-w.PendingSince) / float64((24 * time.Hour).Milliseconds())
	}
	w.Score = math.Round(w.Score*10) / 10
	w.Due = w.PendingSince > 0 && w.Score >= ruleSetDueScore
	return w, nil
}

func diffRuleSet(name string, reviewed, latest []ruleset.Rule) RuleSetChanges {
	out := RuleSetChanges{
		Name: name, NewProviders: []string{}, GoneProviders: []string{},
		Added: []RuleSetChange{}, Removed: []RuleSetChange{},
	}
	before, after := ruleIndex(reviewed), ruleIndex(latest)
	for _, r := range latest {
		if _, kept := before[r.Key()]; kept {
			continue
		}
		change := RuleSetChange{Rule: r.String(), Provider: r.Provider, Tier: r.Tier, Risk: ruleRisk(r)}
		out.Added = append(out.Added, change)
		out.Score += addedRuleScore(r)
		if change.Risk != "" {
			out.Score += scoreRiskyRule
		}
	}
	for _, r := range reviewed {
		if _, kept := after[r.Key()]; kept {
			continue
		}
		out.Removed = append(out.Removed, RuleSetChange{Rule: r.String(), Provider: r.Provider, Tier: r.Tier})
		if r.Tier == "Core" {
			out.Score += scoreRemovedCoreRule
		} else {
			out.Score += scoreRemovedRule
		}
	}
	providersBefore, providersAfter := providerNames(reviewed), providerNames(latest)
	for _, p := range providersAfter {
		if !slices.Contains(providersBefore, p) {
			out.NewProviders = append(out.NewProviders, p)
			out.Score += scoreNewProvider
		}
	}
	for _, p := range providersBefore {
		if !slices.Contains(providersAfter, p) {
			out.GoneProviders = append(out.GoneProviders, p)
			out.Score += scoreGoneProvider
		}
	}
	return out
}

func addedRuleScore(r ruleset.Rule) float64 {
	switch {
	case r.Type == "PROCESS-NAME":
		return scoreProcessRule
	case r.Tier == "Core":
		return scoreCoreRule
	default:
		return scoreOtherRule
	}
}

func ruleIndex(rules []ruleset.Rule) map[string]ruleset.Rule {
	out := make(map[string]ruleset.Rule, len(rules))
	for _, r := range rules {
		out[r.Key()] = r
	}
	return out
}

// providerNames lists the providers in the order the list first names them.
func providerNames(rules []ruleset.Rule) []string {
	var out []string
	for _, r := range rules {
		if r.Provider != "" && !slices.Contains(out, r.Provider) {
			out = append(out, r.Provider)
		}
	}
	return out
}

func ruleRisk(r ruleset.Rule) string {
	switch r.Type {
	case "DOMAIN-KEYWORD":
		return "keyword"
	case "DOMAIN-REGEX":
		return "regex"
	case "IP-ASN":
		return "asn"
	case "IP-CIDR", "IP-CIDR6":
		prefix, err := netip.ParsePrefix(r.Value)
		if err != nil || (prefix.Addr().Is4() && prefix.Bits() < 16) || (!prefix.Addr().Is4() && prefix.Bits() < 32) {
			return "wide"
		}
	case "DOMAIN", "DOMAIN-SUFFIX":
		domain := strings.ToLower(strings.TrimPrefix(r.Value, "."))
		if !strings.Contains(domain, ".") {
			return "tld"
		}
		if sharedDomains[domain] {
			return "shared"
		}
	case "PROCESS-NAME":
	default:
		return "unknown"
	}
	return ""
}
