package abuse

import (
	"fmt"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

func connect(d *Detector, at time.Time, email, host string, port int, outbound string) {
	d.Observe(NewEvent(at, email, host, port, false, "in-443 >> "+outbound))
}

// only returns the one signal of a rule, failing on none or several.
func only(t *testing.T, signals []Signal, rule string) Signal {
	t.Helper()
	var hits []Signal
	for _, s := range signals {
		if s.Rule == rule {
			hits = append(hits, s)
		}
	}
	if len(hits) != 1 {
		t.Fatalf("%s signals = %d (%+v), want 1", rule, len(hits), signals)
	}
	return hits[0]
}

func none(t *testing.T, signals []Signal, rule string) {
	t.Helper()
	for _, s := range signals {
		if s.Rule == rule {
			t.Fatalf("unexpected %s signal %+v", rule, s)
		}
	}
}

// A mail app may try port 25 once or twice; a spammer keeps at it. One run of
// attempts is one strike, not one per check.
func TestSpamNeedsRepeatedPort25AttemptsAndStrikesOnce(t *testing.T) {
	d := NewDetector(DefaultRules())
	for i := range 9 {
		connect(d, t0.Add(time.Duration(i)*time.Minute), "alice", "203.0.113.25", 25, TagSMTP)
	}
	none(t, d.Collect(t0.Add(9*time.Minute)), RuleSpam)

	connect(d, t0.Add(9*time.Minute), "alice", "203.0.113.26", 25, TagSMTP)
	s := only(t, d.Collect(t0.Add(9*time.Minute)), RuleSpam)
	if s.Count != 10 || s.Level != LevelStrike || len(s.Samples) != 2 {
		t.Errorf("spam signal = %+v, want 10 attempts to 2 servers as a strike", s)
	}
	connect(d, t0.Add(9*time.Minute+30*time.Second), "alice", "203.0.113.27", 25, TagSMTP)
	none(t, d.Collect(t0.Add(9*time.Minute+30*time.Second)), RuleSpam)
}

// Browsing reaches hundreds of sites by name; a scan reaches bare addresses.
func TestScanCountsBareIPsNotNamedSites(t *testing.T) {
	d := NewDetector(DefaultRules())
	for i := range 500 {
		connect(d, t0.Add(time.Duration(i%5)*time.Minute), "alice", fmt.Sprintf("site%d.example.com", i), 443, "direct")
	}
	none(t, d.Collect(t0.Add(5*time.Minute)), RuleScan)

	for i := range 150 {
		connect(d, t0.Add(5*time.Minute), "bob", fmt.Sprintf("198.51.%d.%d", i/250, i%250+1), 80, "direct")
	}
	s := only(t, d.Collect(t0.Add(5*time.Minute)), RuleScan)
	if s.Email != "bob" || s.Measure != MeasureIPs || s.Count != 150 {
		t.Errorf("scan signal = %+v, want bob reaching 150 bare IPs", s)
	}
}

func TestScanCatchesPortsOnOneIPAndLoginPortsAcrossIPs(t *testing.T) {
	d := NewDetector(DefaultRules())
	for p := range 50 {
		connect(d, t0, "alice", "192.0.2.10", 1000+p, "direct")
	}
	if s := only(t, d.Collect(t0), RuleScan); s.Measure != MeasurePorts || s.Samples[0] != "192.0.2.10" {
		t.Errorf("port sweep signal = %+v", s)
	}

	d = NewDetector(DefaultRules())
	for i := range 20 {
		connect(d, t0, "bob", fmt.Sprintf("203.0.113.%d", i+1), 22, "direct")
	}
	if s := only(t, d.Collect(t0), RuleScan); s.Measure != MeasureSensitive || s.Count != 20 {
		t.Errorf("login-port sweep signal = %+v", s)
	}
}

// A download manager opens a few dozen connections to one site; a flood opens
// a thousand a minute.
func TestFloodTripsOnABurstNotOnADownloadManager(t *testing.T) {
	d := NewDetector(DefaultRules())
	for range 32 {
		connect(d, t0, "alice", "cdn.example.com", 443, "direct")
	}
	none(t, d.Collect(t0.Add(30*time.Second)), RuleFlood)

	for range 1000 {
		connect(d, t0, "bob", "203.0.113.80", 80, "direct")
	}
	s := only(t, d.Collect(t0.Add(30*time.Second)), RuleFlood)
	if s.Email != "bob" || s.Measure != MeasurePerDest || s.Samples[0] != "203.0.113.80:80" {
		t.Errorf("flood signal = %+v", s)
	}
}

// One busy stretch is browsing; a crawler keeps every window busy.
func TestCrawlerNeedsBusyWindowsInARow(t *testing.T) {
	d := NewDetector(DefaultRules())
	for i := range 700 {
		connect(d, t0.Add(time.Duration(i%10)*time.Minute), "alice", fmt.Sprintf("page%d.example.org", i), 443, "direct")
	}
	none(t, d.Collect(t0.Add(9*time.Minute)), RuleCrawler)

	for i := range 700 {
		connect(d, t0.Add(time.Duration(10+i%10)*time.Minute), "alice", fmt.Sprintf("next%d.example.org", i), 443, "direct")
	}
	if s := only(t, d.Collect(t0.Add(19*time.Minute)), RuleCrawler); s.Measure != MeasureHosts || s.Count != 700 {
		t.Errorf("crawler signal = %+v", s)
	}
}

func speedTest(d *Detector, start time.Time) {
	for i := range 20 {
		connect(d, start.Add(time.Duration(i)*2*time.Second), "alice", "speed.example.net", 443, TagSpeedTest)
	}
}

// A test opens many connections and still counts once; the sixth in an hour trips.
func TestSpeedTestsCountTestsNotConnections(t *testing.T) {
	d := NewDetector(DefaultRules())
	for i := range 5 {
		speedTest(d, t0.Add(time.Duration(i)*5*time.Minute))
	}
	none(t, d.Collect(t0.Add(25*time.Minute)), RuleSpeedTest)

	speedTest(d, t0.Add(30*time.Minute))
	if s := only(t, d.Collect(t0.Add(31*time.Minute)), RuleSpeedTest); s.Measure != MeasureTestsHour || s.Count != 6 {
		t.Errorf("speed test signal = %+v", s)
	}
}

// fullSpeed feeds traffic at mbps in 5-second reports, collecting as a server does.
func fullSpeed(d *Detector, from time.Time, minutes int, mbps int64) []Signal {
	var out []Signal
	per := mbps * 1_000_000 / 8 * 5
	for s := 0; s < minutes*60; s += 5 {
		at := from.Add(time.Duration(s) * time.Second)
		d.ObserveTraffic("alice", per, at)
		out = append(out, d.Collect(at)...)
	}
	return out
}

// A download of under two hours passes; two hours warns, four strikes, and a
// pause in between starts the count again.
func TestFullSpeedWarnsAtTwoHoursStrikesAtFour(t *testing.T) {
	d := NewDetector(DefaultRules())
	none(t, fullSpeed(d, t0, 115, 120), RuleFullSpeed)

	d = NewDetector(DefaultRules())
	got := fullSpeed(d, t0, 100, 120)
	got = append(got, fullSpeed(d, t0.Add(110*time.Minute), 100, 120)...)
	none(t, got, RuleFullSpeed)

	d = NewDetector(DefaultRules())
	warn := only(t, fullSpeed(d, t0, 126, 120), RuleFullSpeed)
	if warn.Level != LevelWarn || warn.Count < 120 {
		t.Errorf("two hours gave %+v, want a warning", warn)
	}
	strike := only(t, fullSpeed(d, t0.Add(126*time.Minute), 120, 120), RuleFullSpeed)
	if strike.Level != LevelStrike || strike.Count < 240 {
		t.Errorf("four hours gave %+v, want a strike", strike)
	}

	// A run that stops on a block boundary ends on a full block: the pause
	// must still start a new run that can warn again.
	d = NewDetector(DefaultRules())
	if first := only(t, fullSpeed(d, t0, 125, 120), RuleFullSpeed); first.Level != LevelWarn {
		t.Fatalf("the first run gave %+v, want a warning", first)
	}
	again := only(t, fullSpeed(d, t0.Add(135*time.Minute), 126, 120), RuleFullSpeed)
	if again.Level != LevelWarn {
		t.Errorf("a new two-hour run after a pause gave %+v, want a fresh warning", again)
	}
}

func TestBitTorrentAttemptsStrike(t *testing.T) {
	d := NewDetector(DefaultRules())
	for i := range 30 {
		connect(d, t0.Add(time.Duration(i%10)*time.Minute), "alice", fmt.Sprintf("198.51.100.%d", i+1), 51413, TagBT)
	}
	if s := only(t, d.Collect(t0.Add(9*time.Minute)), RuleBT); s.Count != 30 {
		t.Errorf("bt signal = %+v", s)
	}
}

func TestParseAccessLine(t *testing.T) {
	cases := []struct {
		line string
		want Event
	}{
		{
			"2026/10/05 14:40:59.123456 from tcp:198.51.100.7:51234 accepted tcp:www.example.com:443 [in-443 >> direct] email: alice",
			Event{Email: "alice", Host: "www.example.com", Port: 443, Tag: "direct"},
		},
		{
			"2026/10/05 14:40:59.123456 from 198.51.100.7:51234 accepted udp:[2001:db8::1]:3478 [in-443 -> abuse-bt] email: bob",
			Event{Email: "bob", Host: "2001:db8::1", IP: true, Port: 3478, UDP: true, Tag: "abuse-bt"},
		},
	}
	for _, c := range cases {
		got, ok := ParseAccessLine(c.line, t0)
		c.want.At = t0
		if !ok || got != c.want {
			t.Errorf("ParseAccessLine(%q) = %+v, %v; want %+v", c.line, got, ok, c.want)
		}
	}
	if _, ok := ParseAccessLine("2026/10/05 14:40:59.123456 [Warning] core: started", t0); ok {
		t.Error("an error line parsed as an access line")
	}
}
