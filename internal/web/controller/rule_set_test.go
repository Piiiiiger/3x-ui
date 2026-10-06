package controller

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

// seedPlanMember gives a plan one member with a VLESS node, so a preview renders.
func seedPlanMember(t *testing.T) int {
	t.Helper()
	db := database.GetDB()
	plan := &model.Plan{Name: "Standard"}
	ib := &model.Inbound{
		UserId: 1, Tag: "edge", Enable: true, Listen: "203.0.113.5", Port: 48000, Protocol: model.VLESS,
		Settings:       `{"clients":[{"id":"11111111-2222-4333-8444-000000048000","email":"amy@e","subId":"s-amy","enable":true}],"decryption":"none"}`,
		StreamSettings: `{"network":"tcp","security":"none"}`,
	}
	for _, row := range []any{plan, ib} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	member := &model.ClientRecord{Email: "amy@e", SubID: "s-amy", Enable: true, PlanId: plan.Id}
	if err := db.Create(member).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ClientInbound{ClientId: member.Id, InboundId: ib.Id}).Error; err != nil {
		t.Fatal(err)
	}
	return plan.Id
}

// A review's proposed rules are tried on a plan's member before any save serves
// them; ones a save would refuse are refused here too.
func TestRuleTemplatePreviewTriesProposedRuleSets(t *testing.T) {
	engine := newRuleTemplateEngine(t)
	planId := seedPlanMember(t)
	preview := map[string]any{
		"planId": planId, "content": "RULE-SET,ai,PROXY\nMATCH,PROXY",
		"ruleSets": map[string]string{"ai": "DOMAIN-SUFFIX,proposed.example\n"},
	}
	reply := callRuleTemplates(t, engine, http.MethodPost, "/preview", preview)
	var out service.RuleTemplatePreview
	if err := json.Unmarshal(reply.Obj, &out); err != nil || !reply.Success || !strings.Contains(out.Content, "DOMAIN-SUFFIX,proposed.example,PROXY") {
		t.Fatalf("preview = %+v (err %v), want the proposed rule on the member's config", reply, err)
	}

	preview["ruleSets"] = map[string]string{"ai": "IP-ASN,64496\n"}
	if reply := callRuleTemplates(t, engine, http.MethodPost, "/preview", preview); reply.Success || !strings.Contains(reply.Msg, "rule set ai: line 1: IP-ASN") {
		t.Fatalf("preview of rules a save refuses: %+v", reply)
	}
}
