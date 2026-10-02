package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/web/locale"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

func getReply(t *testing.T, engine *gin.Engine, path string) apiReply {
	t.Helper()
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	var reply apiReply
	if err := json.Unmarshal(w.Body.Bytes(), &reply); err != nil {
		t.Fatalf("%s: status %d, body %s", path, w.Code, w.Body.String())
	}
	return reply
}

// Host 0 is the local panel, which has no node row; asking a node for it would
// answer "node not found: 0" and the form could never suggest a port there.
func TestInboundFreePortRouteReadsItsHost(t *testing.T) {
	newHostTestDB(t)
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set("I18n", func(_ locale.I18nType, key string, _ ...string) string { return key })
		c.Next()
	})
	NewInboundController(engine.Group("/panel/api/inbounds"))

	local := getReply(t, engine, "/panel/api/inbounds/freePort/0")
	if !local.Success {
		t.Fatalf("local panel: %s", local.Msg)
	}
	var view service.FreePortView
	if err := json.Unmarshal(local.Obj, &view); err != nil || view.Port < 10000 || view.Port > 60000 {
		t.Fatalf("local panel suggested %s (err %v), want a port in 10000-60000", local.Obj, err)
	}

	for path, want := range map[string]string{
		"/panel/api/inbounds/freePort/999": "pages.inbounds.toasts.obtain (node not found: 999)",
		"/panel/api/inbounds/freePort/abc": "get (strconv.Atoi: parsing \"abc\": invalid syntax)",
		"/panel/api/inbounds/freePort/-1":  "get (node id must not be negative: -1)",
	} {
		if reply := getReply(t, engine, path); reply.Success || reply.Msg != want {
			t.Fatalf("%s: success %v, msg %q, want failure %q", path, reply.Success, reply.Msg, want)
		}
	}
}
