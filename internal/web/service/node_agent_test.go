package service

import (
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/agentproto"
	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// An agent dials in, so there is no port or token to check; an outbound bridge
// or an inbound selection would still steer the master, so they are dropped.
func TestNodeService_Normalize_AgentDropsPanelSyncSettings(t *testing.T) {
	n := &model.Node{
		Name:            "lazycat",
		Kind:            model.NodeKindAgent,
		Address:         "216.236.63.53",
		InboundSyncMode: "selected",
		InboundTags:     []string{"in-81-tcp"},
		OutboundTag:     "warp",
	}
	if err := (&NodeService{}).normalize(n); err != nil {
		t.Fatalf("normalize an agent without port or token: %v", err)
	}
	if n.OutboundTag != "" || n.InboundSyncMode != "all" || n.InboundTags != nil {
		t.Fatalf("panel sync fields kept: outbound=%q mode=%q tags=%v", n.OutboundTag, n.InboundSyncMode, n.InboundTags)
	}
	if n.Address != "216.236.63.53" {
		t.Fatalf("address = %q, want the public address kept", n.Address)
	}
}

func TestNodeService_Normalize_UnknownKindIsPanel(t *testing.T) {
	n := &model.Node{Name: "n", Kind: "bogus", Address: "example.com", Port: 0}
	if err := (&NodeService{}).normalize(n); err == nil {
		t.Fatal("a node of unknown kind must be checked as a panel and fail on port 0")
	}
	if n.Kind != model.NodeKindPanel {
		t.Fatalf("kind = %q, want %q", n.Kind, model.NodeKindPanel)
	}
}

func TestNodeService_CreateAgentWithoutToken(t *testing.T) {
	setupConflictDB(t)
	view, err := (&NodeService{}).CreateFromRequest(&NodeMutationRequest{
		Name: "lazycat", Kind: model.NodeKindAgent, Address: "216.236.63.53", Enable: true,
	})
	if err != nil {
		t.Fatalf("create agent node without a token: %v", err)
	}
	if view.Kind != model.NodeKindAgent {
		t.Fatalf("view kind = %q, want agent", view.Kind)
	}
	var stored model.Node
	if err := database.GetDB().First(&stored, view.Id).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Kind != model.NodeKindAgent {
		t.Fatalf("stored kind = %q, want agent", stored.Kind)
	}
}

func TestNodeService_UpdateAgentWithoutToken(t *testing.T) {
	setupConflictDB(t)
	s := &NodeService{}
	view, err := s.CreateFromRequest(&NodeMutationRequest{Name: "a", Kind: model.NodeKindAgent, Address: "1.2.3.4", Enable: true})
	if err != nil {
		t.Fatal(err)
	}
	err = s.UpdateFromRequest(view.Id, &NodeMutationRequest{Name: "a", Kind: model.NodeKindAgent, Address: "5.6.7.8", Enable: true})
	if err != nil {
		t.Fatalf("update an enabled agent without a token: %v", err)
	}
	got, err := s.GetById(view.Id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Address != "5.6.7.8" || !got.IsAgent() {
		t.Fatalf("after update: address=%q kind=%q", got.Address, got.Kind)
	}
}

// The six servers move to agents in place: the stored token must survive the
// switch so turning the node back into a panel needs no new credential.
func TestNodeService_ConvertPanelNodeToAgentAndBack(t *testing.T) {
	setupConflictDB(t)
	s := &NodeService{}
	token := "node-sync-token"
	view, err := s.CreateFromRequest(&NodeMutationRequest{
		Name: "lazycat", Address: "127.0.0.1", Port: 22605, Scheme: "http", BasePath: "/p8jt/",
		ApiToken: &token, AllowPrivateAddress: true, Enable: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateFromRequest(view.Id, &NodeMutationRequest{
		Name: "lazycat", Kind: model.NodeKindAgent, Address: "216.236.63.53", Enable: true,
	}); err != nil {
		t.Fatalf("convert to agent: %v", err)
	}
	got, err := s.GetById(view.Id)
	if err != nil {
		t.Fatal(err)
	}
	if !got.IsAgent() || got.Address != "216.236.63.53" {
		t.Fatalf("after conversion: kind=%q address=%q", got.Kind, got.Address)
	}
	if got.ApiToken != token {
		t.Fatalf("stored token = %q, want it kept for a rollback", got.ApiToken)
	}

	if err := s.UpdateFromRequest(view.Id, &NodeMutationRequest{
		Name: "lazycat", Kind: model.NodeKindPanel, Address: "127.0.0.1", Port: 22605, Scheme: "http",
		BasePath: "/p8jt/", AllowPrivateAddress: true, Enable: true,
	}); err != nil {
		t.Fatalf("convert back to a panel with the stored token: %v", err)
	}
	got, err = s.GetById(view.Id)
	if err != nil {
		t.Fatal(err)
	}
	if got.IsAgent() || got.Port != 22605 {
		t.Fatalf("after rollback: kind=%q port=%d", got.Kind, got.Port)
	}
}

func TestNodeService_AgentSecret(t *testing.T) {
	setupConflictDB(t)
	s := &NodeService{}
	agent, err := s.CreateFromRequest(&NodeMutationRequest{Name: "a", Kind: model.NodeKindAgent, Address: "1.2.3.4", Enable: true})
	if err != nil {
		t.Fatal(err)
	}

	first, err := s.MintAgentSecret(agent.Id)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if len(first) < 32 {
		t.Fatalf("secret %q is too short to be unguessable", first)
	}
	var stored model.Node
	if err := database.GetDB().First(&stored, agent.Id).Error; err != nil {
		t.Fatal(err)
	}
	if stored.AgentSecretHash == "" || strings.Contains(stored.AgentSecretHash, first) {
		t.Fatalf("the database must hold a hash of the secret, got %q", stored.AgentSecretHash)
	}

	n, err := s.AgentNodeBySecret(first)
	if err != nil || n.Id != agent.Id {
		t.Fatalf("AgentNodeBySecret(first) = %v, %v; want node %d", n, err, agent.Id)
	}

	second, err := s.MintAgentSecret(agent.Id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AgentNodeBySecret(first); err == nil {
		t.Fatal("a replaced secret must stop working")
	}
	if _, err := s.AgentNodeBySecret(second); err != nil {
		t.Fatalf("the new secret must work: %v", err)
	}

	if err := s.SetEnable(agent.Id, false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AgentNodeBySecret(second); err == nil {
		t.Fatal("a disabled agent node must not be able to connect")
	}
}

func TestNodeService_MintAgentSecretRejectsPanelNodes(t *testing.T) {
	setupConflictDB(t)
	panelNode := &model.Node{Name: "p", Address: "example.com", Port: 2053, ApiToken: "t", Enable: true}
	if err := database.GetDB().Create(panelNode).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := (&NodeService{}).MintAgentSecret(panelNode.Id); err == nil {
		t.Fatal("a panel node has no agent secret")
	}
}

// A new secret is minted when the old one may have leaked, so whoever holds the
// old one must lose the connection at once, not when the agent next reconnects.
func TestNodeService_MintAgentSecretDropsTheLiveSession(t *testing.T) {
	setupConflictDB(t)
	hub := useAgentHub(t)
	s := &NodeService{}
	agent, err := s.CreateFromRequest(&NodeMutationRequest{Name: "a", Kind: model.NodeKindAgent, Address: "1.2.3.4", Enable: true})
	if err != nil {
		t.Fatal(err)
	}
	n, err := s.GetById(agent.Id)
	if err != nil {
		t.Fatal(err)
	}
	live := connectFakeAgent(t, hub, n.Id, agentproto.Hello{AgentVersion: "v1"})

	if _, err := s.MintAgentSecret(agent.Id); err != nil {
		t.Fatal(err)
	}
	live.ExpectClosed("minting a new secret must drop the agent connected with the old one")
}
