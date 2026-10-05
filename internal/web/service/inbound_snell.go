package service

import (
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/snell"
	"github.com/mhsanaei/3x-ui/v3/internal/web/runtime"
)

func (s *InboundService) validateSnellInbound(ib *model.Inbound) error {
	if ib.Protocol != model.Snell {
		return nil
	}
	if ib.NodeID == nil {
		return fmt.Errorf("Snell must be deployed to an agent host")
	}
	n, err := (&NodeService{}).GetById(*ib.NodeID)
	if err != nil {
		return err
	}
	if !n.IsAgent() {
		return fmt.Errorf("Snell requires an agent host, not a remote Xray panel")
	}
	if _, err := snell.ParseSettings(ib.Settings); err != nil {
		return err
	}
	if ib.Port < 1 || ib.Port > 65535 {
		return fmt.Errorf("Snell requires a TCP/UDP port between 1 and 65535")
	}
	if ib.Listen != "" && net.ParseIP(ib.Listen) == nil {
		return fmt.Errorf("Snell listen address must be an IP address")
	}
	if ib.Total != 0 {
		return fmt.Errorf("Snell does not expose byte accounting; a traffic limit cannot be enforced")
	}
	if ib.ExpiryTime < 0 {
		return fmt.Errorf("Snell expiry must be an absolute timestamp or zero")
	}
	ib.StreamSettings = ""
	ib.Sniffing = `{"enabled":false}`
	return nil
}

func (s *AgentService) withSnellConfig(nodeID int, raw []byte) ([]byte, error) {
	rows, err := s.inboundService.GetNodeInbounds(nodeID)
	if err != nil {
		return nil, err
	}
	var instances []snell.Instance
	for _, ib := range rows {
		if ib.Protocol != model.Snell || !ib.Enable || (ib.ExpiryTime > 0 && ib.ExpiryTime <= time.Now().UnixMilli()) {
			continue
		}
		settings, err := snell.ParseSettings(ib.Settings)
		if err != nil {
			return nil, fmt.Errorf("Snell inbound %d: %w", ib.Id, err)
		}
		inst := snell.Instance{ID: ib.Id, Tag: ib.Tag, Listen: ib.Listen, Port: ib.Port, ExpiryTime: ib.ExpiryTime, Settings: settings}
		if err := inst.Validate(); err != nil {
			return nil, err
		}
		instances = append(instances, inst)
	}
	if len(instances) == 0 {
		return raw, nil
	}
	var top map[string]any
	if err := json.Unmarshal(raw, &top); err != nil {
		return nil, err
	}
	top[snell.ConfigKey] = instances
	return json.Marshal(top)
}

func (s *InboundService) annotateSnellStatus(inbounds []*model.Inbound) {
	hub := runtime.GetAgentHub()
	for _, ib := range inbounds {
		if ib.Protocol != model.Snell {
			continue
		}
		ib.RuntimeState = "pending"
		if !ib.Enable {
			ib.RuntimeState = "stopping"
		}
		if ib.NodeID == nil || hub == nil {
			ib.RuntimeState = "unknown"
			continue
		}
		state, ok := hub.Session(*ib.NodeID)
		if !ok || state.StatusAt.IsZero() || time.Since(state.StatusAt) > agentStatusStaleAfter {
			ib.RuntimeState = "unknown"
			continue
		}
		if st, ok := state.Status.Snell[ib.Tag]; ok {
			ib.RuntimeState = st.State
			ib.RuntimeError = st.Error
		} else if ib.ExpiryTime > 0 && ib.ExpiryTime <= time.Now().UnixMilli() {
			ib.RuntimeState = "expired"
		} else if !ib.Enable {
			ib.RuntimeState = "stopped"
		}
	}
}
