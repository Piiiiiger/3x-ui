package service

import (
	"sync"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func TestAccountActivationStableAndExclusive(t *testing.T) {
	setupConflictDB(t)
	db := database.GetDB()
	c := model.ClientRecord{Email: "one", Enable: true}
	if err := db.Create(&c).Error; err != nil {
		t.Fatal(err)
	}
	a, err := EnsureAccountActivation(&c)
	if err != nil {
		t.Fatal(err)
	}
	if !a.DailyEnabled || a.DailyTime != "20:00" {
		t.Fatal(a)
	}
	c.Email = "renamed"
	c.ExpiryTime = 123456789
	if err := db.Save(&c).Error; err != nil {
		t.Fatal(err)
	}
	b, err := EnsureAccountActivation(&c)
	if err != nil || b.Code != a.Code {
		t.Fatal("rename/renew changed credential", err)
	}
	var wg sync.WaitGroup
	outcomes := make(chan bool, 2)
	for _, id := range []int64{222, 333} {
		wg.Add(1)
		go func(id int64) { defer wg.Done(); _, err := BindAccountActivation(a.Code, id); outcomes <- err == nil }(id)
	}
	wg.Wait()
	close(outcomes)
	wins := 0
	for ok := range outcomes {
		if ok {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("claim winners=%d", wins)
	}
	db.First(&c, c.Id)
	if _, err := BindAccountActivation(a.Code, c.TgID); err != nil {
		t.Fatal("idempotent bind", err)
	}
	other := model.ClientRecord{Email: "two", Enable: true}
	db.Create(&other)
	otherCode, err := EnsureAccountActivation(&other)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BindAccountActivation(otherCode.Code, c.TgID); err == nil {
		t.Fatal("one Telegram claimed two accounts")
	}
	if _, err := BindAccountActivation(a.Code, 0); err == nil {
		t.Fatal("invalid sender accepted")
	}
}

func TestRegistrationKeepsOriginalActivationCode(t *testing.T) {
	a, _, _ := setupPlanDB(t)
	plan := codePlan(t, "Monthly", 0, a)
	codes := mustCreateCodes(t, ActivationCodeInput{PlanId: plan.Id, Count: 1, Days: 30})
	client, _, err := (&ActivationCodeService{}).Register(&InboundService{}, "newuser", "secret-pass", codes[0].Code)
	if err != nil {
		t.Fatal(err)
	}
	row, err := EnsureAccountActivation(client)
	if err != nil || row.Code != codes[0].Code {
		t.Fatalf("registration code not preserved: %v", err)
	}
}
