// Package agentproto is the wire format between the panel and pigger-agent.
// Both sides exchange JSON Messages over one WebSocket the agent opens.
package agentproto

import (
	"encoding/json"

	"github.com/mhsanaei/3x-ui/v3/internal/snell"
)

// ConnectPath is where an agent dials, relative to the panel's web base path.
const ConnectPath = "agent/connect"

// Message types. The agent sends hello first, then status and traffic on its
// own schedule; the panel sends apply and restart and acks every traffic report.
const (
	TypeHello       = "hello"
	TypeStatus      = "status"
	TypeTraffic     = "traffic"
	TypeResult      = "result"
	TypeApply       = "apply"
	TypeRestart     = "restart"
	TypeAck         = "ack"
	TypeProbe       = "probe"
	ProbeCapability = "tcp-probe-v1"
)

// Message is one frame; ID pairs an apply or restart with its result.
type Message struct {
	Probe   *Probe   `json:"probe,omitempty"`
	Type    string   `json:"type"`
	ID      uint64   `json:"id,omitempty"`
	Hello   *Hello   `json:"hello,omitempty"`
	Status  *Status  `json:"status,omitempty"`
	Traffic *Traffic `json:"traffic,omitempty"`
	Result  *Result  `json:"result,omitempty"`
	Apply   *Apply   `json:"apply,omitempty"`
	Ack     *Ack     `json:"ack,omitempty"`
}

type Hello struct {
	Capabilities []string `json:"capabilities,omitempty"`
	AgentVersion string   `json:"agentVersion"`
	XrayVersion  string   `json:"xrayVersion"`
	// ConfigHash is the hash of the config the agent is running, "" for none.
	ConfigHash string `json:"configHash"`
}

// Status is the agent's latest state; a newer one replaces it entirely.
type Status struct {
	Snell       map[string]snell.Status `json:"snell,omitempty"`
	CpuPct      float64                 `json:"cpuPct"`
	MemPct      float64                 `json:"memPct"`
	UptimeSecs  uint64                  `json:"uptimeSecs"`
	NetUp       uint64                  `json:"netUp"`
	NetDown     uint64                  `json:"netDown"`
	XrayVersion string                  `json:"xrayVersion"`
	XrayState   string                  `json:"xrayState"`
	XrayError   string                  `json:"xrayError,omitempty"`
	ConfigHash  string                  `json:"configHash"`
	// Online and ActiveInbounds are the emails and inbound tags active within
	// the agent's online grace window.
	Online         []string             `json:"online"`
	ActiveInbounds []string             `json:"activeInbounds"`
	IPs            map[string][]IPEntry `json:"ips,omitempty"`
}

// IPEntry is a client's live source address and when it was last seen (unix seconds).
type IPEntry struct {
	IP        string `json:"ip"`
	Timestamp int64  `json:"timestamp"`
}

// Traffic carries byte deltas. The panel applies (Instance, Seq) at most once:
// the agent resends an unacked report unchanged until the panel acks it.
type Traffic struct {
	Instance string    `json:"instance"`
	Seq      int64     `json:"seq"`
	Inbounds []Counter `json:"inbounds,omitempty"`
	Clients  []Counter `json:"clients,omitempty"`
}

// Counter is the traffic of one inbound tag or one client email.
type Counter struct {
	Name string `json:"name"`
	Up   int64  `json:"up"`
	Down int64  `json:"down"`
}

// Apply hands the agent the whole Xray config it must run.
type Apply struct {
	Config json.RawMessage `json:"config"`
	Hash   string          `json:"hash"`
	// RestartToDropUsers restarts the core when users leave the config, since the
	// core API only removes a credential and keeps its live sessions.
	RestartToDropUsers bool `json:"restartToDropUsers"`
}

type Result struct {
	OK         bool   `json:"ok"`
	Error      string `json:"error,omitempty"`
	ConfigHash string `json:"configHash,omitempty"`
	Restarted  bool   `json:"restarted,omitempty"`
}

type Ack struct {
	Instance string `json:"instance"`
	Seq      int64  `json:"seq"`
}

// Probe tests TCP reachability from the relay, not from the panel.
type Probe struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}
