package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/web/locale"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

func newRuleTemplateEngine(t *testing.T) *gin.Engine {
	t.Helper()
	newControllerTestDB(t)
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set("I18n", func(_ locale.I18nType, key string, _ ...string) string { return key })
		c.Next()
	})
	NewRuleTemplateController(engine.Group("/panel/api/ruleTemplates"))
	return engine
}

func callRuleTemplates(t *testing.T, engine *gin.Engine, method, path string, body any) apiEnvelope {
	t.Helper()
	var payload bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&payload).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, "/panel/api/ruleTemplates"+path, &payload)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	var reply apiEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &reply); err != nil || w.Code != http.StatusOK {
		t.Fatalf("%s %s: status %d, body %s", method, path, w.Code, w.Body.String())
	}
	return reply
}

func TestRuleTemplateAPI_SavesListsAndMovesTheDefault(t *testing.T) {
	engine := newRuleTemplateEngine(t)
	created := callRuleTemplates(t, engine, http.MethodPost, "/add", service.RuleTemplateInput{Name: "pigger_v3", Content: "MATCH,DIRECT"})
	var tpl struct{ Id int }
	if err := json.Unmarshal(created.Obj, &tpl); err != nil || !created.Success || tpl.Id == 0 {
		t.Fatalf("add: %+v (err %v)", created, err)
	}

	if reply := callRuleTemplates(t, engine, http.MethodPost, "/setDefault/"+strconv.Itoa(tpl.Id), nil); !reply.Success {
		t.Fatalf("setDefault: %s", reply.Msg)
	}
	listed := callRuleTemplates(t, engine, http.MethodGet, "/list", nil)
	var list []service.RuleTemplateSummary
	if err := json.Unmarshal(listed.Obj, &list); err != nil || len(list) != 1 || !list[0].IsDefault || list[0].Kind != "rules" {
		t.Fatalf("list = %s (err %v), want the one template as the default", listed.Obj, err)
	}

	if reply := callRuleTemplates(t, engine, http.MethodPost, "/del/"+strconv.Itoa(tpl.Id), nil); reply.Success || !strings.Contains(reply.Msg, "default") {
		t.Fatalf("deleting the default: %+v, want it refused", reply)
	}
}

func TestRuleTemplateAPI_RefusesWhatCannotRender(t *testing.T) {
	engine := newRuleTemplateEngine(t)
	broken := "proxy-groups:\n  - name: PROXY\n    type: select\n    proxies: [DIRECT]\n"
	if reply := callRuleTemplates(t, engine, http.MethodPost, "/add", service.RuleTemplateInput{Name: "x", Content: broken}); reply.Success || !strings.Contains(reply.Msg, "__PROXY_NODES__") {
		t.Fatalf("add without __PROXY_NODES__: %+v, want it refused", reply)
	}
	if reply := callRuleTemplates(t, engine, http.MethodPost, "/preview", map[string]any{"planId": 1, "content": "MATCH,DIRECT"}); reply.Success || !strings.Contains(reply.Msg, "no users") {
		t.Fatalf("preview on a plan without users: %+v, want it refused", reply)
	}
}
