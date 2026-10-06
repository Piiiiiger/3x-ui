package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/agentproto"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/snell"
)

// snellTable is the nftables table the agent counts Snell traffic in; it holds
// counters only and accepts everything, so it never filters.
const snellTable = "pigger_snell"

// snellTraffic counts what each managed Snell port moves, by direction: what
// users send arrives on the port, what they receive leaves from it.
type snellTraffic struct {
	nft     func(stdin string, args ...string) (string, error)
	counted map[int]snell.Instance
	pending map[string][2]int64
	off     bool
}

func nftCommand(stdin string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "nft", args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("nft %s: %w", args[0], err)
	}
	return string(out), nil
}

// collect returns each instance's traffic since the last call, first keeping the
// table in step with want. Bytes counted before a rebuild are kept for the report.
func (s *snellTraffic) collect(want map[int]snell.Instance) []agentproto.Counter {
	if s.off || (len(want) == 0 && len(s.counted) == 0) {
		return nil
	}
	known := maps.Clone(s.counted)
	if known == nil {
		known = map[int]snell.Instance{}
	}
	for id, i := range want {
		if _, ok := known[id]; !ok {
			known[id] = i
		}
	}
	s.read(known)
	if !samePorts(want, s.counted) {
		if _, err := s.nft(snellRuleset(want), "-f", "-"); err != nil {
			s.fail(err)
		} else {
			s.counted = maps.Clone(want)
			if s.counted == nil {
				s.counted = map[int]snell.Instance{}
			}
		}
	}
	out := make([]agentproto.Counter, 0, len(s.pending))
	for tag, c := range s.pending {
		out = append(out, agentproto.Counter{Name: tag, Up: c[0], Down: c[1]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	s.pending = nil
	return out
}

// read moves the table's counters into pending and zeroes them in one step. A
// missing table is nothing to read: the next rebuild creates it.
func (s *snellTraffic) read(known map[int]snell.Instance) {
	raw, err := s.nft("", "-j", "reset", "counters", "table", "inet", snellTable)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			s.fail(err)
		}
		return
	}
	var doc struct {
		Nftables []struct {
			Counter *struct {
				Name  string `json:"name"`
				Bytes int64  `json:"bytes"`
			} `json:"counter"`
		} `json:"nftables"`
	}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		logger.Warning("agent: unreadable Snell counters:", err)
		return
	}
	for _, item := range doc.Nftables {
		if item.Counter == nil || item.Counter.Bytes == 0 {
			continue
		}
		dir, idText, ok := strings.Cut(item.Counter.Name, "_")
		id, err := strconv.Atoi(idText)
		inst, found := known[id]
		if !ok || err != nil || !found || (dir != "in" && dir != "out") {
			continue
		}
		if s.pending == nil {
			s.pending = map[string][2]int64{}
		}
		c := s.pending[inst.Tag]
		if dir == "in" {
			c[0] += item.Counter.Bytes
		} else {
			c[1] += item.Counter.Bytes
		}
		s.pending[inst.Tag] = c
	}
}

func (s *snellTraffic) fail(err error) {
	if errors.Is(err, exec.ErrNotFound) {
		s.off = true
		logger.Warning("agent: nftables is not installed, so Snell traffic is not counted")
		return
	}
	logger.Warning("agent: counting Snell traffic failed:", err)
}

func samePorts(a, b map[int]snell.Instance) bool {
	if len(a) != len(b) {
		return false
	}
	for id, i := range a {
		if j, ok := b[id]; !ok || j.Port != i.Port || j.Tag != i.Tag {
			return false
		}
	}
	return true
}

// snellRuleset replaces the whole table in one transaction; without instances it
// only removes it.
func snellRuleset(want map[int]snell.Instance) string {
	var b strings.Builder
	fmt.Fprintf(&b, "table inet %s {}\ndelete table inet %s\n", snellTable, snellTable)
	if len(want) == 0 {
		return b.String()
	}
	ids := snellIDs(want)
	fmt.Fprintf(&b, "table inet %s {\n", snellTable)
	for _, id := range ids {
		fmt.Fprintf(&b, "\tcounter in_%d {}\n\tcounter out_%d {}\n", id, id)
	}
	b.WriteString("\tchain input {\n\t\ttype filter hook input priority filter + 10; policy accept;\n")
	for _, id := range ids {
		p := want[id].Port
		fmt.Fprintf(&b, "\t\ttcp dport %d counter name \"in_%d\"\n\t\tudp dport %d counter name \"in_%d\"\n", p, id, p, id)
	}
	b.WriteString("\t}\n\tchain output {\n\t\ttype filter hook output priority filter + 10; policy accept;\n")
	for _, id := range ids {
		p := want[id].Port
		fmt.Fprintf(&b, "\t\ttcp sport %d counter name \"out_%d\"\n\t\tudp sport %d counter name \"out_%d\"\n", p, id, p, id)
	}
	b.WriteString("\t}\n}\n")
	return b.String()
}
