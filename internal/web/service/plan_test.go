package service

import (
	"encoding/json"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

const (
	planGiB   = int64(1) << 30
	planDayMs = int64(24 * time.Hour / time.Millisecond)
)

// setupPlanDB seeds three local VLESS inbounds and returns their ids.
func setupPlanDB(t *testing.T) (a, b, c int) {
	t.Helper()
	setupConflictDB(t)
	startSerializedWriter(t)
	ids := make([]int, 0, 3)
	for i, tag := range []string{"plan-a", "plan-b", "plan-c"} {
		seedInboundConflict(t, tag, "0.0.0.0", 47101+i, model.VLESS, `{"network":"tcp"}`, `{"clients":[]}`)
		var ib model.Inbound
		if err := database.GetDB().Where("tag = ?", tag).First(&ib).Error; err != nil {
			t.Fatalf("read seeded inbound %s: %v", tag, err)
		}
		ids = append(ids, ib.Id)
	}
	return ids[0], ids[1], ids[2]
}

func createPlanClient(t *testing.T, email string, inboundIds []int, expiry int64) {
	t.Helper()
	if _, err := (&ClientService{}).Create(&InboundService{}, &ClientCreatePayload{
		Client: model.Client{
			Email: email, ID: uuid.NewString(), SubID: "sub-" + uuid.NewString()[:8],
			Enable: true, ExpiryTime: expiry,
		},
		InboundIds: inboundIds,
	}); err != nil {
		t.Fatalf("create client %s: %v", email, err)
	}
}

func planRecord(t *testing.T, email string) *model.ClientRecord {
	t.Helper()
	rec, err := (&ClientService{}).GetRecordByEmail(nil, email)
	if err != nil {
		t.Fatalf("read client %s: %v", email, err)
	}
	return rec
}

func planInboundIdsOf(t *testing.T, email string) []int {
	t.Helper()
	ids, err := (&ClientService{}).GetInboundIdsForRecord(planRecord(t, email).Id)
	if err != nil {
		t.Fatalf("inbound ids of %s: %v", email, err)
	}
	slices.Sort(ids)
	return ids
}

// inboundClientEntry returns the client's entry in an inbound's settings JSON:
// what Xray and the subscription actually see, not just the clients row.
func inboundClientEntry(t *testing.T, inboundId int, email string) map[string]any {
	t.Helper()
	var ib model.Inbound
	if err := database.GetDB().First(&ib, inboundId).Error; err != nil {
		t.Fatalf("read inbound %d: %v", inboundId, err)
	}
	var settings struct {
		Clients []map[string]any `json:"clients"`
	}
	if err := json.Unmarshal([]byte(ib.Settings), &settings); err != nil {
		t.Fatalf("parse inbound %d settings: %v", inboundId, err)
	}
	for _, c := range settings.Clients {
		if c["email"] == email {
			return c
		}
	}
	t.Fatalf("client %s not in inbound %d settings", email, inboundId)
	return nil
}

func withinMs(got, want, slack int64) bool { return got >= want-slack && got <= want+slack }

func TestAssignPlanStampsLimitsAndGrantsExactlyThePlanInbounds(t *testing.T) {
	a, b, c := setupPlanDB(t)
	createPlanClient(t, "alice@plan", []int{a, c}, 0)
	s := &PlanService{}
	plan, err := s.Create(PlanInput{
		Name: "HK 100G", TotalGB: 100 * planGiB, DurationDays: 30,
		TrafficReset: "monthly", TrafficResetDay: 5, LimitIP: 2, InboundIds: []int{a, b},
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	start := time.Now().UnixMilli()
	if _, err := s.Assign(&InboundService{}, []string{"alice@plan"}, plan.Id, PlanStartNow, true); err != nil {
		t.Fatalf("assign: %v", err)
	}
	end := time.Now().UnixMilli()

	rec := planRecord(t, "alice@plan")
	if rec.PlanId != plan.Id || rec.TotalGB != 100*planGiB || rec.LimitIP != 2 ||
		rec.TrafficReset != "monthly" || rec.TrafficResetDay != 5 {
		t.Fatalf("record = plan %d quota %d limitIp %d reset %s/%d, want the plan's values",
			rec.PlanId, rec.TotalGB, rec.LimitIP, rec.TrafficReset, rec.TrafficResetDay)
	}
	if rec.ExpiryTime < start+30*planDayMs || rec.ExpiryTime > end+30*planDayMs {
		t.Fatalf("expiry %d not 30 days from the assignment", rec.ExpiryTime)
	}
	if got := planInboundIdsOf(t, "alice@plan"); !slices.Equal(got, []int{a, b}) {
		t.Fatalf("inbounds = %v, want exactly the plan's %v (c detached, b attached)", got, []int{a, b})
	}
	entry := inboundClientEntry(t, b, "alice@plan")
	if entry["totalGB"] != float64(100*planGiB) || entry["limitIp"] != float64(2) ||
		entry["expiryTime"] != float64(rec.ExpiryTime) {
		t.Fatalf("inbound b entry = %v, want the plan's quota, IP limit and expiry", entry)
	}
}

func TestAssignPlanFirstUseStartsTheClockAtFirstConnection(t *testing.T) {
	a, _, _ := setupPlanDB(t)
	createPlanClient(t, "first@plan", []int{a}, 0)
	s := &PlanService{}
	plan, err := s.Create(PlanInput{Name: "30 days", DurationDays: 30, InboundIds: []int{a}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if _, err := s.Assign(&InboundService{}, []string{"first@plan"}, plan.Id, PlanStartFirstUse, true); err != nil {
		t.Fatalf("assign: %v", err)
	}
	// A negative expiry is 3x-ui's "this long after the first connection".
	if got := planRecord(t, "first@plan").ExpiryTime; got != -30*planDayMs {
		t.Fatalf("expiry = %d, want %d", got, -30*planDayMs)
	}
}

func TestAssignPlanKeepLeavesTheClientsExpiry(t *testing.T) {
	a, _, _ := setupPlanDB(t)
	expiry := time.Now().Add(72 * time.Hour).UnixMilli()
	createPlanClient(t, "keep@plan", []int{a}, expiry)
	s := &PlanService{}
	plan, err := s.Create(PlanInput{Name: "Quota only", TotalGB: 10 * planGiB, DurationDays: 30, InboundIds: []int{a}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if _, err := s.Assign(&InboundService{}, []string{"keep@plan"}, plan.Id, PlanStartKeep, false); err != nil {
		t.Fatalf("assign: %v", err)
	}
	rec := planRecord(t, "keep@plan")
	if rec.ExpiryTime != expiry || rec.TotalGB != 10*planGiB {
		t.Fatalf("expiry %d quota %d, want expiry kept at %d and the plan quota", rec.ExpiryTime, rec.TotalGB, expiry)
	}
}

func TestRenewPlanExtendsFromTheLaterOfNowAndTheCurrentExpiry(t *testing.T) {
	a, _, _ := setupPlanDB(t)
	future := time.Now().Add(10 * 24 * time.Hour).UnixMilli()
	past := time.Now().Add(-5 * 24 * time.Hour).UnixMilli()
	createPlanClient(t, "future@plan", []int{a}, future)
	createPlanClient(t, "lapsed@plan", []int{a}, past)
	s := &PlanService{}
	plan, err := s.Create(PlanInput{Name: "Monthly", TotalGB: 50 * planGiB, DurationDays: 30, InboundIds: []int{a}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	emails := []string{"future@plan", "lapsed@plan"}
	if _, err := s.Assign(&InboundService{}, emails, plan.Id, PlanStartKeep, false); err != nil {
		t.Fatalf("assign: %v", err)
	}
	if err := database.GetDB().Model(&xray.ClientTraffic{}).Where("email IN ?", emails).
		Updates(map[string]any{"up": 7 * planGiB, "down": 9 * planGiB}).Error; err != nil {
		t.Fatalf("seed usage: %v", err)
	}

	if _, _, err := (&ClientService{}).SetClientEnableByEmail(&InboundService{}, "lapsed@plan", false); err != nil {
		t.Fatalf("disable the lapsed client: %v", err)
	}

	now := time.Now().UnixMilli()
	if _, err := s.Renew(&InboundService{}, emails); err != nil {
		t.Fatalf("renew: %v", err)
	}

	if got := planRecord(t, "future@plan").ExpiryTime; got != future+30*planDayMs {
		t.Fatalf("future client expiry = %d, want its old expiry plus 30 days (%d)", got, future+30*planDayMs)
	}
	if got := planRecord(t, "lapsed@plan").ExpiryTime; !withinMs(got, now+30*planDayMs, 60_000) {
		t.Fatalf("lapsed client expiry = %d, want 30 days from now (~%d)", got, now+30*planDayMs)
	}
	if !planRecord(t, "lapsed@plan").Enable {
		t.Fatal("a disabled client is still disabled after renewing it")
	}
	var used []xray.ClientTraffic
	if err := database.GetDB().Where("email IN ?", emails).Find(&used).Error; err != nil {
		t.Fatalf("read usage: %v", err)
	}
	for _, u := range used {
		if u.Up != 0 || u.Down != 0 {
			t.Fatalf("%s usage = %d/%d after renew, want zeroed", u.Email, u.Up, u.Down)
		}
	}
}

func TestRenewRefusesAClientWithoutAPlan(t *testing.T) {
	a, _, _ := setupPlanDB(t)
	createPlanClient(t, "noplan@plan", []int{a}, 0)
	if _, err := (&PlanService{}).Renew(&InboundService{}, []string{"noplan@plan"}); err == nil {
		t.Fatal("renewing a client with no plan succeeded; it has no duration to renew by")
	}
}

func TestDeletePlanIsRefusedWhileClientsUseIt(t *testing.T) {
	a, b, _ := setupPlanDB(t)
	createPlanClient(t, "member@plan", []int{a}, 0)
	s := &PlanService{}
	plan, err := s.Create(PlanInput{Name: "Busy", TotalGB: 5 * planGiB, InboundIds: []int{a, b}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if _, err := s.Assign(&InboundService{}, []string{"member@plan"}, plan.Id, PlanStartKeep, false); err != nil {
		t.Fatalf("assign: %v", err)
	}
	if err := s.Delete(plan.Id); err == nil {
		t.Fatal("deleted a plan that a client still uses")
	}

	if err := s.Unassign([]string{"member@plan"}); err != nil {
		t.Fatalf("unassign: %v", err)
	}
	rec := planRecord(t, "member@plan")
	if rec.PlanId != 0 || rec.TotalGB != 5*planGiB {
		t.Fatalf("after unassign plan=%d quota=%d, want no plan and the quota left as it was", rec.PlanId, rec.TotalGB)
	}
	if err := s.Delete(plan.Id); err != nil {
		t.Fatalf("delete an unused plan: %v", err)
	}
	var left int64
	database.GetDB().Model(&model.PlanInbound{}).Where("plan_id = ?", plan.Id).Count(&left)
	if left != 0 {
		t.Fatalf("%d plan_inbounds rows survived the plan", left)
	}
}

func TestUpdatePlanAppliedToMembersKeepsTheirExpiry(t *testing.T) {
	a, b, _ := setupPlanDB(t)
	expiry := time.Now().Add(20 * 24 * time.Hour).UnixMilli()
	createPlanClient(t, "member@upd", []int{a}, expiry)
	createPlanClient(t, "outsider@upd", []int{a}, 0)
	s := &PlanService{}
	plan, err := s.Create(PlanInput{Name: "Basic", TotalGB: 100 * planGiB, DurationDays: 30, InboundIds: []int{a}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if _, err := s.Assign(&InboundService{}, []string{"member@upd"}, plan.Id, PlanStartKeep, false); err != nil {
		t.Fatalf("assign: %v", err)
	}

	in := PlanInput{Name: "Basic", TotalGB: 200 * planGiB, DurationDays: 30, InboundIds: []int{b}}
	if _, err := s.Update(&InboundService{}, plan.Id, in, true); err != nil {
		t.Fatalf("update plan: %v", err)
	}

	member := planRecord(t, "member@upd")
	if member.TotalGB != 200*planGiB || member.ExpiryTime != expiry {
		t.Fatalf("member quota %d expiry %d, want the new quota and the old expiry %d", member.TotalGB, member.ExpiryTime, expiry)
	}
	if got := planInboundIdsOf(t, "member@upd"); !slices.Equal(got, []int{b}) {
		t.Fatalf("member inbounds = %v, want the plan's new set [%d]", got, b)
	}
	if outsider := planRecord(t, "outsider@upd"); outsider.TotalGB != 0 {
		t.Fatalf("a client outside the plan got quota %d", outsider.TotalGB)
	}
}

func TestCreatePlanRejectsInvalidInput(t *testing.T) {
	a, _, _ := setupPlanDB(t)
	s := &PlanService{}
	if _, err := s.Create(PlanInput{Name: "Taken", InboundIds: []int{a}}); err != nil {
		t.Fatalf("seed plan: %v", err)
	}
	cases := map[string]PlanInput{
		"blank name":        {Name: "  "},
		"duplicate name":    {Name: "Taken"},
		"negative quota":    {Name: "Q", TotalGB: -1},
		"negative duration": {Name: "D", DurationDays: -1},
		"negative IP limit": {Name: "I", LimitIP: -1},
		"unknown reset":     {Name: "R", TrafficReset: "yearly"},
		"missing inbound":   {Name: "M", InboundIds: []int{a, 99999}},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := s.Create(in); err == nil {
				t.Fatalf("Create(%+v) succeeded", in)
			}
		})
	}
}

func TestListPlansReportsInboundsAndMembers(t *testing.T) {
	a, b, _ := setupPlanDB(t)
	createPlanClient(t, "one@list", []int{a}, 0)
	createPlanClient(t, "two@list", []int{a}, 0)
	s := &PlanService{}
	plan, err := s.Create(PlanInput{Name: "Shared", InboundIds: []int{b, a}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if _, err := s.Assign(&InboundService{}, []string{"one@list", "two@list"}, plan.Id, PlanStartKeep, false); err != nil {
		t.Fatalf("assign: %v", err)
	}
	plans, err := s.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(plans) != 1 || plans[0].MemberCount != 2 || !slices.Equal(plans[0].InboundIds, []int{a, b}) {
		t.Fatalf("list = %+v, want one plan with 2 members and inbounds %v", plans, []int{a, b})
	}
}

func TestDeletingAnInboundDropsItFromPlans(t *testing.T) {
	a, b, _ := setupPlanDB(t)
	s := &PlanService{}
	if _, err := s.Create(PlanInput{Name: "Two servers", InboundIds: []int{a, b}}); err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if _, err := (&InboundService{}).DelInbound(a); err != nil {
		t.Fatalf("delete inbound: %v", err)
	}
	plans, err := s.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(plans) != 1 || !slices.Equal(plans[0].InboundIds, []int{b}) {
		t.Fatalf("plan inbounds = %+v, want only [%d] after the other inbound was deleted", plans, b)
	}
}

func TestListPagedFiltersByPlanAndReportsIt(t *testing.T) {
	svc, inboundSvc, settingSvc := setupPagingServices(t)
	seedPagingClients(t)
	plan, err := (&PlanService{}).Create(PlanInput{Name: "VIP"})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if err := database.GetDB().Model(&model.ClientRecord{}).Where("email IN ?", []string{"bravo@x", "india@x"}).
		UpdateColumn("plan_id", plan.Id).Error; err != nil {
		t.Fatalf("put clients on the plan: %v", err)
	}

	onPlan, err := svc.ListPaged(inboundSvc, settingSvc, ClientPageParams{PageSize: 50, Plan: strconv.Itoa(plan.Id)})
	if err != nil {
		t.Fatalf("list by plan: %v", err)
	}
	var got []string
	for _, c := range onPlan.Items {
		got = append(got, c.Email)
		if c.PlanId != plan.Id {
			t.Fatalf("row %s planId = %d, want %d", c.Email, c.PlanId, plan.Id)
		}
	}
	if !slices.Equal(got, []string{"bravo@x", "india@x"}) {
		t.Fatalf("plan filter = %v, want [bravo@x india@x]", got)
	}

	without, err := svc.ListPaged(inboundSvc, settingSvc, ClientPageParams{PageSize: 50, Plan: "0"})
	if err != nil {
		t.Fatalf("list without plan: %v", err)
	}
	if without.Filtered != 10 || slices.ContainsFunc(without.Items, func(c ClientSlim) bool { return c.PlanId != 0 }) {
		t.Fatalf("plan=0 returned %d rows, want the 10 clients without a plan", without.Filtered)
	}
}
