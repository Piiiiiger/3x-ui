package service

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/abuse"
	"github.com/mhsanaei/3x-ui/v3/internal/config"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/util/json_util"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
	"github.com/mhsanaei/3x-ui/v3/internal/xray/geodata"
)

// AbuseAccessLogName is the access log the panel's own core writes for the
// abuse checks when nothing else asked for one; the panel trims it as it reads.
const AbuseAccessLogName = "abuse-access.log"

// injectAbuseBans keeps every banned or locked account off the servers of cfg,
// its relay identities too: a ban holds everywhere, not only where detection runs.
func injectAbuseBans(cfg *xray.Config, transitOwners map[string]string, now time.Time) error {
	bans, err := (&AbuseService{}).ActiveBans(now)
	if err != nil || len(bans) == 0 {
		return err
	}
	banned := map[string]bool{}
	for _, b := range bans {
		banned[b.Email] = true
	}
	var users []string
	for _, email := range servedEmails(cfg) {
		owner := email
		if o, ok := transitOwners[email]; ok {
			owner = o
		}
		if banned[owner] {
			users = append(users, email)
		}
	}
	if len(users) == 0 {
		return nil
	}
	slices.Sort(users)
	users = slices.Compact(users)
	return prependRoutingWithOutbound(cfg,
		[]any{map[string]any{"type": "field", "user": users, "outboundTag": abuse.TagBlock}},
		map[string]any{"tag": abuse.TagBlock, "protocol": "blackhole", "settings": map[string]any{}})
}

// injectAbuseChecks routes what a detecting server must tell apart: mail port 25
// and BitTorrent to blackholes, speed tests out as usual under their own tag.
func injectAbuseChecks(cfg *xray.Config) error {
	speedTest := map[string]any{"tag": abuse.TagSpeedTest, "protocol": "freedom", "settings": map[string]any{}}
	var outbounds []map[string]any
	if err := json.Unmarshal(cfg.OutboundConfigs, &outbounds); err != nil {
		return err
	}
	for _, o := range outbounds {
		if o["tag"] == "direct" && o["protocol"] == "freedom" && o["settings"] != nil {
			speedTest["settings"] = o["settings"]
		}
	}
	return prependRoutingWithOutbound(cfg,
		[]any{
			map[string]any{"type": "field", "port": "25", "outboundTag": abuse.TagSMTP},
			map[string]any{"type": "field", "protocol": []string{"bittorrent"}, "outboundTag": abuse.TagBT},
			map[string]any{"type": "field", "domain": abuseSpeedTestDomains(), "outboundTag": abuse.TagSpeedTest},
		},
		map[string]any{"tag": abuse.TagSMTP, "protocol": "blackhole", "settings": map[string]any{}},
		map[string]any{"tag": abuse.TagBT, "protocol": "blackhole", "settings": map[string]any{}},
		speedTest)
}

// abuseSpeedTestExtra are speed-test sites the geosite list leaves out; each
// serves a speed test and nothing else, so a visit is a test.
var abuseSpeedTestExtra = []string{"domain:fast.com", "full:speed.cloudflare.com", "domain:speedtest.cn"}

// The geosite speed-test list, written out: an agent may have no geosite.dat,
// and a config naming a list it lacks is refused whole. Read every 10 minutes.
var abuseSpeedTests struct {
	sync.Mutex
	at      time.Time
	domains []string
}

func abuseSpeedTestDomains() []string {
	abuseSpeedTests.Lock()
	defer abuseSpeedTests.Unlock()
	if time.Since(abuseSpeedTests.at) < 10*time.Minute && abuseSpeedTests.domains != nil {
		return abuseSpeedTests.domains
	}
	domains := slices.Clone(abuseSpeedTestExtra)
	page, err := assetStore().Entries("geosite.dat", "speedtest", "", 0, geodata.MaxPageSize)
	if err != nil {
		logger.Debug("abuse: no geosite speedtest list, using the built-in sites:", err)
	}
	for _, e := range page.Items {
		switch e.Kind {
		case "domain", "full", "regexp", "keyword":
			domains = append(domains, e.Kind+":"+e.Value)
		}
	}
	abuseSpeedTests.at, abuseSpeedTests.domains = time.Now(), domains
	return domains
}

// withAbuseAccessLog has the panel's own core log its connections for the abuse
// checks, keeping an access log someone already chose.
func withAbuseAccessLog(logCfg json_util.RawMessage) json_util.RawMessage {
	parsed := map[string]any{}
	if len(logCfg) > 0 {
		if err := json.Unmarshal(logCfg, &parsed); err != nil {
			return logCfg
		}
	}
	if current, _ := parsed["access"].(string); strings.TrimSpace(current) != "" && !strings.EqualFold(strings.TrimSpace(current), "none") {
		return logCfg
	}
	parsed["access"] = filepath.Join(config.GetLogFolder(), AbuseAccessLogName)
	raw, err := json.Marshal(parsed)
	if err != nil {
		return logCfg
	}
	return raw
}

// withAbuseSettings adds the detection thresholds to an agent's config when its
// host runs detection; the agent takes them out before Xray reads the rest.
func (s *AgentService) withAbuseSettings(nodeID int, raw []byte) ([]byte, error) {
	a := &AbuseService{}
	if a.Mode(nodeID) == AbuseModeOff {
		return raw, nil
	}
	return abuse.JoinConfig(raw, a.Rules())
}
