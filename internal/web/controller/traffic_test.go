package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/web/locale"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

func getTrafficOverview(t *testing.T, query string) apiEnvelope {
	t.Helper()
	newControllerTestDB(t)
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set("I18n", func(_ locale.I18nType, key string, _ ...string) string { return key })
		c.Next()
	})
	NewTrafficController(engine.Group("/panel/api/traffic"))
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/panel/api/traffic/overview"+query, nil))
	var reply apiEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &reply); err != nil || w.Code != http.StatusOK {
		t.Fatalf("GET overview%s: status %d, body %s", query, w.Code, w.Body.String())
	}
	return reply
}

// The page's period switch reaches the rankings, and the page opens on the month.
func TestTrafficOverviewTakesThePeriodFromTheQuery(t *testing.T) {
	for query, want := range map[string]string{"": "month", "?period=week": "week", "?period=today": "today"} {
		reply := getTrafficOverview(t, query)
		var ov service.TrafficOverview
		if err := json.Unmarshal(reply.Obj, &ov); err != nil || !reply.Success || ov.Period != want {
			t.Errorf("overview%s: success %v, period %q (err %v); want %q", query, reply.Success, ov.Period, err, want)
		}
	}
}

// A period the page does not offer is refused, not quietly read as the month.
func TestTrafficOverviewRefusesAnUnknownPeriod(t *testing.T) {
	reply := getTrafficOverview(t, "?period=year")
	if reply.Success || !strings.Contains(reply.Msg, `unknown traffic period "year"`) {
		t.Fatalf("overview?period=year: success %v, msg %q; want it refused naming the period", reply.Success, reply.Msg)
	}
}
