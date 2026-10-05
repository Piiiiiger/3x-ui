package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/web/locale"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

func newAiUsageTestEngine(t *testing.T) *gin.Engine {
	t.Helper()
	newControllerTestDB(t)
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set("I18n", func(_ locale.I18nType, key string, _ ...string) string { return key })
		c.Next()
	})
	NewAiUsageController(engine.Group("/panel/api/aiUsage"))
	return engine
}

func aiUsageCall(t *testing.T, engine *gin.Engine, method, path, body string) apiEnvelope {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	var reply apiEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &reply); err != nil {
		t.Fatalf("%s %s: status %d, body %s", method, path, w.Code, w.Body.String())
	}
	return reply
}

// The desktop's JSON (camelCase, a session and a plan reading) must arrive whole,
// and the overview must read its period and app filter from the query string.
func TestAiUsageIngestThenOverviewOverHTTP(t *testing.T) {
	engine := newAiUsageTestEngine(t)
	// Sessions are kept 90 days, so this one must be recent.
	lastAt := time.Now().Unix() - 3600
	report := fmt.Sprintf(`{
		"device": {"key": "device-key-http", "name": "laptop", "appVersion": "1.0.0"},
		"from": "2026-01-01", "to": "2026-01-01",
		"daily": [
			{"day": "2026-01-01", "app": "claude", "project": "/w/app", "model": "opus",
			 "requests": 3, "inputTokens": 30, "outputTokens": 300, "cacheReadTokens": 3000,
			 "cacheWriteTokens": 3, "costUsd": 1.25},
			{"day": "2026-01-01", "app": "codex", "project": "/w/lib", "model": "gpt-5.5",
			 "requests": 2, "inputTokens": 20, "outputTokens": 200, "cacheReadTokens": 0,
			 "cacheWriteTokens": 0, "costUsd": 0.5}
		],
		"replaceSessions": true,
		"sessions": [{"app": "claude", "sessionId": "s1", "title": "fix login", "costUsd": 1.25,
			"requests": 3, "firstAt": %d, "lastAt": %d}],
		"quotas": [{"tool": "codex", "success": true, "planLabel": "Plus", "queriedAt": 1767229200000,
			"tiers": [{"name": "seven_day", "utilization": 64.5, "resetsAt": "2026-01-05T00:00:00Z"}]}]
	}`, lastAt-600, lastAt)
	if reply := aiUsageCall(t, engine, http.MethodPost, "/panel/api/aiUsage/ingest", report); !reply.Success {
		t.Fatalf("ingest refused: %s", reply.Msg)
	}

	reply := aiUsageCall(t, engine, http.MethodGet, "/panel/api/aiUsage/overview?period=all&app=claude", "")
	if !reply.Success {
		t.Fatalf("overview refused: %s", reply.Msg)
	}
	var ov service.AiUsageOverview
	if err := json.Unmarshal(reply.Obj, &ov); err != nil {
		t.Fatalf("decode overview: %v", err)
	}
	if ov.Period != "all" || ov.Totals.CostUsd != 1.25 || ov.Totals.Requests != 3 || ov.Totals.CacheReadTokens != 3000 {
		t.Fatalf("totals = %+v (period %q), want claude's $1.25 over all time", ov.Totals, ov.Period)
	}
	if len(ov.Sessions) != 1 || ov.Sessions[0].Title != "fix login" {
		t.Fatalf("sessions = %+v", ov.Sessions)
	}
	if len(ov.Quotas) != 1 || ov.Quotas[0].PlanLabel != "Plus" || ov.Quotas[0].Tiers[0].Utilization != 64.5 {
		t.Fatalf("quotas = %+v", ov.Quotas)
	}

	if reply := aiUsageCall(t, engine, http.MethodGet, "/panel/api/aiUsage/overview?period=year", ""); reply.Success {
		t.Fatal("an unknown period must be refused")
	}
}
