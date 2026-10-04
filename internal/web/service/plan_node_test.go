package service

import (
	"slices"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func TestPlanNodeVariantsPersistAndKeepRequiredRelay(t *testing.T) {
	relay, target, _ := setupPlanDB(t)
	db := database.GetDB()
	chain := model.ProxyChain{TargetInboundId: target, RelayInboundId: relay, Enabled: true}
	if err := db.Create(&chain).Error; err != nil {
		t.Fatal(err)
	}
	s := &PlanService{}
	key := model.PlanChainKey(target, chain.Id)
	plan, err := s.Create(PlanInput{Name: "variants", NodeKeys: []string{key}, ProxyGroups: []PlanProxyGroup{{Name: "AI", NodeKeys: []string{key}}}})
	if err != nil {
		t.Fatal(err)
	}
	_, ids, err := s.Get(plan.Id)
	if err != nil || !slices.Contains(ids, relay) || !slices.Contains(ids, target) {
		t.Fatalf("missing authenticated hop: %v %v", ids, err)
	}
	list, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || !slices.Equal(list[0].NodeKeys, []string{key}) {
		t.Fatalf("selection changed on reload: %+v", list)
	}
	if !slices.Equal(list[0].ProxyGroups[0].NodeKeys, []string{key}) {
		t.Fatalf("group lost variant: %+v", list[0].ProxyGroups)
	}
	createPlanClient(t, "variant-member", []int{target}, 0)
	if _, err := s.Assign(&InboundService{}, []string{"variant-member"}, plan.Id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update(&InboundService{}, plan.Id, PlanInput{Name: "variants", NodeKeys: []string{model.PlanNodeKey(target, false)}}, false); err != nil {
		t.Fatal(err)
	}
	if got := planInboundIdsOf(t, "variant-member"); !slices.Equal(got, []int{target}) {
		t.Fatalf("changing variant detached target or retained obsolete relay: %v", got)
	}
}

func TestPlanNodeVariantsRejectInvalidSelection(t *testing.T) {
	relay, target, _ := setupPlanDB(t)
	db := database.GetDB()
	chain := model.ProxyChain{TargetInboundId: target, RelayInboundId: relay, Enabled: true}
	if err := db.Create(&chain).Error; err != nil {
		t.Fatal(err)
	}
	s := &PlanService{}
	for _, input := range []PlanInput{
		{Name: "bad-key", NodeKeys: []string{"0:direct"}},
		{Name: "missing-chain", NodeKeys: []string{model.PlanNodeKey(relay, true)}},
		{Name: "unselected-variant", NodeKeys: []string{model.PlanNodeKey(target, false)}, ProxyGroups: []PlanProxyGroup{{Name: "AI", NodeKeys: []string{model.PlanChainKey(target, chain.Id)}}}},
	} {
		if _, err := s.Create(input); err == nil {
			t.Fatalf("accepted invalid selection: %+v", input)
		}
	}
}

func TestPlanNodeVariantDisplayNamesKeepStableKeys(t *testing.T) {
	relay, target, _ := setupPlanDB(t)
	db := database.GetDB()
	chain := model.ProxyChain{TargetInboundId: target, RelayInboundId: relay, Enabled: true, DirectName: "旧直连名", RelayName: "经香港到家宽"}
	if err := db.Create(&chain).Error; err != nil {
		t.Fatal(err)
	}
	options, err := (&PlanService{}).NodeOptions()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]PlanNodeOption{}
	for _, option := range options {
		got[option.Key] = option
	}
	if got[model.PlanNodeKey(target, false)].Label != "plan-b" || got[model.PlanChainKey(target, chain.Id)].Label != chain.RelayName || got[model.PlanChainKey(target, chain.Id)].RelayInboundId != relay {
		t.Fatalf("aliases changed identity: %+v", got)
	}
}

func TestPlanExplicitRelayCanBeAddedRemovedAndAddedAgain(t *testing.T) {
	relay, target, _ := setupPlanDB(t)
	chain := model.ProxyChain{TargetInboundId: target, RelayInboundId: relay, Enabled: true}
	if err := database.GetDB().Create(&chain).Error; err != nil {
		t.Fatal(err)
	}
	svc := &PlanService{}
	route := model.PlanChainKey(target, chain.Id)
	direct := model.PlanNodeKey(relay, false)
	plan, err := svc.Create(PlanInput{Name: "independent choices", NodeKeys: []string{route, direct}})
	if err != nil {
		t.Fatal(err)
	}
	createPlanClient(t, "relay-choice-user", []int{target}, 0)
	if _, err := svc.Assign(&InboundService{}, []string{"relay-choice-user"}, plan.Id); err != nil {
		t.Fatal(err)
	}
	for _, keys := range [][]string{{route}, {route, direct}, {route}, {}} {
		if _, err := svc.Update(&InboundService{}, plan.Id, PlanInput{Name: plan.Name, NodeKeys: keys}, false); err != nil {
			t.Fatal(err)
		}
		rows, err := svc.List()
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 || !slices.Equal(rows[0].NodeKeys, keys) {
			t.Fatalf("reload changed explicit selection %v: %+v", keys, rows)
		}
		grants := planInboundIdsOf(t, "relay-choice-user")
		if len(keys) > 0 && (!slices.Contains(grants, relay) || !slices.Contains(grants, target)) {
			t.Fatalf("chain lost authenticated hops: %v", grants)
		}
		if len(keys) == 0 && len(grants) > 0 {
			t.Fatalf("clear all retained grants: %v", grants)
		}
	}
}
