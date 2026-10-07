package controller

import (
	"encoding/json"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/web/locale"
)

func newTrafficMultiplierTestEngine(t *testing.T) *gin.Engine {
	t.Helper()
	newControllerTestDB(t)
	a := &ServerController{}
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set("I18n", func(_ locale.I18nType, key string, _ ...string) string { return key })
		c.Next()
	})
	engine.GET("/panel/api/server/trafficMultiplier", a.getTrafficMultiplier)
	engine.POST("/panel/api/server/trafficMultiplier", a.setTrafficMultiplier)
	return engine
}

func panelTrafficMultiplier(t *testing.T, engine *gin.Engine) float64 {
	t.Helper()
	reply := getReply(t, engine, "/panel/api/server/trafficMultiplier")
	var view struct {
		Multiplier float64 `json:"multiplier"`
	}
	if err := json.Unmarshal(reply.Obj, &view); err != nil || !reply.Success {
		t.Fatalf("read the multiplier: %s, %v", reply.Obj, err)
	}
	return view.Multiplier
}

// A save that leaves the value out must not read as 0 and make the panel's
// own host free.
func TestPanelTrafficMultiplierSaveNeedsAValue(t *testing.T) {
	engine := newTrafficMultiplierTestEngine(t)

	if reply := postJSON(t, engine, "/panel/api/server/trafficMultiplier", map[string]any{}); reply.Success {
		t.Fatal("a save without a multiplier must be refused")
	}
	if got := panelTrafficMultiplier(t, engine); got != 1 {
		t.Fatalf("after a refused save the multiplier = %v, want 1", got)
	}

	if reply := postJSON(t, engine, "/panel/api/server/trafficMultiplier", map[string]any{"multiplier": 0.1}); !reply.Success {
		t.Fatalf("save 0.1: %s", reply.Msg)
	}
	if got := panelTrafficMultiplier(t, engine); got != 0.1 {
		t.Fatalf("the multiplier = %v, want 0.1", got)
	}
}
