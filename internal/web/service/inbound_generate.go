package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
	"github.com/mhsanaei/3x-ui/v3/internal/web/entity"
	"github.com/mhsanaei/3x-ui/v3/internal/web/runtime"

	"gorm.io/gorm"
)

// GenerateNodeRequest is a new node on one host: the inbound as the add route takes
// it, the plans whose members get it, and the port NAT exposes it on (0 for none).
type GenerateNodeRequest struct {
	Inbound    model.Inbound `json:"inbound"`
	PlanIds    []int         `json:"planIds" example:"[1,2]"`
	PublicPort int           `json:"publicPort" example:"20443" validate:"min=0,max=65535"`
}

// generateApplyTimeout bounds the wait for a host's verdict on a generated node.
const generateApplyTimeout = 30 * time.Second

// GenerateNode creates a node on the local panel or an agent host, gives it to the
// chosen plans' members and enables it only once the host runs it. A node the host
// refuses comes back disabled with the error; an earlier failure removes it.
func (s *InboundService) GenerateNode(req *GenerateNodeRequest) (*model.Inbound, error) {
	if strings.TrimSpace(req.Inbound.Remark) == "" {
		return nil, errors.New("a node needs a name: it is the name people see in their subscription")
	}
	host, err := generationHost(req.Inbound.NodeID)
	if err != nil {
		return nil, err
	}
	inbound := req.Inbound
	inbound.Enable = false
	if inbound.SubSortIndex, err = nextSubSortIndex(); err != nil {
		return nil, err
	}
	created, _, err := s.AddInbound(&inbound)
	if err != nil {
		return nil, err
	}
	if err := s.attachGeneratedNode(created, req); err != nil {
		if _, delErr := s.DelInbound(created.Id); delErr != nil {
			logger.Warning("generate node: removing", created.Tag, "after a failed step failed:", delErr)
		}
		return nil, err
	}
	if err := s.enableGeneratedNode(created, host); err != nil {
		return created, common.NewErrorf("the node was left disabled: %v", err)
	}
	created.Enable = true
	return created, nil
}

// generationHost loads the host a node is generated on; only the local panel (nil)
// and a connected agent can report whether they run it.
func generationHost(nodeID *int) (*model.Node, error) {
	if nodeID == nil {
		return nil, nil
	}
	var node model.Node
	if err := database.GetDB().First(&node, *nodeID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, common.NewErrorf("node not found: %d", *nodeID)
		}
		return nil, err
	}
	if !node.IsAgent() {
		return nil, errors.New("a node can only be generated on the local panel or an agent host")
	}
	if !node.Enable {
		return nil, common.NewErrorf("host %s is disabled", node.Name)
	}
	if !agentConnected(node.Id) {
		return nil, hostNotConnected(node.Name)
	}
	return &node, nil
}

func agentConnected(nodeID int) bool {
	hub := runtime.GetAgentHub()
	if hub == nil {
		return false
	}
	_, ok := hub.Session(nodeID)
	return ok
}

func hostNotConnected(name string) error {
	return common.NewErrorf("host %s is not connected; start its agent first", name)
}

// nextSubSortIndex places a new node after every node people already have, since
// subscriptions list by this index first.
func nextSubSortIndex() (int, error) {
	var top int
	err := database.GetDB().Model(&model.Inbound{}).Select("COALESCE(MAX(sub_sort_index), 0)").Scan(&top).Error
	return top + 1, err
}

// attachGeneratedNode adds the NAT entry and the plans' members while the node is
// still disabled; enabling it adds them live, so no restart is asked for here.
func (s *InboundService) attachGeneratedNode(created *model.Inbound, req *GenerateNodeRequest) error {
	if req.PublicPort > 0 && req.PublicPort != created.Port {
		entry := &entity.HostGroup{InboundIds: []int{created.Id}, Port: req.PublicPort, Remark: created.Remark, Security: "same"}
		if _, err := (&HostService{}).AddHostGroup(entry); err != nil {
			return err
		}
	}
	_, err := (&PlanService{}).AddInboundToPlans(s, created.Id, req.PlanIds)
	return err
}

func (s *InboundService) enableGeneratedNode(created *model.Inbound, host *model.Node) error {
	if host == nil {
		return s.enableOnLocalPanel(created)
	}
	agents := &AgentService{}
	defer agents.LockAgent(host.Id)()
	if _, err := s.SetInboundEnable(created.Id, true); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), generateApplyTimeout)
	defer cancel()
	if err := agents.SyncAgent(ctx, host); err != nil {
		s.disableGeneratedNode(created)
		return err
	}
	return nil
}

// enableOnLocalPanel refuses a port another program holds before the live add: a
// failed add would otherwise ask for an Xray restart that cannot bind it either.
func (s *InboundService) enableOnLocalPanel(created *model.Inbound) error {
	bits := inboundTransports(created.Protocol, created.StreamSettings, created.Settings)
	if !machineCanBind(created.Listen, created.Port, bits) {
		return common.NewErrorf("port %d is already in use on this machine", created.Port)
	}
	needRestart, err := s.SetInboundEnable(created.Id, true)
	if err != nil {
		return err
	}
	if needRestart {
		s.disableGeneratedNode(created)
		return errors.New("the panel's Xray did not take it live; see the panel log")
	}
	return nil
}

func (s *InboundService) disableGeneratedNode(created *model.Inbound) {
	if _, err := s.SetInboundEnable(created.Id, false); err != nil {
		logger.Warning("generate node: disabling refused node", created.Tag, "failed:", err)
	}
}
