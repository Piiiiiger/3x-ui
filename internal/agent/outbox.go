package agent

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"sort"
	"sync"

	"github.com/mhsanaei/3x-ui/v3/internal/agentproto"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
)

// outbox holds traffic until the panel acks it. Only add saves: the saved state
// rebuilds any later report with the same number and usage, so a crash only resends.
type outbox struct {
	mu   sync.Mutex
	path string
	data outboxData
}

type outboxData struct {
	Instance string              `json:"instance"`
	NextSeq  int64               `json:"nextSeq"`
	InFlight *agentproto.Traffic `json:"inFlight,omitempty"`
	Inbounds map[string][2]int64 `json:"inbounds"`
	Clients  map[string][2]int64 `json:"clients"`
}

func openOutbox(path string) (*outbox, error) {
	o := &outbox{path: path}
	raw, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := json.Unmarshal(raw, &o.data); err != nil {
			return nil, err
		}
	case errors.Is(err, os.ErrNotExist):
		id := make([]byte, 8)
		if _, err := rand.Read(id); err != nil {
			return nil, err
		}
		o.data = outboxData{Instance: hex.EncodeToString(id), NextSeq: 1}
	default:
		return nil, err
	}
	if o.data.Inbounds == nil {
		o.data.Inbounds = map[string][2]int64{}
	}
	if o.data.Clients == nil {
		o.data.Clients = map[string][2]int64{}
	}
	return o, nil
}

func (o *outbox) add(inbounds, clients []agentproto.Counter) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if accumulate(o.data.Inbounds, inbounds)+accumulate(o.data.Clients, clients) == 0 {
		return
	}
	o.persist()
}

func accumulate(into map[string][2]int64, counters []agentproto.Counter) int {
	moved := 0
	for _, c := range counters {
		if c.Up == 0 && c.Down == 0 {
			continue
		}
		cur := into[c.Name]
		into[c.Name] = [2]int64{cur[0] + c.Up, cur[1] + c.Down}
		moved++
	}
	return moved
}

// next is the report to send: the unacked one if there is one, otherwise a new
// one holding everything queued; nil when there is nothing to report.
func (o *outbox) next() *agentproto.Traffic {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.data.InFlight != nil {
		return o.data.InFlight
	}
	if len(o.data.Inbounds) == 0 && len(o.data.Clients) == 0 {
		return nil
	}
	o.data.InFlight = &agentproto.Traffic{
		Instance: o.data.Instance,
		Seq:      o.data.NextSeq,
		Inbounds: drain(o.data.Inbounds),
		Clients:  drain(o.data.Clients),
	}
	o.data.NextSeq++
	return o.data.InFlight
}

func drain(from map[string][2]int64) []agentproto.Counter {
	out := make([]agentproto.Counter, 0, len(from))
	for name, v := range from {
		out = append(out, agentproto.Counter{Name: name, Up: v[0], Down: v[1]})
		delete(from, name)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (o *outbox) ack(instance string, seq int64) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.data.InFlight == nil || o.data.InFlight.Instance != instance || o.data.InFlight.Seq != seq {
		return
	}
	o.data.InFlight = nil
}

// persist keeps the queue in memory when the disk refuses it: losing it on the
// next restart beats refusing to account for live traffic now.
func (o *outbox) persist() {
	if err := o.save(); err != nil {
		logger.Warning("agent: saving the traffic queue failed:", err)
	}
}

func (o *outbox) save() error {
	raw, err := json.Marshal(o.data)
	if err != nil {
		return err
	}
	return writeFileAtomic(o.path, raw, 0o600)
}
