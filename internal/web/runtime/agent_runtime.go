package runtime

import (
	"context"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// AgentRuntime dispatches to a pigger-agent. The panel owns the agent's whole
// config, so a change only asks the sync loop to push that config again.
type AgentRuntime struct {
	nodeID int
	name   string
}

func NewAgentRuntime(n *model.Node) *AgentRuntime {
	return &AgentRuntime{nodeID: n.Id, name: n.Name}
}

func (a *AgentRuntime) Name() string { return "agent:" + a.name }

func (a *AgentRuntime) changed() error {
	if hub := GetAgentHub(); hub != nil {
		hub.Nudge(a.nodeID)
	}
	return nil
}

func (a *AgentRuntime) AddInbound(context.Context, *model.Inbound) error { return a.changed() }

func (a *AgentRuntime) DelInbound(context.Context, *model.Inbound) error { return a.changed() }

func (a *AgentRuntime) UpdateInbound(context.Context, *model.Inbound, *model.Inbound) error {
	return a.changed()
}

func (a *AgentRuntime) SetInboundSubSortIndex(context.Context, *model.Inbound, int) error {
	return a.changed()
}

func (a *AgentRuntime) AddUser(context.Context, *model.Inbound, map[string]any) error {
	return a.changed()
}

func (a *AgentRuntime) RemoveUser(context.Context, *model.Inbound, string) error { return a.changed() }

func (a *AgentRuntime) UpdateUser(context.Context, *model.Inbound, string, model.Client) error {
	return a.changed()
}

func (a *AgentRuntime) DeleteUser(context.Context, *model.Inbound, string) error { return a.changed() }

func (a *AgentRuntime) AddClient(context.Context, *model.Inbound, model.Client) error {
	return a.changed()
}

func (a *AgentRuntime) DeleteClient(context.Context, string) error { return a.changed() }

func (a *AgentRuntime) RestartXray(ctx context.Context) error {
	hub := GetAgentHub()
	if hub == nil {
		return ErrAgentNotConnected
	}
	return hub.Restart(ctx, a.nodeID)
}

// The reset methods succeed without a call: an agent keeps no counters, and the
// panel has already zeroed its own.
func (a *AgentRuntime) ResetClientTraffic(context.Context, *model.Inbound, string) error {
	return nil
}

func (a *AgentRuntime) ResetInboundTraffic(context.Context, *model.Inbound) error { return nil }

func (a *AgentRuntime) ResetAllTraffics(context.Context) error { return nil }
