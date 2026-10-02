package service

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
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

// giveQuota sets a client's own quota, as the admin does on the users page.
func giveQuota(t *testing.T, email string, bytes int64) {
	t.Helper()
	rec := planRecord(t, email)
	client := rec.ToClient()
	client.TotalGB = bytes
	if _, err := (&ClientService{}).Update(&InboundService{}, rec.Id, *client, rec.LimitHwid); err != nil {
		t.Fatalf("give %s a quota: %v", email, err)
	}
}

// A plan grants its servers and its IP limit; quota, expiry and reset stay the user's own.
func TestAssignPlanGrantsItsInboundsAndIPLimitAndKeepsTheUsersOwnLimits(t *testing.T) {
	a, b, c := setupPlanDB(t)
	expiry := time.Now().Add(20 * 24 * time.Hour).UnixMilli()
	createPlanClient(t, "alice@plan", []int{a, c}, expiry)
	giveQuota(t, "alice@plan", 100*planGiB)
	s := &PlanService{}
	plan, err := s.Create(PlanInput{Name: "HK", LimitIP: 2, InboundIds: []int{a, b}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	if _, err := s.Assign(&InboundService{}, []string{"alice@plan"}, plan.Id); err != nil {
		t.Fatalf("assign: %v", err)
	}

	rec := planRecord(t, "alice@plan")
	if rec.PlanId != plan.Id || rec.LimitIP != 2 || rec.TotalGB != 100*planGiB || rec.ExpiryTime != expiry {
		t.Fatalf("record = plan %d limitIp %d quota %d expiry %d, want the plan's IP limit and her own quota and expiry",
			rec.PlanId, rec.LimitIP, rec.TotalGB, rec.ExpiryTime)
	}
	if got := planInboundIdsOf(t, "alice@plan"); !slices.Equal(got, []int{a, b}) {
		t.Fatalf("inbounds = %v, want exactly the plan's %v (c detached, b attached)", got, []int{a, b})
	}
	entry := inboundClientEntry(t, b, "alice@plan")
	if entry["totalGB"] != float64(100*planGiB) || entry["limitIp"] != float64(2) || entry["expiryTime"] != float64(expiry) {
		t.Fatalf("inbound b entry = %v, want her quota and expiry with the plan's IP limit", entry)
	}
}

func TestDeletePlanIsRefusedWhileClientsUseIt(t *testing.T) {
	a, b, _ := setupPlanDB(t)
	createPlanClient(t, "member@plan", []int{a}, 0)
	s := &PlanService{}
	plan, err := s.Create(PlanInput{Name: "Busy", LimitIP: 2, InboundIds: []int{a, b}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if _, err := s.Assign(&InboundService{}, []string{"member@plan"}, plan.Id); err != nil {
		t.Fatalf("assign: %v", err)
	}
	if err := s.Delete(plan.Id); err == nil {
		t.Fatal("deleted a plan that a client still uses")
	}

	if err := s.Unassign([]string{"member@plan"}); err != nil {
		t.Fatalf("unassign: %v", err)
	}
	rec := planRecord(t, "member@plan")
	if rec.PlanId != 0 || rec.LimitIP != 2 {
		t.Fatalf("after unassign plan=%d limitIp=%d, want no plan and the IP limit left as it was", rec.PlanId, rec.LimitIP)
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

// attachByHand gives a client an inbound outside its plan, as the admin does on the clients page.
func attachByHand(t *testing.T, email string, inboundId int) {
	t.Helper()
	if _, err := (&ClientService{}).AttachByEmail(&InboundService{}, email, []int{inboundId}); err != nil {
		t.Fatalf("attach %s to inbound %d: %v", email, inboundId, err)
	}
}

// Unticked, an inbound added to a plan never reached its members; ticking the
// re-apply box was the only way, and that also reset their limits.
func TestUpdatePlanAttachesAnAddedInboundWithoutReapplyingLimits(t *testing.T) {
	a, b, _ := setupPlanDB(t)
	createPlanClient(t, "member@add", []int{a}, 0)
	createPlanClient(t, "outsider@add", []int{a}, 0)
	giveQuota(t, "member@add", 100*planGiB)
	s := &PlanService{}
	plan, err := s.Create(PlanInput{Name: "HK", LimitIP: 2, InboundIds: []int{a}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if _, err := s.Assign(&InboundService{}, []string{"member@add"}, plan.Id); err != nil {
		t.Fatalf("assign: %v", err)
	}

	in := PlanInput{Name: "HK", LimitIP: 3, InboundIds: []int{a, b}}
	needRestart, err := s.Update(&InboundService{}, plan.Id, in, false)
	if err != nil {
		t.Fatalf("update plan: %v", err)
	}

	if got := planInboundIdsOf(t, "member@add"); !slices.Equal(got, []int{a, b}) {
		t.Fatalf("member inbounds = %v, want %v with the added inbound", got, []int{a, b})
	}
	member := planRecord(t, "member@add")
	if member.TotalGB != 100*planGiB || member.LimitIP != 2 {
		t.Fatalf("member quota %d limitIp %d, want its own 100 GiB and 2 kept", member.TotalGB, member.LimitIP)
	}
	if entry := inboundClientEntry(t, b, "member@add"); entry["totalGB"] != float64(100*planGiB) || entry["limitIp"] != float64(2) {
		t.Fatalf("added inbound entry = %v, want the member's own quota and IP limit", entry)
	}
	if got := planInboundIdsOf(t, "outsider@add"); !slices.Equal(got, []int{a}) {
		t.Fatalf("a client outside the plan now has inbounds %v, want only [%d]", got, a)
	}
	// No Xray runs here, so a user added to a local inbound waits for a restart.
	if !needRestart {
		t.Fatal("Update dropped the restart the member's new inbound needs")
	}
}

// detachByHand takes an inbound off one client, as the admin does on the clients page.
func detachByHand(t *testing.T, email string, inboundId int) {
	t.Helper()
	if _, err := (&ClientService{}).DetachByEmailMany(&InboundService{}, email, []int{inboundId}); err != nil {
		t.Fatalf("detach %s from inbound %d: %v", email, inboundId, err)
	}
}

// Attaching all the plan's inbounds on each save, not only the ones it gained, gave
// back an inbound an admin had taken off one member, on any later save of the plan.
func TestUpdatePlanAttachesOnlyTheInboundsItGained(t *testing.T) {
	a, b, c := setupPlanDB(t)
	createPlanClient(t, "member@gain", []int{a}, 0)
	s := &PlanService{}
	plan, err := s.Create(PlanInput{Name: "Gain", InboundIds: []int{a, c}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if _, err := s.Assign(&InboundService{}, []string{"member@gain"}, plan.Id); err != nil {
		t.Fatalf("assign: %v", err)
	}
	detachByHand(t, "member@gain", c)

	if _, err := s.Update(&InboundService{}, plan.Id, PlanInput{Name: "Gain", InboundIds: []int{a, b, c}}, false); err != nil {
		t.Fatalf("update plan: %v", err)
	}

	if got := planInboundIdsOf(t, "member@gain"); !slices.Equal(got, []int{a, b}) {
		t.Fatalf("member inbounds = %v, want %v: the added one attached, the one taken off by hand left off", got, []int{a, b})
	}
}

// breakInbound makes every client change on the inbound fail, as an unparsable
// inbound does, and returns the call that mends it.
func breakInbound(t *testing.T, inboundId int) (mend func()) {
	t.Helper()
	var ib model.Inbound
	if err := database.GetDB().First(&ib, inboundId).Error; err != nil {
		t.Fatalf("read inbound %d: %v", inboundId, err)
	}
	setSettings := func(settings string) {
		if err := database.GetDB().Model(&model.Inbound{}).Where("id = ?", inboundId).
			UpdateColumn("settings", settings).Error; err != nil {
			t.Fatalf("set inbound %d settings: %v", inboundId, err)
		}
	}
	setSettings(`{"clients":`)
	return func() { setSettings(ib.Settings) }
}

// A save that failed on a member had already stored the plan's new inbound, so
// saving again found nothing left to attach and the members it missed never got it.
func TestUpdatePlanSavedAgainAfterAFailedSaveReachesEveryMember(t *testing.T) {
	a, b, _ := setupPlanDB(t)
	members := []string{"one@resave", "two@resave"}
	for _, email := range members {
		createPlanClient(t, email, []int{a}, 0)
	}
	s := &PlanService{}
	plan, err := s.Create(PlanInput{Name: "Resave", InboundIds: []int{a}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if _, err := s.Assign(&InboundService{}, members, plan.Id); err != nil {
		t.Fatalf("assign: %v", err)
	}
	in := PlanInput{Name: "Resave", InboundIds: []int{a, b}}

	mend := breakInbound(t, b)
	_, err = s.Update(&InboundService{}, plan.Id, in, false)
	if want := "inbound " + strconv.Itoa(b) + ": "; err == nil || !strings.HasPrefix(err.Error(), want) {
		t.Fatalf("save over a broken inbound: err = %v, want one starting %q", err, want)
	}
	mend()
	if _, err := s.Update(&InboundService{}, plan.Id, in, false); err != nil {
		t.Fatalf("save again: %v", err)
	}

	for _, email := range members {
		if got := planInboundIdsOf(t, email); !slices.Equal(got, []int{a, b}) {
			t.Fatalf("%s inbounds = %v after saving again, want %v", email, got, []int{a, b})
		}
	}
}

// Members are updated before the plan is saved, so an edit the plan refuses has to
// be refused before any member is moved, not when the plan row is finally written.
func TestUpdatePlanRefusesABadEditBeforeMovingAnyMember(t *testing.T) {
	a, b, _ := setupPlanDB(t)
	createPlanClient(t, "member@refused", []int{a}, 0)
	s := &PlanService{}
	if _, err := s.Create(PlanInput{Name: "Taken"}); err != nil {
		t.Fatalf("create the other plan: %v", err)
	}
	plan, err := s.Create(PlanInput{Name: "Mine", InboundIds: []int{a}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if _, err := s.Assign(&InboundService{}, []string{"member@refused"}, plan.Id); err != nil {
		t.Fatalf("assign: %v", err)
	}

	_, err = s.Update(&InboundService{}, plan.Id, PlanInput{Name: "Taken", InboundIds: []int{a, b}}, false)
	if want := "a plan with this name already exists: Taken"; err == nil || strings.TrimSpace(err.Error()) != want {
		t.Errorf("err = %v, want %q", err, want)
	}
	if got := planInboundIdsOf(t, "member@refused"); !slices.Equal(got, []int{a}) {
		t.Fatalf("member inbounds = %v after a refused edit, want [%d]", got, a)
	}
}

// Unticked, members kept an inbound the plan no longer grants. Only what the plan
// lost may go: an inbound attached to a member by hand stays.
func TestUpdatePlanDetachesARemovedInboundAndKeepsOnesAttachedByHand(t *testing.T) {
	a, b, c := setupPlanDB(t)
	createPlanClient(t, "member@drop", []int{a}, 0)
	s := &PlanService{}
	plan, err := s.Create(PlanInput{Name: "Two", InboundIds: []int{a, b}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if _, err := s.Assign(&InboundService{}, []string{"member@drop"}, plan.Id); err != nil {
		t.Fatalf("assign: %v", err)
	}
	attachByHand(t, "member@drop", c)

	needRestart, err := s.Update(&InboundService{}, plan.Id, PlanInput{Name: "Two", InboundIds: []int{a}}, false)
	if err != nil {
		t.Fatalf("update plan: %v", err)
	}

	if got := planInboundIdsOf(t, "member@drop"); !slices.Equal(got, []int{a, c}) {
		t.Fatalf("member inbounds = %v, want %v: the removed one gone, the hand-attached one kept", got, []int{a, c})
	}
	if !needRestart {
		t.Fatal("Update dropped the restart removing the member from a local inbound needs")
	}
}

// A ticked save that keeps the servers needs a restart for the re-stamp alone. Without
// it the members' rewritten entries would wait for some unrelated restart to apply.
func TestUpdatePlanReapplyingOnlyLimitsAsksForARestart(t *testing.T) {
	a, _, _ := setupPlanDB(t)
	createPlanClient(t, "member@quota", []int{a}, 0)
	s := &PlanService{}
	plan, err := s.Create(PlanInput{Name: "Quota", LimitIP: 1, InboundIds: []int{a}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if _, err := s.Assign(&InboundService{}, []string{"member@quota"}, plan.Id); err != nil {
		t.Fatalf("assign: %v", err)
	}

	in := PlanInput{Name: "Quota", LimitIP: 4, InboundIds: []int{a}}
	needRestart, err := s.Update(&InboundService{}, plan.Id, in, true)
	if err != nil {
		t.Fatalf("update plan: %v", err)
	}

	if got := planRecord(t, "member@quota").LimitIP; got != 4 {
		t.Fatalf("member IP limit = %d, want the re-applied 4", got)
	}
	// No Xray runs here, so the re-stamped entry can only reach it through a restart.
	if !needRestart {
		t.Fatal("Update dropped the restart the member's re-stamped limits need")
	}
}

// Re-applying limits also made each member's inbounds exactly the plan's,
// stripping the ones an admin had attached by hand.
func TestUpdatePlanReapplyingLimitsKeepsExpiryAndHandAttachedInbounds(t *testing.T) {
	a, b, c := setupPlanDB(t)
	expiry := time.Now().Add(20 * 24 * time.Hour).UnixMilli()
	createPlanClient(t, "member@upd", []int{a}, expiry)
	createPlanClient(t, "outsider@upd", []int{a}, 0)
	giveQuota(t, "member@upd", 100*planGiB)
	s := &PlanService{}
	plan, err := s.Create(PlanInput{Name: "Basic", LimitIP: 1, InboundIds: []int{a}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if _, err := s.Assign(&InboundService{}, []string{"member@upd"}, plan.Id); err != nil {
		t.Fatalf("assign: %v", err)
	}
	attachByHand(t, "member@upd", c)

	in := PlanInput{Name: "Basic", LimitIP: 3, InboundIds: []int{b}}
	if _, err := s.Update(&InboundService{}, plan.Id, in, true); err != nil {
		t.Fatalf("update plan: %v", err)
	}

	member := planRecord(t, "member@upd")
	if member.TotalGB != 100*planGiB || member.LimitIP != 3 || member.ExpiryTime != expiry {
		t.Fatalf("member quota %d limitIp %d expiry %d, want the new IP limit 3 with its own 100 GiB and expiry %d",
			member.TotalGB, member.LimitIP, member.ExpiryTime, expiry)
	}
	if got := planInboundIdsOf(t, "member@upd"); !slices.Equal(got, []int{b, c}) {
		t.Fatalf("member inbounds = %v, want the plan's new [%d] plus the hand-attached [%d]", got, b, c)
	}
	if entry := inboundClientEntry(t, b, "member@upd"); entry["limitIp"] != float64(3) || entry["totalGB"] != float64(100*planGiB) {
		t.Fatalf("added inbound entry = %v, want the re-applied IP limit and its own quota", entry)
	}
	if outsider := planRecord(t, "outsider@upd"); outsider.LimitIP != 0 {
		t.Fatalf("a client outside the plan got IP limit %d", outsider.LimitIP)
	}
}

// putOnPlan makes clients members of a plan without stamping it, so their own
// limits stay unlike the plan's.
func putOnPlan(t *testing.T, planId int, emails ...string) {
	t.Helper()
	if err := database.GetDB().Model(&model.ClientRecord{}).Where("email IN ?", emails).
		UpdateColumn("plan_id", planId).Error; err != nil {
		t.Fatalf("put %v on plan %d: %v", emails, planId, err)
	}
}

// A node generated for some plans must reach their members with their own limits and
// other inbounds intact, reach no one else, and survive being granted twice.
func TestAddInboundToPlansAttachesOnlyTheirMembersAndIsIdempotent(t *testing.T) {
	a, b, c := setupPlanDB(t)
	s := &PlanService{}
	planIds := map[string]int{}
	for _, name := range []string{"hk", "sg", "us"} {
		createPlanClient(t, name+"@grant", []int{a}, 0)
		plan, err := s.Create(PlanInput{Name: name, LimitIP: 2, InboundIds: []int{a}})
		if err != nil {
			t.Fatalf("create plan %s: %v", name, err)
		}
		putOnPlan(t, plan.Id, name+"@grant")
		planIds[name] = plan.Id
	}
	attachByHand(t, "hk@grant", c)
	chosen := []int{planIds["hk"], planIds["sg"]}

	needRestart, err := s.AddInboundToPlans(&InboundService{}, b, chosen)
	if err != nil {
		t.Fatalf("add the inbound to plans: %v", err)
	}
	if !needRestart {
		t.Fatal("AddInboundToPlans dropped the restart the members' new local inbound needs")
	}
	if needRestart, err = s.AddInboundToPlans(&InboundService{}, b, chosen); err != nil || needRestart {
		t.Fatalf("granting it again: needRestart %v err %v, want a no-op", needRestart, err)
	}

	for email, want := range map[string][]int{"hk@grant": {a, b, c}, "sg@grant": {a, b}, "us@grant": {a}} {
		if got := planInboundIdsOf(t, email); !slices.Equal(got, want) {
			t.Fatalf("%s inbounds = %v, want %v", email, got, want)
		}
	}
	if rec := planRecord(t, "hk@grant"); rec.TotalGB != 0 || rec.LimitIP != 0 {
		t.Fatalf("member quota %d limitIp %d, want its own unlimited values, not the plan's", rec.TotalGB, rec.LimitIP)
	}
	plans, err := s.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, p := range plans {
		want := []int{a, b}
		if p.Id == planIds["us"] {
			want = []int{a}
		}
		if !slices.Equal(p.InboundIds, want) {
			t.Fatalf("plan %s inbounds = %v, want %v", p.Name, p.InboundIds, want)
		}
	}
}

// A grant that failed partway had already stored the plan rows, so a retry must reach
// the members it missed, not only those of plans that gain the inbound on that call.
func TestAddInboundToPlansRetriedAfterAFailureReachesEveryMember(t *testing.T) {
	a, b, _ := setupPlanDB(t)
	createPlanClient(t, "member@regrant", []int{a}, 0)
	s := &PlanService{}
	plan, err := s.Create(PlanInput{Name: "Regrant", InboundIds: []int{a}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	putOnPlan(t, plan.Id, "member@regrant")

	mend := breakInbound(t, b)
	_, err = s.AddInboundToPlans(&InboundService{}, b, []int{plan.Id})
	if want := "inbound " + strconv.Itoa(b) + ": "; err == nil || !strings.HasPrefix(err.Error(), want) {
		t.Fatalf("grant over a broken inbound: err = %v, want one starting %q", err, want)
	}
	mend()
	needRestart, err := s.AddInboundToPlans(&InboundService{}, b, []int{plan.Id})
	if err != nil {
		t.Fatalf("grant again: %v", err)
	}

	if got := planInboundIdsOf(t, "member@regrant"); !slices.Equal(got, []int{a, b}) {
		t.Fatalf("member inbounds = %v after granting again, want %v", got, []int{a, b})
	}
	if !needRestart {
		t.Fatal("the retried grant dropped the restart the member's new inbound needs")
	}
}

// The ids arrive in a request; a stale one must fail the grant before any plan
// or member changes, rather than leave it half applied.
func TestAddInboundToPlansWritesNothingForAnUnknownPlanOrInbound(t *testing.T) {
	a, b, _ := setupPlanDB(t)
	createPlanClient(t, "member@stale", []int{a}, 0)
	s := &PlanService{}
	plan, err := s.Create(PlanInput{Name: "Known", InboundIds: []int{a}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	putOnPlan(t, plan.Id, "member@stale")

	cases := []struct {
		name      string
		inboundId int
		planIds   []int
		wantErr   string
	}{
		{"unknown plan", b, []int{plan.Id, 9999}, "plan not found: [9999]"},
		{"unknown inbound", 9999, []int{plan.Id}, "inbound not found: 9999"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := s.AddInboundToPlans(&InboundService{}, tc.inboundId, tc.planIds)
			if err == nil || strings.TrimSpace(err.Error()) != tc.wantErr {
				t.Fatalf("err = %v, want %q", err, tc.wantErr)
			}
			var written int64
			if err := database.GetDB().Model(&model.PlanInbound{}).Where("inbound_id <> ?", a).Count(&written).Error; err != nil {
				t.Fatalf("count plan inbounds: %v", err)
			}
			if written != 0 {
				t.Fatalf("%d plan_inbounds rows written for a refused grant", written)
			}
			if got := planInboundIdsOf(t, "member@stale"); !slices.Equal(got, []int{a}) {
				t.Fatalf("member inbounds = %v after a refused grant, want [%d]", got, a)
			}
		})
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
		"negative IP limit": {Name: "I", LimitIP: -1},
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
	if _, err := s.Assign(&InboundService{}, []string{"one@list", "two@list"}, plan.Id); err != nil {
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

// A plan names its rule template by id; 0 leaves it on the default template.
func TestPlanTemplateMustExist(t *testing.T) {
	setupPlanDB(t)
	svc := &PlanService{}
	if _, err := svc.Create(PlanInput{Name: "Ghost", TemplateId: 42}); err == nil || !strings.Contains(err.Error(), "rule template that does not exist") {
		t.Fatalf("create on a missing template: err = %v, want it refused", err)
	}
	tpl, err := (&RuleTemplateService{}).Create(RuleTemplateInput{Name: "alpha_v3", Content: "MATCH,DIRECT"})
	if err != nil {
		t.Fatalf("create template: %v", err)
	}
	plan, err := svc.Create(PlanInput{Name: "Real", TemplateId: tpl.Id})
	if err != nil || plan.TemplateId != tpl.Id {
		t.Fatalf("create on the template: plan %+v, err %v; want it stored", plan, err)
	}
	if plan, err := svc.Create(PlanInput{Name: "Default"}); err != nil || plan.TemplateId != 0 {
		t.Fatalf("create without a template: plan %+v, err %v; want template 0", plan, err)
	}
}
