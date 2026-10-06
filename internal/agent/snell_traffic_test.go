package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/agentproto"
	"github.com/mhsanaei/3x-ui/v3/internal/snell"
)

// fakeNft keeps the one table the agent manages the way the kernel would: its
// counters, and which port and direction feeds each of them.
type fakeNft struct {
	counters map[string]int64
	in, out  map[int]string
	missing  bool
	calls    int
}

var (
	fakeCounterDecl = regexp.MustCompile(`counter ((?:in|out)_\d+) \{\}`)
	fakePortRule    = regexp.MustCompile(`(?:tcp|udp) (dport|sport) (\d+) counter name "(\w+)"`)
)

func (f *fakeNft) run(stdin string, args ...string) (string, error) {
	f.calls++
	if f.missing {
		return "", fmt.Errorf("nft: %w", exec.ErrNotFound)
	}
	switch {
	case slices.Equal(args, []string{"-f", "-"}):
		f.counters, f.in, f.out = nil, nil, nil
		if !strings.Contains(stdin, "table inet "+snellTable+" {\n") {
			return "", nil
		}
		f.counters, f.in, f.out = map[string]int64{}, map[int]string{}, map[int]string{}
		for _, m := range fakeCounterDecl.FindAllStringSubmatch(stdin, -1) {
			f.counters[m[1]] = 0
		}
		for _, m := range fakePortRule.FindAllStringSubmatch(stdin, -1) {
			port, _ := strconv.Atoi(m[2])
			if _, ok := f.counters[m[3]]; !ok {
				return "", errors.New("rule names a counter the table does not declare")
			}
			if m[1] == "dport" {
				f.in[port] = m[3]
			} else {
				f.out[port] = m[3]
			}
		}
		return "", nil
	case slices.Equal(args, []string{"-j", "reset", "counters", "table", "inet", snellTable}):
		if f.counters == nil {
			return "", errors.New("Error: No such file or directory")
		}
		items := []map[string]any{{"metainfo": map[string]any{"json_schema_version": 1}}}
		for name, bytes := range f.counters {
			items = append(items, map[string]any{"counter": map[string]any{"family": "inet", "name": name, "table": snellTable, "packets": 1, "bytes": bytes}})
			f.counters[name] = 0
		}
		out, _ := json.Marshal(map[string]any{"nftables": items})
		return string(out), nil
	}
	return "", fmt.Errorf("unexpected nft call %v", args)
}

// send moves bytes through a port the way users do: up arrives, down leaves.
func (f *fakeNft) send(port int, up, down int64) {
	if name, ok := f.in[port]; ok {
		f.counters[name] += up
	}
	if name, ok := f.out[port]; ok {
		f.counters[name] += down
	}
}

func snellAt(id, port int, tag string) snell.Instance {
	return snell.Instance{ID: id, Tag: tag, Port: port, Settings: snell.Settings{PSK: "0123456789abcdef0123456789abcdef", Version: 5}}
}

func byName(counters []agentproto.Counter) map[string][2]int64 {
	out := map[string][2]int64{}
	for _, c := range counters {
		out[c.Name] = [2]int64{c.Up, c.Down}
	}
	return out
}

// Each Snell port's traffic is reported under its inbound's tag, what users sent
// as up and what they received as down, and once only.
func TestSnellTrafficCountsEachPortByDirection(t *testing.T) {
	nft := &fakeNft{}
	s := &snellTraffic{nft: nft.run}
	want := map[int]snell.Instance{11: snellAt(11, 26163, "n3-snell-a"), 12: snellAt(12, 30000, "n3-snell-b")}
	if got := s.collect(want); len(got) != 0 {
		t.Fatalf("before any traffic: %+v", got)
	}
	nft.send(26163, 100, 900)
	nft.send(30000, 5, 0)
	nft.send(443, 1000, 1000)

	got := byName(s.collect(want))
	if len(got) != 2 || got["n3-snell-a"] != [2]int64{100, 900} || got["n3-snell-b"] != [2]int64{5, 0} {
		t.Fatalf("traffic = %v, want a 100/900 and b 5/0 and nothing for other ports", got)
	}
	if again := s.collect(want); len(again) != 0 {
		t.Fatalf("the same bytes came twice: %+v", again)
	}
}

// Moving a Snell inbound to another port keeps what the old port counted, and
// from then on counts the new one.
func TestSnellTrafficKeepsTheBytesAcrossAPortChange(t *testing.T) {
	nft := &fakeNft{}
	s := &snellTraffic{nft: nft.run}
	s.collect(map[int]snell.Instance{11: snellAt(11, 26163, "n3-snell")})
	nft.send(26163, 10, 20)

	moved := map[int]snell.Instance{11: snellAt(11, 26164, "n3-snell")}
	if got := byName(s.collect(moved)); got["n3-snell"] != [2]int64{10, 20} {
		t.Fatalf("after the move: %v, want the old port's 10/20", got)
	}
	nft.send(26163, 7, 7)
	nft.send(26164, 3, 4)
	if got := byName(s.collect(moved)); got["n3-snell"] != [2]int64{3, 4} {
		t.Fatalf("new port: %v, want 3/4 only", got)
	}
}

// A restarted agent reports what the table counted while it was away.
func TestSnellTrafficReportsWhatAnEarlierAgentLeft(t *testing.T) {
	nft := &fakeNft{}
	want := map[int]snell.Instance{11: snellAt(11, 26163, "n3-snell")}
	(&snellTraffic{nft: nft.run}).collect(want)
	nft.send(26163, 50, 60)

	if got := byName((&snellTraffic{nft: nft.run}).collect(want)); got["n3-snell"] != [2]int64{50, 60} {
		t.Fatalf("after a restart: %v, want 50/60", got)
	}
}

// With its last Snell inbound gone the agent removes its table.
func TestSnellTrafficDropsTheTableWithTheLastInbound(t *testing.T) {
	nft := &fakeNft{}
	s := &snellTraffic{nft: nft.run}
	s.collect(map[int]snell.Instance{11: snellAt(11, 26163, "n3-snell")})
	nft.send(26163, 1, 2)
	if got := byName(s.collect(nil)); got["n3-snell"] != [2]int64{1, 2} || nft.counters != nil {
		t.Fatalf("last report %v, table %v; want the bytes reported and the table gone", got, nft.counters)
	}
}

// A host without nftables runs Snell uncounted and stops asking for nft.
func TestSnellTrafficWithoutNftablesReportsNothing(t *testing.T) {
	nft := &fakeNft{missing: true}
	s := &snellTraffic{nft: nft.run}
	want := map[int]snell.Instance{11: snellAt(11, 26163, "n3-snell")}
	for range 3 {
		if got := s.collect(want); len(got) != 0 {
			t.Fatalf("traffic without nft: %+v", got)
		}
	}
	if nft.calls > 2 {
		t.Fatalf("nft was tried %d times, want it given up on", nft.calls)
	}
}

// Snell's traffic reaches the outbox even while Xray's stats are not up.
func TestAgent_ReportsSnellTrafficWithoutXrayStats(t *testing.T) {
	nft := &fakeNft{}
	box, err := openOutbox(filepath.Join(t.TempDir(), "outbox.json"))
	if err != nil {
		t.Fatal(err)
	}
	m := &snellManager{known: map[int]snell.Instance{11: snellAt(11, 26163, "n3-snell")}, counter: &snellTraffic{nft: nft.run}}
	a := &Agent{snell: m, outbox: box, activity: newActivity(onlineGrace)}
	a.pollLocked(time.Now())
	nft.send(26163, 100, 900)
	a.pollLocked(time.Now())

	report := box.next()
	if report == nil || len(report.Inbounds) != 1 || report.Inbounds[0] != (agentproto.Counter{Name: "n3-snell", Up: 100, Down: 900}) {
		t.Fatalf("report = %+v, want n3-snell 100/900", report)
	}
}

// A host without Snell inbounds never runs nft.
func TestSnellTrafficLeavesHostsWithoutSnellAlone(t *testing.T) {
	nft := &fakeNft{}
	s := &snellTraffic{nft: nft.run}
	for range 3 {
		s.collect(nil)
	}
	if nft.calls != 0 {
		t.Fatalf("nft ran %d times on a host without Snell", nft.calls)
	}
}
