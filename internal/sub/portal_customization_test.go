package sub

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func TestPortalCustomizationIsPrivateAndReachesOnlyThatClashSubscription(t *testing.T) {
	router, controller := seedPortal(t)
	cookie := sessionCookie(t, portalLogin(router, "pa@e", "alpha-pass", "198.51.100.1"))
	payload := map[string]any{
		"nodesYaml": "proxies:\n  - name: personal-vless\n    type: vless\n    server: example.com\n    port: 443\n    uuid: 00000000-0000-0000-0000-000000000001\n    tls: true\n",
		"rulesYaml": "proxy-groups:\n  - name: PERSONAL\n    type: select\n    proxies: [personal-vless, DIRECT]\nrules:\n  - MATCH,PERSONAL\n",
		"links":     []map[string]string{{"kind": "link", "value": clashExternalVlessLink, "remark": "personal-link"}},
	}
	body, _ := json.Marshal(payload)
	res := portalRequest(router, http.MethodPut, "/sub/portal/customization", string(body), "198.51.100.1", cookie)
	if res.Code != http.StatusOK {
		t.Fatalf("save customization = %d %s, want 200", res.Code, res.Body)
	}
	var got struct {
		NodeCount int   `json:"nodeCount"`
		Links     []any `json:"links"`
		Groups    []struct {
			Name string `json:"name"`
		} `json:"groups"`
		Rules []string `json:"rules"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode customization: %v", err)
	}
	if got.NodeCount != 2 || len(got.Links) != 1 || len(got.Groups) == 0 || got.Groups[0].Name != "PERSONAL" || len(got.Rules) == 0 || got.Rules[0] != "MATCH,PERSONAL" {
		t.Fatalf("customization response = %+v", got)
	}

	clash, _, err := controller.subClashService.GetClash("s1", "example.com")
	if err != nil {
		t.Fatalf("personal Clash = %v", err)
	}
	if !strings.Contains(clash, "personal-vless") || !strings.Contains(clash, "personal-link") || !strings.Contains(clash, "MATCH,PERSONAL") {
		t.Fatalf("personal Clash does not contain its overlay:\n%s", clash)
	}
	other, _, err := controller.subClashService.GetClash("s2", "example.com")
	if err != nil {
		t.Fatalf("other Clash = %v", err)
	}
	if strings.Contains(other, "personal-vless") || strings.Contains(other, "PERSONAL") {
		t.Fatalf("other user's Clash contains pa@e's overlay:\n%s", other)
	}

	var rows []model.ClientSubscriptionCustomization
	if err := database.GetDB().Find(&rows).Error; err != nil {
		t.Fatalf("load customization rows: %v", err)
	}
	if len(rows) != 1 || rows[0].ClientId != clientIdOf(t, "pa@e") {
		t.Fatalf("customization rows = %+v, want only pa@e", rows)
	}
}

func TestPortalCustomizationRollbackRestoresPreviousOverlay(t *testing.T) {
	router, _ := seedPortal(t)
	cookie := sessionCookie(t, portalLogin(router, "pa@e", "alpha-pass", "198.51.100.1"))
	save := func(nodes string) *struct {
		Versions []struct {
			Id int `json:"id"`
		} `json:"versions"`
	} {
		body, _ := json.Marshal(map[string]any{"nodesYaml": nodes, "rulesYaml": ""})
		res := portalRequest(router, http.MethodPut, "/sub/portal/customization", string(body), "198.51.100.1", cookie)
		if res.Code != http.StatusOK {
			t.Fatalf("save = %d %s", res.Code, res.Body)
		}
		var out struct {
			Versions []struct {
				Id int `json:"id"`
			} `json:"versions"`
		}
		if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode save: %v", err)
		}
		return &out
	}
	first := save("proxies:\n  - name: first\n    type: ss\n    server: example.com\n    port: 443\n    cipher: aes-128-gcm\n    password: pass\n")
	if len(first.Versions) != 0 {
		t.Fatalf("first save versions = %+v, want none", first.Versions)
	}
	second := save("proxies:\n  - name: second\n    type: ss\n    server: example.com\n    port: 443\n    cipher: aes-128-gcm\n    password: pass\n")
	if len(second.Versions) != 1 {
		t.Fatalf("second save versions = %+v, want one", second.Versions)
	}
	body, _ := json.Marshal(map[string]int{"versionId": second.Versions[0].Id})
	res := portalRequest(router, http.MethodPost, "/sub/portal/customization/rollback", string(body), "198.51.100.1", cookie)
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), "first") || strings.Contains(res.Body.String(), "second") {
		t.Fatalf("rollback = %d %s", res.Code, res.Body)
	}
}

// A rule file can exceed both legacy limits (256 KiB rules, 1 MiB JSON body).
// Saving and subscription rendering must agree on the supported size.
func TestPortalCustomizationSavesLargeRulesAndRendersSubscription(t *testing.T) {
	router, controller := seedPortal(t)
	cookie := sessionCookie(t, portalLogin(router, "pa@e", "alpha-pass", "198.51.100.1"))
	rules := strings.Repeat("# "+strings.Repeat("large rule comment ", 8)+"\n", 8500) + "rules:\n  - DOMAIN-SUFFIX,large.example,DIRECT\n  - MATCH,DIRECT\n"
	if len(rules) <= 1<<20 {
		t.Fatal("fixture must exceed the old request limit")
	}
	body, _ := json.Marshal(map[string]any{"nodesYaml": "", "links": []any{}, "rulesYaml": rules})
	res := portalRequest(router, http.MethodPut, "/sub/portal/customization", string(body), "198.51.100.1", cookie)
	if res.Code != http.StatusOK {
		t.Fatalf("large save = %d: %.300s", res.Code, res.Body.String())
	}
	var out struct {
		MaxRulesBytes int `json:"maxRulesBytes"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.MaxRulesBytes != portalCustomizationMaxRules {
		t.Fatal("editor limit differs from validator")
	}
	var row model.ClientSubscriptionCustomization
	if err := database.GetDB().First(&row, "client_id = ?", clientIdOf(t, "pa@e")).Error; err != nil {
		t.Fatal(err)
	}
	if row.Rules != strings.TrimSpace(rules) {
		t.Fatal("saved rules truncated")
	}
	clash, _, err := controller.subClashService.GetClash("s1", "example.com")
	if err != nil || !strings.Contains(clash, "DOMAIN-SUFFIX,large.example,DIRECT") {
		t.Fatalf("large overlay missing from subscription: %v", err)
	}
}

func TestPortalCustomizationKeepsBoundedRuleAndNodeInputs(t *testing.T) {
	if _, err := portalRulesDocument(strings.Repeat("x", portalCustomizationMaxRules+1)); err == nil || !strings.Contains(err.Error(), "4 MB") {
		t.Fatalf("oversized rules: %v", err)
	}
	if _, err := portalNodeMaps(strings.Repeat("x", portalCustomizationMaxNodeYAML+1)); err == nil {
		t.Fatal("oversized nodes accepted after increasing request limit")
	}
}
