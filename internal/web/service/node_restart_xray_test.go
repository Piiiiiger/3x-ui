package service

import (
	"context"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

type restartCountingRuntime struct {
	fakeNodeRuntime
	restarts int
}

func (r *restartCountingRuntime) RestartXray(context.Context) error {
	r.restarts++
	return nil
}

func restartTestNode(t *testing.T, name string, enable bool) (*model.Node, *restartCountingRuntime) {
	t.Helper()
	node := &model.Node{Name: name, Kind: "agent", Address: "192.0.2.10", Enable: true, Status: "online"}
	if err := database.GetDB().Create(node).Error; err != nil {
		t.Fatalf("create node: %v", err)
	}
	if !enable {
		if err := database.GetDB().Model(node).Update("enable", false).Error; err != nil {
			t.Fatalf("disable node: %v", err)
		}
	}
	rt := &restartCountingRuntime{}
	useTestRuntimeManager(t).SetRuntimeOverride(node.Id, rt)
	return node, rt
}

// The hosts page restarts one host's Xray: an agent's core, not this panel's.
func TestRestartNodeXrayGoesThroughTheHostsRuntime(t *testing.T) {
	setupBulkDB(t)
	node, rt := restartTestNode(t, "edge", true)

	if err := (&NodeService{}).RestartXray(node.Id); err != nil {
		t.Fatalf("restart: %v", err)
	}
	if rt.restarts != 1 {
		t.Fatalf("host runtime restarted %d times, want 1", rt.restarts)
	}
}

func TestRestartNodeXrayRefusesADisabledOrUnknownHost(t *testing.T) {
	setupBulkDB(t)
	node, rt := restartTestNode(t, "parked", false)

	if err := (&NodeService{}).RestartXray(node.Id); err == nil {
		t.Fatal("restarting a disabled host succeeded")
	}
	if rt.restarts != 0 {
		t.Fatalf("a disabled host's runtime was restarted %d times", rt.restarts)
	}
	if err := (&NodeService{}).RestartXray(node.Id + 100); err == nil {
		t.Fatal("restarting a host that does not exist succeeded")
	}
}
