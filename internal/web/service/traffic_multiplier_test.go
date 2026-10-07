package service

import (
	"errors"
	"math"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/mtproto"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

func setHostTrafficMultiplier(t *testing.T, nodeID int, multiplier float64) {
	t.Helper()
	if err := database.GetDB().Model(&model.Node{}).Where("id = ?", nodeID).
		Update("traffic_multiplier", multiplier).Error; err != nil {
		t.Fatalf("set the multiplier of host %d: %v", nodeID, err)
	}
}

func storedTrafficMultiplier(t *testing.T, nodeID int) float64 {
	t.Helper()
	var row struct{ TrafficMultiplier float64 }
	if err := database.GetDB().Raw("SELECT traffic_multiplier FROM nodes WHERE id = ?", nodeID).
		Scan(&row).Error; err != nil {
		t.Fatal(err)
	}
	return row.TrafficMultiplier
}

// A user's bytes on a server count toward their quota at the server's
// multiplier, while the server's own counters keep the bytes it really moved.
func TestAgentTrafficIsChargedAtItsServersMultiplier(t *testing.T) {
	setupSettingTestDB(t)
	a := seedAgentNodeRow(t, "edge-hk")
	setHostTrafficMultiplier(t, a.Id, 0.1)
	alice := model.Client{Email: "alice", ID: "11111111-1111-1111-1111-111111111111", Enable: true}
	ib := seedNodeInbound(t, &a.Id, "n1-in-81-tcp", 81, model.VLESS, true, []model.Client{alice})
	seedClientTrafficRow(t, ib.Id, "alice", 0)

	agentReport(t, a.Id, "i1", 1, "n1-in-81-tcp", "alice", 10_000, 20_000)

	if ct := clientUsage(t, "alice"); ct.Up != 1_000 || ct.Down != 2_000 {
		t.Fatalf("alice's usage = %d/%d, want 1000/2000: a 0.1 server counts a tenth", ct.Up, ct.Down)
	}
	if up, down := inboundUsage(t, "n1-in-81-tcp"); up != 10_000 || down != 20_000 {
		t.Fatalf("the node's traffic = %d/%d, want the real 10000/20000", up, down)
	}
	if got := nodeUsageOf(t, a.Id, "alice"); got != [2]int64{10_000, 20_000} {
		t.Fatalf("alice's traffic on the server = %v, want the real 10000/20000", got)
	}
}

// The panel's own server has a multiplier too. The abuse checks read the same
// poll after it is stored, so they must still see the bytes the core reported.
func TestLocalTrafficIsChargedAtThePanelsMultiplier(t *testing.T) {
	setupSettingTestDB(t)
	if err := (&SettingService{}).SetLocalTrafficMultiplier(0.5); err != nil {
		t.Fatal(err)
	}
	alice := model.Client{Email: "alice", ID: "11111111-1111-1111-1111-111111111111", Enable: true}
	ib := seedNodeInbound(t, nil, "in-443-tcp", 443, model.VLESS, true, []model.Client{alice})
	seedClientTrafficRow(t, ib.Id, "alice", 0)
	polled := []*xray.ClientTraffic{{Email: "alice", Up: 3_000, Down: 5_001}}

	if _, _, err := (&InboundService{}).AddTraffic(
		[]*xray.Traffic{{IsInbound: true, Tag: "in-443-tcp", Up: 3_000, Down: 5_001}}, polled); err != nil {
		t.Fatal(err)
	}

	if ct := clientUsage(t, "alice"); ct.Up != 1_500 || ct.Down != 2_501 {
		t.Fatalf("alice's usage = %d/%d, want 1500/2501 (half, rounded)", ct.Up, ct.Down)
	}
	if up, down := inboundUsage(t, "in-443-tcp"); up != 3_000 || down != 5_001 {
		t.Fatalf("the node's traffic = %d/%d, want the real 3000/5001", up, down)
	}
	if polled[0].Up != 3_000 || polled[0].Down != 5_001 {
		t.Fatalf("the poll handed on to the abuse checks became %d/%d, want the real 3000/5001",
			polled[0].Up, polled[0].Down)
	}
}

// On a free server bytes cost nothing, but a user moving them is still online:
// the local poll leaves such users to the traffic write to mark as seen.
func TestFreeServerStillMarksItsUsersOnline(t *testing.T) {
	setupSettingTestDB(t)
	if err := (&SettingService{}).SetLocalTrafficMultiplier(0); err != nil {
		t.Fatal(err)
	}
	alice := model.Client{Email: "alice", ID: "11111111-1111-1111-1111-111111111111", Enable: true}
	ib := seedNodeInbound(t, nil, "in-443-tcp", 443, model.VLESS, true, []model.Client{alice})
	seedClientTrafficRow(t, ib.Id, "alice", 0)

	if _, _, err := (&InboundService{}).AddTraffic(nil,
		[]*xray.ClientTraffic{{Email: "alice", Up: 3_000, Down: 5_000}}); err != nil {
		t.Fatal(err)
	}

	ct := clientUsage(t, "alice")
	if ct.Up != 0 || ct.Down != 0 {
		t.Fatalf("alice's usage = %d/%d, want 0/0 on a free server", ct.Up, ct.Down)
	}
	if ct.LastOnline == 0 {
		t.Fatal("alice moved bytes, so she must show as just seen")
	}
}

// A host saved without a multiplier counts bytes as they are; zero is a real
// value (a free host), and a save that leaves the multiplier out keeps it.
func TestAgentHostKeepsTheMultiplierItWasGiven(t *testing.T) {
	setupSettingTestDB(t)
	svc := &NodeService{}
	view, err := svc.CreateFromRequest(&NodeMutationRequest{Name: "edge-free", Kind: model.NodeKindAgent, Enable: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := storedTrafficMultiplier(t, view.Id); got != 1 {
		t.Fatalf("a new host's multiplier = %v, want 1", got)
	}

	free := 0.0
	if err := svc.UpdateFromRequest(view.Id, &NodeMutationRequest{
		Name: "edge-free", Kind: model.NodeKindAgent, Enable: true, TrafficMultiplier: &free,
	}); err != nil {
		t.Fatal(err)
	}
	if got := storedTrafficMultiplier(t, view.Id); got != 0 {
		t.Fatalf("after setting 0 the multiplier = %v, want 0", got)
	}
	if err := svc.UpdateFromRequest(view.Id, &NodeMutationRequest{
		Name: "edge-free", Kind: model.NodeKindAgent, Enable: true, Remark: "renamed",
	}); err != nil {
		t.Fatal(err)
	}
	if got := storedTrafficMultiplier(t, view.Id); got != 0 {
		t.Fatalf("a save without the multiplier changed it to %v, want 0 kept", got)
	}
	reread, err := svc.GetById(view.Id)
	if err != nil {
		t.Fatal(err)
	}
	if got := toNodeView(reread).TrafficMultiplier; got != 0 {
		t.Fatalf("the host list shows %v, want 0", got)
	}

	created, err := svc.CreateFromRequest(&NodeMutationRequest{
		Name: "edge-born-free", Kind: model.NodeKindAgent, Enable: true, TrafficMultiplier: &free,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := storedTrafficMultiplier(t, created.Id); got != 0 {
		t.Fatalf("a host created free has multiplier %v, want 0 rather than the column default", got)
	}
}

// A 3x-ui child panel limits its users on the bytes it counts itself, so a
// multiplier cannot apply there: it is refused, and switching to it resets it.
func TestTrafficMultiplierBounds(t *testing.T) {
	setupSettingTestDB(t)
	svc := &NodeService{}
	for _, bad := range []float64{-0.1, 100.01, math.NaN(), math.Inf(1)} {
		m := bad
		_, err := svc.CreateFromRequest(&NodeMutationRequest{Name: "edge-bad", Kind: model.NodeKindAgent, Enable: true, TrafficMultiplier: &m})
		if !errors.Is(err, errTrafficMultiplierRange) {
			t.Errorf("multiplier %v: err = %v, want errTrafficMultiplierRange", bad, err)
		}
		if err := (&SettingService{}).SetLocalTrafficMultiplier(bad); !errors.Is(err, errTrafficMultiplierRange) {
			t.Errorf("panel multiplier %v: err = %v, want errTrafficMultiplierRange", bad, err)
		}
	}
	if got, err := (&SettingService{}).GetLocalTrafficMultiplier(); err != nil || got != 1 {
		t.Fatalf("the panel's multiplier = %v, %v; want 1 after refused saves", got, err)
	}

	half := 0.5
	view, err := svc.CreateFromRequest(&NodeMutationRequest{Name: "edge", Kind: model.NodeKindAgent, Enable: true, TrafficMultiplier: &half})
	if err != nil {
		t.Fatal(err)
	}
	panel := NodeMutationRequest{Name: "edge", Kind: model.NodeKindPanel, Address: "203.0.113.9", Port: 2053, TlsVerifyMode: "mtls", Scheme: "https"}
	withHalf := panel
	withHalf.TrafficMultiplier = &half
	if err := svc.UpdateFromRequest(view.Id, &withHalf); !errors.Is(err, errPanelTrafficMultiplier) {
		t.Fatalf("a child panel with multiplier 0.5: err = %v, want errPanelTrafficMultiplier", err)
	}
	if err := svc.UpdateFromRequest(view.Id, &panel); err != nil {
		t.Fatal(err)
	}
	if got := storedTrafficMultiplier(t, view.Id); got != 1 {
		t.Fatalf("a host turned into a child panel keeps multiplier %v, want 1", got)
	}
}

// The mtg sidecar stops a secret on the bytes it moves itself; when the panel
// charges a multiple of them, its limit is the quota at that rate.
func TestMtprotoSidecarQuotaFollowsThePanelsMultiplier(t *testing.T) {
	setupConflictDB(t)
	svc := &InboundService{}
	seedInboundConflict(t, "mt-rated", "", 46011, model.MTProto, "",
		`{"clients":[{"email":"alice","secret":"`+mtprotoTestSecretA+`","enable":true,"totalGB":1000}]}`)
	ib := loadInboundByTag(t, "mt-rated")
	seedClientTraffic(t, ib.Id, "alice", true)

	for _, tc := range []struct {
		multiplier float64
		quota      int64
	}{{1, 1000}, {0.5, 2000}, {4, 250}, {0, 0}} {
		if err := (&SettingService{}).SetLocalTrafficMultiplier(tc.multiplier); err != nil {
			t.Fatal(err)
		}
		desired, err := svc.DesiredMtprotoInstances()
		if err != nil || len(desired) != 1 || len(desired[0].Secrets) != 1 {
			t.Fatalf("multiplier %v: desired = %+v, %v", tc.multiplier, desired, err)
		}
		if got := desired[0].Secrets[0].QuotaBytes; got != tc.quota {
			t.Errorf("multiplier %v: the reconcile job's quota = %d, want %d", tc.multiplier, got, tc.quota)
		}
		built, err := svc.buildInboundForLocalRuntime(database.GetDB(), ib)
		if err != nil {
			t.Fatal(err)
		}
		pushed, ok := mtproto.InstanceFromInbound(built)
		if !ok || len(pushed.Secrets) != 1 {
			t.Fatalf("multiplier %v: the push path made no instance", tc.multiplier)
		}
		if got := pushed.Secrets[0].QuotaBytes; got != tc.quota {
			t.Errorf("multiplier %v: an edit's push gives quota %d, want %d like the job", tc.multiplier, got, tc.quota)
		}
	}
}
