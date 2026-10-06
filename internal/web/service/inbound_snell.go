package service

import (
	"encoding/json"
	"fmt"
	"net"
	"time"

	"gorm.io/gorm"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/snell"
	"github.com/mhsanaei/3x-ui/v3/internal/web/runtime"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
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
		return fmt.Errorf("a Snell inbound has no traffic limit of its own; with a single user, that user's quota applies")
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
		if suspended, err := snellSuspended(database.GetDB(), ib.Id); err != nil {
			return nil, err
		} else if suspended {
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
		} else if suspended, err := snellSuspended(database.GetDB(), ib.Id); err == nil && suspended {
			ib.RuntimeState = "suspended"
			continue
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

// snellSoleUser is the one user a Snell inbound serves, if it serves exactly one.
// Snell cannot tell users apart, so only then are its bytes someone's.
func snellSoleUser(db *gorm.DB, inboundID int) (model.ClientRecord, bool, error) {
	var rows []model.ClientRecord
	err := db.Table("clients").
		Joins("JOIN client_inbounds ON client_inbounds.client_id = clients.id").
		Where("client_inbounds.inbound_id = ?", inboundID).
		Limit(2).Find(&rows).Error
	if err != nil || len(rows) != 1 {
		return model.ClientRecord{}, false, err
	}
	return rows[0], true, nil
}

// snellSuspended says a Snell inbound's only user may not use it now: disabled,
// expired or out of traffic. A shared inbound never stops for one of its users.
func snellSuspended(db *gorm.DB, inboundID int) (bool, error) {
	user, ok, err := snellSoleUser(db, inboundID)
	if err != nil || !ok {
		return false, err
	}
	if !user.Enable {
		return true, nil
	}
	var ct xray.ClientTraffic
	if err := db.Where("email = ?", user.Email).Limit(1).Find(&ct).Error; err != nil {
		return false, err
	}
	return ct.Id != 0 && !ct.Enable, nil
}

// creditSnellUsers adds each single-user Snell inbound's traffic to that user's,
// merged into their own entry so the report counts it once.
func creditSnellUsers(db *gorm.DB, nodeID int, inbounds []*xray.Traffic, clients []*xray.ClientTraffic) ([]*xray.ClientTraffic, error) {
	byTag := make(map[string]*xray.Traffic, len(inbounds))
	tags := make([]string, 0, len(inbounds))
	for _, t := range inbounds {
		if t != nil && t.IsInbound && t.Up+t.Down > 0 {
			byTag[t.Tag] = t
			tags = append(tags, t.Tag)
		}
	}
	if len(tags) == 0 {
		return clients, nil
	}
	var snells []model.Inbound
	if err := db.Where("node_id = ? AND protocol = ? AND tag IN ?", nodeID, model.Snell, tags).Find(&snells).Error; err != nil {
		return clients, err
	}
	byEmail := make(map[string]*xray.ClientTraffic, len(clients))
	for _, c := range clients {
		if c != nil {
			byEmail[c.Email] = c
		}
	}
	for _, ib := range snells {
		user, ok, err := snellSoleUser(db, ib.Id)
		if err != nil {
			return clients, err
		}
		if !ok {
			continue
		}
		t := byTag[ib.Tag]
		if c := byEmail[user.Email]; c != nil {
			c.Up += t.Up
			c.Down += t.Down
			continue
		}
		c := &xray.ClientTraffic{Email: user.Email, Up: t.Up, Down: t.Down}
		clients = append(clients, c)
		byEmail[user.Email] = c
	}
	return clients, nil
}
