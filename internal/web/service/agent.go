package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/snell"

	"github.com/mhsanaei/3x-ui/v3/internal/agentproto"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/runtime"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

// agentStatusStaleAfter: agents report every few seconds, so an older status
// means the agent stopped reporting even if its socket still looks open.
var agentStatusStaleAfter = 20 * time.Second

// agentPushLocks keeps every caller that pushes to one agent from doing so at once.
var agentPushLocks sync.Map

// LockAgent holds nodeID's push lock until the returned func is called.
func (s *AgentService) LockAgent(nodeID int) (unlock func()) {
	lock, _ := agentPushLocks.LoadOrStore(nodeID, &sync.Mutex{})
	mu := lock.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// AgentService joins the panel's data to pigger-agents: the config each one
// runs, and the usage and state they report back.
type AgentService struct {
	xrayService    XrayService
	inboundService InboundService
	nodeService    NodeService
	settingService SettingService
}

// AgentConfig is the config agent node nodeID must run, exactly as sent, and
// its hash.
func (s *AgentService) AgentConfig(nodeID int) ([]byte, string, error) {
	cfg, err := s.xrayService.GetAgentXrayConfig(nodeID)
	if err != nil {
		return nil, "", err
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, "", err
	}
	raw, err = s.withSnellConfig(nodeID, raw)
	if err != nil {
		return nil, "", err
	}
	raw, err = s.withAbuseSettings(nodeID, raw)
	if err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(raw)
	return raw, hex.EncodeToString(sum[:]), nil
}

// SyncAgent pushes a connected agent its config when it runs anything else, then
// clears the node's dirty flag; without an agent it is runtime.ErrAgentNotConnected.
func (s *AgentService) SyncAgent(ctx context.Context, n *model.Node) error {
	hub := runtime.GetAgentHub()
	if hub == nil {
		return runtime.ErrAgentNotConnected
	}
	state, ok := hub.Session(n.Id)
	if !ok {
		return runtime.ErrAgentNotConnected
	}
	_, _, dirty, dirtyAt, err := s.nodeService.NodeSyncState(n.Id)
	if err != nil {
		return err
	}
	raw, hash, err := s.AgentConfig(n.Id)
	if err != nil {
		return err
	}
	if _, instances, splitErr := snell.Split(raw); splitErr != nil {
		return splitErr
	} else if len(instances) > 0 && !slices.Contains(state.Hello.Capabilities, snell.Capability) {
		return fmt.Errorf("agent on %s needs Snell support and a Snell v5 binary before this config can be applied", n.Name)
	}
	if hash != state.AppliedHash {
		dropUsers, err := s.settingService.GetRestartXrayOnClientDisable()
		if err != nil {
			return err
		}
		res, err := hub.Apply(ctx, n.Id, agentproto.Apply{Config: raw, Hash: hash, RestartToDropUsers: dropUsers})
		if err != nil {
			return err
		}
		if !res.OK {
			return fmt.Errorf("agent on %s refused its config: %s", n.Name, res.Error)
		}
	}
	if dirty {
		return s.nodeService.ClearNodeDirty(n.Id, dirtyAt)
	}
	return nil
}

// HandleTraffic stores one traffic report; a nil return is what acks it.
func (s *AgentService) HandleTraffic(nodeID int, t *agentproto.Traffic) error {
	inbounds := make([]*xray.Traffic, 0, len(t.Inbounds))
	for _, c := range t.Inbounds {
		inbounds = append(inbounds, &xray.Traffic{IsInbound: true, Tag: c.Name, Up: c.Up, Down: c.Down})
	}
	clients := make([]*xray.ClientTraffic, 0, len(t.Clients))
	for _, c := range t.Clients {
		clients = append(clients, &xray.ClientTraffic{Email: c.Name, Up: c.Up, Down: c.Down})
	}
	clients, err := creditSnellUsers(database.GetDB(), nodeID, inbounds, clients)
	if err != nil {
		return err
	}
	applied, err := s.inboundService.AddAgentTraffic(nodeID, t.Instance, t.Seq, inbounds, clients)
	if err != nil || !applied || len(t.Abuse) == 0 {
		return err
	}
	// The usage is counted already, so a failure here must not refuse the report.
	changed, err := (&AbuseService{}).HandleSignals(nodeID, t.Abuse, time.Now())
	if err != nil {
		logger.Warning("abuse: handling the report of node", nodeID, "failed:", err)
	}
	if changed {
		s.xrayService.SetToNeedRestart()
	}
	return nil
}

// HandleStatus shows an agent's online clients, active inbounds and client IPs
// under the key its inbounds are attributed to.
func (s *AgentService) HandleStatus(nodeID int, st *agentproto.Status) {
	node, err := s.nodeService.GetById(nodeID)
	if err != nil {
		return
	}
	key := effectiveNodeKey(node)
	s.inboundService.SetNodeOnlineTree(nodeID, map[string][]string{key: st.Online})
	if process := currentXrayProcess(); process != nil {
		process.SetNodeActiveInboundTree(nodeID, map[string][]string{key: st.ActiveInbounds})
	}
	if err := s.inboundService.BumpClientsLastOnline(st.Online); err != nil {
		logger.Warning("agent", node.Name, "bump last online failed:", err)
	}
	now := time.Now()
	observed := observationsFromAgentStatus(nodeID, runtime.AgentState{Status: *st, StatusAt: now}, now)
	if err := (&IpLimitService{}).recordLive(key, observed, now); err != nil {
		logger.Warning("agent", node.Name, "recording client ips failed:", err)
	}
}

// HandleGone drops what a disconnected agent reported as online.
func (s *AgentService) HandleGone(nodeID int) {
	s.inboundService.ClearNodeOnlineClients(nodeID)
}

// probeAgent turns an agent's last status into a heartbeat; an agent that is not
// connected, or stopped reporting, is offline.
func (s *NodeService) probeAgent(n *model.Node) (HeartbeatPatch, error) {
	patch := HeartbeatPatch{LastHeartbeat: time.Now().Unix()}
	hub := runtime.GetAgentHub()
	if hub == nil {
		patch.LastError = runtime.ErrAgentNotConnected.Error()
		return patch, runtime.ErrAgentNotConnected
	}
	state, ok := hub.Session(n.Id)
	if !ok {
		patch.LastError = runtime.ErrAgentNotConnected.Error()
		return patch, runtime.ErrAgentNotConnected
	}
	if state.StatusAt.IsZero() || time.Since(state.StatusAt) > agentStatusStaleAfter {
		patch.LastError = "agent stopped reporting its status"
		return patch, errors.New(patch.LastError)
	}
	st := state.Status
	patch.LatencyMs = state.LatencyMs
	patch.CpuPct = st.CpuPct
	patch.MemPct = st.MemPct
	patch.UptimeSecs = st.UptimeSecs
	patch.NetUp = st.NetUp
	patch.NetDown = st.NetDown
	patch.XrayVersion = st.XrayVersion
	patch.XrayState = st.XrayState
	patch.XrayError = st.XrayError
	patch.PanelVersion = state.Hello.AgentVersion
	return patch, nil
}
