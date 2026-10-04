package service

import (
	"encoding/json"
	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/snell"
	"slices"
	"strings"
	"testing"
)

const snellTestSettings = `{"psk":"0123456789abcdef0123456789abcdef","version":5,"reuse":true,"clients":[]}`

func TestSnellAgentIsolationHashAndDisable(t *testing.T) {
	setupSettingTestDB(t)
	a := seedAgentNodeRow(t, "snell-host")
	b := seedAgentNodeRow(t, "other-host")
	i := seedNodeInbound(t, &a.Id, "snell-node", 26163, model.Snell, true, nil)
	if err := database.GetDB().Model(i).Update("settings", snellTestSettings).Error; err != nil {
		t.Fatal(err)
	}
	svc := &AgentService{}
	raw, hash, err := svc.AgentConfig(a.Id)
	if err != nil {
		t.Fatal(err)
	}
	clean, instances, err := snell.Split(raw)
	if err != nil || len(instances) != 1 || instances[0].ID != i.Id {
		t.Fatalf("missing sidecar: %v", err)
	}
	if strings.Contains(string(clean), "psk") || strings.Contains(string(clean), "snell-node") {
		t.Fatal("Snell leaked into Xray config")
	}
	other, _, err := svc.AgentConfig(b.Id)
	if err != nil {
		t.Fatal(err)
	}
	_, others, err := snell.Split(other)
	if err != nil || len(others) != 0 {
		t.Fatal("cross-host config leak")
	}
	changed := strings.Replace(snellTestSettings, "0123456789abcdef0123456789abcdef", "abcdef0123456789abcdef0123456789", 1)
	database.GetDB().Model(i).Update("settings", changed)
	_, hash2, err := svc.AgentConfig(a.Id)
	if err != nil || hash == hash2 {
		t.Fatal("PSK change did not change config hash")
	}
	database.GetDB().Model(i).Update("enable", false)
	raw, _, err = svc.AgentConfig(a.Id)
	if err != nil {
		t.Fatal(err)
	}
	_, instances, err = snell.Split(raw)
	if err != nil || len(instances) != 0 {
		t.Fatal("disabled Snell still served")
	}
}
func TestSnellValidationAndPlanAssignment(t *testing.T) {
	a, _, _ := setupPlanDB(t)
	n := seedAgentNodeRow(t, "snell-plan")
	i := &model.Inbound{NodeID: &n.Id, Protocol: model.Snell, Settings: snellTestSettings, Port: 26163, Listen: "0.0.0.0", Enable: true, Tag: "managed-snell"}
	svc := &InboundService{}
	if err := svc.validateSnellInbound(i); err != nil {
		t.Fatal(err)
	}
	invalid := *i
	invalid.NodeID = nil
	if svc.validateSnellInbound(&invalid) == nil {
		t.Fatal("local host accepted")
	}
	invalid = *i
	invalid.Total = 100
	if svc.validateSnellInbound(&invalid) == nil {
		t.Fatal("unenforceable quota accepted")
	}
	if err := database.GetDB().Create(i).Error; err != nil {
		t.Fatal(err)
	}
	createPlanClient(t, "snell-alice", []int{a}, 0)
	createPlanClient(t, "snell-bob", []int{a}, 0)
	if slices.Contains(planInboundIdsOf(t, "snell-alice"), i.Id) {
		t.Fatal("node assigned automatically")
	}
	plans := &PlanService{}
	plan, err := plans.Create(PlanInput{Name: "Snell plan", NodeKeys: []string{model.PlanNodeKey(i.Id, false)}})
	if err != nil {
		t.Fatalf("create selectable Snell plan: %v", err)
	}
	if _, err := plans.Assign(svc, []string{"snell-alice"}, plan.Id); err != nil {
		t.Fatalf("assign Snell to existing client: %v", err)
	}
	if got := planInboundIdsOf(t, "snell-alice"); !slices.Equal(got, []int{i.Id}) {
		t.Fatalf("assigned inbounds = %v, want only Snell %d", got, i.Id)
	}
	if slices.Contains(planInboundIdsOf(t, "snell-bob"), i.Id) {
		t.Fatal("Snell leaked to a client outside the selected plan")
	}
	var current model.Inbound
	database.GetDB().First(&current, i.Id)
	var settings map[string]any
	json.Unmarshal([]byte(current.Settings), &settings)
	if settings["psk"] != "0123456789abcdef0123456789abcdef" {
		t.Fatal("plan validation changed shared key")
	}
}

func TestSnellRegisterAndEditClientWithoutUUID(t *testing.T) {
	setupPlanDB(t)
	n := seedAgentNodeRow(t, "snell-register")
	ib := &model.Inbound{NodeID: &n.Id, Protocol: model.Snell, Settings: snellTestSettings, Port: 26163, Enable: true, Tag: "snell-register"}
	db := database.GetDB()
	if err := db.Create(ib).Error; err != nil {
		t.Fatal(err)
	}
	// Seed an existing plan so registration exercises credential validation independently.
	plan := &model.Plan{Name: "Existing Snell plan"}
	if err := db.Create(plan).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.PlanInbound{PlanId: plan.Id, InboundId: ib.Id}).Error; err != nil {
		t.Fatal(err)
	}
	code := mustCreateCodes(t, ActivationCodeInput{PlanId: plan.Id, Count: 1, Days: 30})[0]
	inbounds := &InboundService{}
	rec, _, err := (&ActivationCodeService{}).Register(inbounds, "snell-newbie", "secret-pass", code.Code)
	if err != nil {
		t.Fatalf("Snell-only registration: %v", err)
	}
	if rec.PlanId != plan.Id || !rec.Enable || rec.UUID != "" {
		t.Fatalf("registration did not preserve shared-key membership: plan %d enable %v UUID %q", rec.PlanId, rec.Enable, rec.UUID)
	}
	if got := planInboundIdsOf(t, rec.Email); !slices.Equal(got, []int{ib.Id}) {
		t.Fatalf("new client nodes = %v", got)
	}
	client := rec.ToClient()
	client.Comment = "edited Snell subscriber"
	if _, err := (&ClientService{}).Update(inbounds, rec.Id, *client, rec.LimitHwid); err != nil {
		t.Fatalf("edit UUID-less subscriber: %v", err)
	}
	entry := inboundClientEntry(t, ib.Id, rec.Email)
	if entry["comment"] != client.Comment {
		t.Fatalf("edit not persisted: %v", entry["comment"])
	}
	if _, _, err := (&ClientPortalService{}).Authenticate(rec.Email, "secret-pass"); err != nil {
		t.Fatalf("registered subscriber cannot log in: %v", err)
	}
	if used := codeRow(t, code.Id); used.UsedBy != rec.Email || used.UsedAt == 0 {
		t.Fatal("registration did not consume code")
	}
	var current model.Inbound
	if err := db.First(&current, ib.Id).Error; err != nil {
		t.Fatal(err)
	}
	var settings map[string]any
	if err := json.Unmarshal([]byte(current.Settings), &settings); err != nil {
		t.Fatal(err)
	}
	if settings["psk"] != "0123456789abcdef0123456789abcdef" {
		t.Fatal("membership changed shared key")
	}
}
