package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

func TestPlanCodesCanBeCreatedListedAndRevoked(t *testing.T) {
	engine := newProbeTestEngine(t)
	NewPlanController(engine.Group("/panel/api/plans"))
	ib := &model.Inbound{Tag: "edge", Protocol: model.VLESS, Port: 48000, Settings: `{"clients":[]}`}
	if err := database.GetDB().Create(ib).Error; err != nil {
		t.Fatal(err)
	}
	plan, err := (&service.PlanService{}).Create(service.PlanInput{Name: "Standard", InboundIds: []int{ib.Id}})
	if err != nil {
		t.Fatal(err)
	}
	created := postJSON(t, engine, "/panel/api/plans/codes/add", service.ActivationCodeInput{PlanId: plan.Id, Count: 2, TotalGB: 10 << 30, Days: 30, ResetDay: 9})
	var codes []model.ActivationCode
	if err := json.Unmarshal(created.Obj, &codes); err != nil || !created.Success || len(codes) != 2 {
		t.Fatalf("create codes = %+v (%v), want two codes", created, err)
	}
	firstId, secondId := codes[0].Id, codes[1].Id
	listPath := fmt.Sprintf("/panel/api/plans/codes/%d", plan.Id)
	listed := getProbe(t, engine, listPath)
	if err := json.Unmarshal(listed.Obj, &codes); err != nil || !listed.Success || len(codes) != 2 || codes[0].Id != secondId || codes[1].Id != firstId {
		t.Fatalf("list codes = %+v, want the batch newest first", listed)
	}
	deleted := postJSON(t, engine, fmt.Sprintf("/panel/api/plans/codes/del/%d", firstId), nil)
	if !deleted.Success {
		t.Fatalf("revoke code = %+v", deleted)
	}
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, listPath, nil))
	var reply struct {
		Success bool                   `json:"success"`
		Obj     []model.ActivationCode `json:"obj"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &reply); err != nil || !reply.Success || len(reply.Obj) != 1 || reply.Obj[0].Id != secondId {
		t.Fatalf("after revoke = %s (%v), want only the second code", w.Body, err)
	}
}
