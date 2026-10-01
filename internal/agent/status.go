package agent

import (
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
	gnet "github.com/shirou/gopsutil/v4/net"

	"github.com/mhsanaei/3x-ui/v3/internal/agentproto"
)

// hostSampler reads the server's load for the panel's node list; network speed
// is the change in interface counters since the previous sample.
type hostSampler struct {
	lastAt   time.Time
	lastSent uint64
	lastRecv uint64
}

func (h *hostSampler) sample(st *agentproto.Status) {
	if pct, err := cpu.Percent(0, false); err == nil && len(pct) > 0 {
		st.CpuPct = pct[0]
	}
	if vm, err := mem.VirtualMemory(); err == nil {
		st.MemPct = vm.UsedPercent
	}
	if up, err := host.Uptime(); err == nil {
		st.UptimeSecs = up
	}
	counters, err := gnet.IOCounters(true)
	if err != nil {
		return
	}
	var sent, recv uint64
	for _, c := range counters {
		if c.Name == "lo" {
			continue
		}
		sent += c.BytesSent
		recv += c.BytesRecv
	}
	now := time.Now()
	if !h.lastAt.IsZero() && sent >= h.lastSent && recv >= h.lastRecv {
		secs := now.Sub(h.lastAt).Seconds()
		if secs > 0 {
			st.NetUp = uint64(float64(sent-h.lastSent) / secs)
			st.NetDown = uint64(float64(recv-h.lastRecv) / secs)
		}
	}
	h.lastAt, h.lastSent, h.lastRecv = now, sent, recv
}
