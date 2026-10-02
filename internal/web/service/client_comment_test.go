package service

import (
	"slices"
	"testing"
	"time"
)

// The clients table edits a remark in place, so the save must not rebuild the
// rest of the client the way a full form update does.
func TestSetCommentChangesOnlyTheComment(t *testing.T) {
	a, b, _ := setupPlanDB(t)
	expiry := time.Now().Add(20 * 24 * time.Hour).UnixMilli()
	createPlanClient(t, "note@plan", []int{a, b}, expiry)
	before := planRecord(t, "note@plan")

	if _, err := (&ClientService{}).SetComment(&InboundService{}, "note@plan", "  pays on the 5th  "); err != nil {
		t.Fatalf("set comment: %v", err)
	}

	after := planRecord(t, "note@plan")
	if after.Comment != "pays on the 5th" {
		t.Fatalf("comment = %q, want the trimmed remark", after.Comment)
	}
	if after.UUID != before.UUID || after.SubID != before.SubID || after.ExpiryTime != before.ExpiryTime ||
		after.TotalGB != before.TotalGB || after.Enable != before.Enable || after.PlanId != before.PlanId {
		t.Fatalf("client changed beyond its comment:\nbefore %+v\nafter  %+v", before, after)
	}
	if got := planInboundIdsOf(t, "note@plan"); !slices.Equal(got, []int{a, b}) {
		t.Fatalf("inbounds = %v, want [%d %d]", got, a, b)
	}
	for _, id := range []int{a, b} {
		if got := inboundClientEntry(t, id, "note@plan")["comment"]; got != "pays on the 5th" {
			t.Fatalf("inbound %d still carries comment %v", id, got)
		}
	}
}

func TestSetCommentRefusesAnUnknownClient(t *testing.T) {
	setupPlanDB(t)
	if _, err := (&ClientService{}).SetComment(&InboundService{}, "ghost@plan", "x"); err == nil {
		t.Fatal("setting a comment on a client that does not exist succeeded")
	}
}
