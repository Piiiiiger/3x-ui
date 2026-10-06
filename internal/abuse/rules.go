// Package abuse spots accounts that misuse a proxy server, from their
// connections and traffic only, never their content.
package abuse

// Rule names, stored with every hit and shown to people.
const (
	RuleSpam      = "spam"
	RuleBT        = "bt"
	RuleScan      = "scan"
	RuleFlood     = "flood"
	RuleCrawler   = "crawler"
	RuleSpeedTest = "speedtest"
	RuleFullSpeed = "fullspeed"
)

// Routing tags a server with detection on sends classified traffic to: the first
// two are blackholes, speed tests go out as usual and are only counted.
const (
	TagSMTP      = "abuse-smtp"
	TagBT        = "abuse-bt"
	TagSpeedTest = "abuse-speedtest"
	TagBlock     = "abuse-block"
)

// Levels say how far a hit went: a strike is a full hit, a warning the first
// stage of one, which never costs a strike whatever the rule is set to do.
const (
	LevelStrike = "strike"
	LevelWarn   = "warn"
)

// Measures name what a signal's Count counted, so a message can say it.
const (
	MeasureAttempts  = "attempts"
	MeasureIPs       = "ips"
	MeasurePorts     = "ports"
	MeasureSensitive = "sensitive"
	MeasurePerDest   = "perdest"
	MeasureConns     = "conns"
	MeasureHosts     = "hosts"
	MeasureTestsHour = "tests-hour"
	MeasureTestsDay  = "tests-day"
	MeasureMinutes   = "minutes"
)

// Signal is one rule one account tripped, with what made it trip.
type Signal struct {
	Email   string   `json:"email"`
	Rule    string   `json:"rule"`
	Level   string   `json:"level"`
	Measure string   `json:"measure"`
	Count   int64    `json:"count"`
	Limit   int64    `json:"limit"`
	Window  int      `json:"window"`
	Samples []string `json:"samples,omitempty"`
	At      int64    `json:"at"`
}

// Rules are the thresholds, per account and per server; a zero turns a check
// off. Windows are minutes and stay within the hour the detector remembers.
type Rules struct {
	SpamAttempts  int `json:"spamAttempts" example:"10"`
	SpamWindowMin int `json:"spamWindowMin" example:"10"`

	BTAttempts  int `json:"btAttempts" example:"30"`
	BTWindowMin int `json:"btWindowMin" example:"10"`

	ScanIPs          int `json:"scanIps" example:"150"`
	ScanPortsOnIP    int `json:"scanPortsOnIp" example:"50"`
	ScanSensitiveIPs int `json:"scanSensitiveIps" example:"20"`
	ScanWindowMin    int `json:"scanWindowMin" example:"5"`

	FloodPerDest int `json:"floodPerDest" example:"1000"`
	FloodTotal   int `json:"floodTotal" example:"3000"`

	CrawlerConns     int `json:"crawlerConns" example:"4000"`
	CrawlerHosts     int `json:"crawlerHosts" example:"600"`
	CrawlerWindowMin int `json:"crawlerWindowMin" example:"10"`
	CrawlerWindows   int `json:"crawlerWindows" example:"2"`

	SpeedTestsPerHour int `json:"speedTestsPerHour" example:"5"`
	SpeedTestsPerDay  int `json:"speedTestsPerDay" example:"15"`
	SpeedTestGapMin   int `json:"speedTestGapMin" example:"3"`

	FullSpeedMbps      int `json:"fullSpeedMbps" example:"100"`
	FullSpeedWarnMin   int `json:"fullSpeedWarnMin" example:"120"`
	FullSpeedStrikeMin int `json:"fullSpeedStrikeMin" example:"240"`
}

// DefaultRules are the starting thresholds: a week of observing on real traffic
// is meant to tune them before any server bans.
func DefaultRules() Rules {
	return Rules{
		SpamAttempts: 10, SpamWindowMin: 10,
		BTAttempts: 30, BTWindowMin: 10,
		ScanIPs: 150, ScanPortsOnIP: 50, ScanSensitiveIPs: 20, ScanWindowMin: 5,
		FloodPerDest: 1000, FloodTotal: 3000,
		CrawlerConns: 4000, CrawlerHosts: 600, CrawlerWindowMin: 10, CrawlerWindows: 2,
		SpeedTestsPerHour: 5, SpeedTestsPerDay: 15, SpeedTestGapMin: 3,
		FullSpeedMbps: 100, FullSpeedWarnMin: 120, FullSpeedStrikeMin: 240,
	}
}

// ringMinutes is how far back the detector remembers connections.
const ringMinutes = 60

// Normalized keeps each window inside what the detector remembers, so a stored
// setting can never ask for history it does not have.
func (r Rules) Normalized() Rules {
	window := func(v, fallback int) int {
		if v <= 0 {
			return fallback
		}
		return min(v, ringMinutes)
	}
	d := DefaultRules()
	r.SpamWindowMin = window(r.SpamWindowMin, d.SpamWindowMin)
	r.BTWindowMin = window(r.BTWindowMin, d.BTWindowMin)
	r.ScanWindowMin = window(r.ScanWindowMin, d.ScanWindowMin)
	r.CrawlerWindowMin = window(r.CrawlerWindowMin, d.CrawlerWindowMin)
	if r.CrawlerWindows <= 0 {
		r.CrawlerWindows = d.CrawlerWindows
	}
	r.CrawlerWindows = max(1, min(r.CrawlerWindows, ringMinutes/r.CrawlerWindowMin))
	if r.SpeedTestGapMin <= 0 {
		r.SpeedTestGapMin = d.SpeedTestGapMin
	}
	return r
}

// sensitivePorts are remote-login and database ports, the usual targets of a
// scan or a password-guessing run.
var sensitivePorts = map[int]bool{
	22: true, 23: true, 135: true, 139: true, 445: true, 1433: true, 1521: true, 3306: true,
	3389: true, 5432: true, 5900: true, 6379: true, 9200: true, 11211: true, 27017: true,
}
