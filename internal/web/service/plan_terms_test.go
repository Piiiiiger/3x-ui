package service

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

func termsOf(t *testing.T, planId int) []int {
	t.Helper()
	plans, err := (&PlanService{}).List()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range plans {
		if p.Id == planId {
			return p.TermDays
		}
	}
	t.Fatalf("plan %d is not listed", planId)
	return nil
}

// A plan sold only by some terms keeps each once, shortest first; a term is a
// whole number of days the panel can renew by.
func TestPlanTermsAreKeptSortedAndChecked(t *testing.T) {
	a, _, _ := setupPlanDB(t)
	s := &PlanService{}
	plan, err := s.Create(PlanInput{Name: "SG", InboundIds: []int{a}, TermDays: []int{365, 90, 90}})
	if err != nil {
		t.Fatal(err)
	}
	if got := termsOf(t, plan.Id); !slices.Equal(got, []int{90, 365}) {
		t.Fatalf("terms = %v, want [90 365]", got)
	}
	for i, bad := range [][]int{{0}, {-30}, {36501}} {
		_, err := s.Create(PlanInput{Name: fmt.Sprint("bad", i), InboundIds: []int{a}, TermDays: bad})
		if err == nil || !strings.Contains(err.Error(), "a term is 1 to 36500 days") {
			t.Fatalf("terms %v: %v, want them refused", bad, err)
		}
	}
	if _, err := s.Update(&InboundService{}, plan.Id, PlanInput{Name: "SG", InboundIds: []int{a}}, false); err != nil {
		t.Fatal(err)
	}
	if got := termsOf(t, plan.Id); len(got) != 0 {
		t.Fatalf("terms after clearing = %v, want none", got)
	}
}

// A user on a plan sold by terms renews only by one of them, from the panel and
// the bot alike; a renewal of several users with one refused renews none.
func TestRenewKeepsToThePlansTerms(t *testing.T) {
	a, _, _ := setupPlanDB(t)
	expiry := time.Now().Add(20 * 24 * time.Hour).UnixMilli()
	createPlanClient(t, "sg@plan", []int{a}, expiry)
	createPlanClient(t, "free@plan", []int{a}, expiry)
	s := &PlanService{}
	plan, err := s.Create(PlanInput{Name: "SG", InboundIds: []int{a}, TermDays: []int{90, 365}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Assign(&InboundService{}, []string{"sg@plan"}, plan.Id); err != nil {
		t.Fatal(err)
	}

	_, err = (&ClientService{}).Renew(&InboundService{}, []string{"free@plan", "sg@plan"}, 30, false)
	if err == nil || !strings.Contains(err.Error(), `sg@plan is on plan "SG", which renews by 90 or 365 days only`) {
		t.Fatalf("Renew(30) = %v, want it refused", err)
	}
	for _, email := range []string{"free@plan", "sg@plan"} {
		if got := planRecord(t, email).ExpiryTime; got != expiry {
			t.Fatalf("%s expiry moved to %d by a refused renewal", email, got)
		}
	}
	if _, err := (&ClientService{}).Renew(&InboundService{}, []string{"sg@plan"}, 90, false); err != nil {
		t.Fatalf("Renew(90) = %v", err)
	}
	if _, err := (&ClientService{}).Renew(&InboundService{}, []string{"free@plan"}, 30, false); err != nil {
		t.Fatalf("a user on no restricted plan renews by any days: %v", err)
	}
}

// Codes for a plan sold by terms grant one of those terms, never another.
func TestActivationCodesKeepToThePlansTerms(t *testing.T) {
	a, _, _ := setupPlanDB(t)
	plan, err := (&PlanService{}).Create(PlanInput{Name: "SG", InboundIds: []int{a}, TermDays: []int{90, 365}})
	if err != nil {
		t.Fatal(err)
	}
	for _, days := range []int{0, 30} {
		_, err := (&ActivationCodeService{}).Create(ActivationCodeInput{PlanId: plan.Id, Count: 1, Days: days})
		if err == nil || !strings.Contains(err.Error(), `plan "SG" is sold by 90 or 365 days only`) {
			t.Fatalf("a %d-day code: %v, want it refused", days, err)
		}
	}
	mustCreateCodes(t, ActivationCodeInput{PlanId: plan.Id, Count: 1, Days: 365})
}
