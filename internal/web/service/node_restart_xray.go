package service

import (
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
	"github.com/mhsanaei/3x-ui/v3/internal/web/runtime"
)

// RestartXray restarts one host's Xray through its runtime: an agent restarts its
// embedded core, a panel node its own process.
func (s *NodeService) RestartXray(id int) error {
	node, err := s.GetById(id)
	if err != nil {
		return err
	}
	if !node.Enable {
		return common.NewError("host is disabled:", node.Name)
	}
	mgr := runtime.GetManager()
	if mgr == nil {
		return common.NewError("runtime manager is not ready")
	}
	rt, err := mgr.RuntimeFor(&id)
	if err != nil {
		return err
	}
	ctx, cancel := nodePushContext()
	defer cancel()
	return rt.RestartXray(ctx)
}
