package service

import (
	"context"
	"net/netip"
	"strings"

	"gorm.io/gorm"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
)

func (s *NodeService) CreateFromProbe(ctx context.Context, req *NodeMutationRequest) (*NodeView, error) {
	if err := req.validateCredentials(true); err != nil {
		return nil, err
	}
	if req.Kind != model.NodeKindAgent {
		return nil, common.NewError("a probe server must be added as an agent host")
	}
	n := req.toNode()
	n.ApiToken = ""
	if err := s.normalize(n); err != nil {
		return nil, err
	}
	serverID := strings.TrimSpace(req.ProbeServerId)
	snap, configured, err := (&ProbeService{}).Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	if !configured || snap.Stale || snap.Error != "" {
		return nil, common.NewError("the probe server list is unavailable; refresh and try again")
	}
	found := false
	for _, server := range snap.Servers {
		found = found || server.Id == serverID
	}
	if !found {
		return nil, common.NewError("the selected probe server no longer exists")
	}
	err = database.GetDB().Transaction(func(tx *gorm.DB) error {
		var taken int64
		if err := tx.Model(&model.ProbeLink{}).Where("server_id = ?", serverID).Count(&taken).Error; err != nil {
			return err
		}
		if taken != 0 {
			return common.NewError("the selected probe server already has a host")
		}
		if err := tx.Create(n).Error; err != nil {
			return err
		}
		return tx.Create(&model.ProbeLink{NodeId: n.Id, ServerId: serverID}).Error
	})
	if err != nil {
		return nil, err
	}
	return toNodeView(n), nil
}

// NoteAgentRemoteIP remembers the public address an agent last connected from;
// the IP limit exempts it, since a relay behind NAT dials out from there.
func (s *NodeService) NoteAgentRemoteIP(id int, address string) error {
	ip, err := netip.ParseAddr(address)
	if err != nil || !isPublicAddr(ip.Unmap()) {
		return nil
	}
	return database.GetDB().Model(&model.Node{}).Where("id = ?", id).
		Update("agent_remote_ip", ip.Unmap().String()).Error
}

// Only the first authenticated public connection fills an empty address;
// a reconnect never replaces an address the administrator has chosen.
func (s *NodeService) DiscoverAgentAddress(id int, address string) error {
	ip, err := netip.ParseAddr(address)
	if err != nil || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.Zone() != "" {
		return nil
	}
	return database.GetDB().Model(&model.Node{}).
		Where("id = ? AND kind = ? AND (address = '' OR address IS NULL)", id, model.NodeKindAgent).
		Update("address", ip.Unmap().String()).Error
}
