package job

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
	"github.com/mhsanaei/3x-ui/v3/internal/web/runtime"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

const (
	agentSyncTimeout = 30 * time.Second
	// agentNudgeSettle gathers the burst of runtime calls one edit makes, so the
	// agent gets one push for it instead of several.
	agentNudgeSettle = 300 * time.Millisecond
)

// AgentSyncJob keeps every connected agent on the config the panel holds for
// it, and disconnects agents whose node no longer allows them.
type AgentSyncJob struct {
	nodeService    service.NodeService
	agentService   service.AgentService
	inboundService service.InboundService
}

func NewAgentSyncJob() *AgentSyncJob {
	return &AgentSyncJob{}
}

func (j *AgentSyncJob) Run() {
	hub := runtime.GetAgentHub()
	if hub == nil {
		return
	}
	nodes, err := j.nodeService.GetAll()
	if err != nil {
		logger.Warning("agent sync: load nodes failed:", err)
		return
	}
	allowed := make(map[int]*model.Node, len(nodes))
	for _, n := range nodes {
		if n.IsAgent() && n.Enable {
			allowed[n.Id] = n
		}
	}
	var wg sync.WaitGroup
	for _, id := range hub.Connected() {
		n, ok := allowed[id]
		if !ok {
			hub.Disconnect(id)
			continue
		}
		wg.Add(1)
		common.GoRecover("agent-sync:"+n.Name, func() {
			defer wg.Done()
			j.syncNode(n)
		})
	}
	wg.Wait()
}

func (j *AgentSyncJob) syncNode(n *model.Node) {
	defer j.agentService.LockAgent(n.Id)()

	ctx, cancel := context.WithTimeout(context.Background(), agentSyncTimeout)
	defer cancel()
	if err := j.inboundService.DeliverNodeResets(ctx, n.Id, runtime.NewAgentRuntime(n)); err != nil {
		logger.Warning("agent sync: clear queued resets for", n.Name, "failed:", err)
	}
	// An agent that dropped since the tick listed it reconnects and is synced then.
	if err := j.agentService.SyncAgent(ctx, n); err != nil && !errors.Is(err, runtime.ErrAgentNotConnected) {
		logger.Warning("agent sync: push config to", n.Name, "failed, retrying next tick:", err)
	}
}

// WatchNudges syncs a nudged agent right away instead of on the next tick.
func (j *AgentSyncJob) WatchNudges(ctx context.Context) {
	hub := runtime.GetAgentHub()
	if hub == nil {
		return
	}
	for {
		var first int
		select {
		case <-ctx.Done():
			return
		case first = <-hub.Nudges():
		}
		pending := map[int]struct{}{first: {}}
		settle := time.NewTimer(agentNudgeSettle)
	collect:
		for {
			select {
			case <-ctx.Done():
				settle.Stop()
				return
			case id := <-hub.Nudges():
				pending[id] = struct{}{}
			case <-settle.C:
				break collect
			}
		}
		for id := range pending {
			n, err := j.nodeService.GetById(id)
			if err != nil || !n.IsAgent() || !n.Enable {
				continue
			}
			common.GoRecover("agent-nudge:"+n.Name, func() { j.syncNode(n) })
		}
	}
}
