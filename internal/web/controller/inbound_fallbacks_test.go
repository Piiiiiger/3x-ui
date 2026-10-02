package controller

import (
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/locale"
	"github.com/mhsanaei/3x-ui/v3/internal/web/runtime"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

// The inbound form posts a TLS or REALITY master's fallbacks right after the master.
// They are part of its config on an agent, which got them only on its next sync tick.
func TestInboundFallbacksReachAnAgentAtOnce(t *testing.T) {
	newHostTestDB(t)
	prevManager := runtime.GetManager()
	runtime.SetManager(runtime.NewManager(runtime.LocalDeps{APIPort: func() int { return 0 }, SetNeedRestart: func() {}}))
	t.Cleanup(func() { runtime.SetManager(prevManager) })
	hub := runtime.NewAgentHub()
	prevHub := runtime.GetAgentHub()
	runtime.SetAgentHub(hub)
	t.Cleanup(func() { runtime.SetAgentHub(prevHub) })

	agent := &model.Node{Name: "edge-hk", Kind: model.NodeKindAgent, Address: "203.0.113.53", Enable: true, Status: "online"}
	if err := database.GetDB().Create(agent).Error; err != nil {
		t.Fatal(err)
	}
	master := &model.Inbound{
		Tag: "edge-hk-in-443-tcp", Enable: true, Port: 443, Protocol: model.VLESS, NodeID: &agent.Id,
		Settings:       `{"clients":[],"decryption":"none"}`,
		StreamSettings: `{"network":"tcp","security":"reality"}`,
	}
	if err := database.GetDB().Create(master).Error; err != nil {
		t.Fatal(err)
	}
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set("I18n", func(_ locale.I18nType, key string, _ ...string) string { return key })
		c.Next()
	})
	NewInboundController(engine.Group("/panel/api/inbounds"))

	reply := postJSON(t, engine, "/panel/api/inbounds/"+strconv.Itoa(master.Id)+"/fallbacks",
		map[string]any{"fallbacks": []map[string]any{{"dest": "127.0.0.1:8080"}}})
	if !reply.Success {
		t.Fatalf("save fallbacks: %s", reply.Msg)
	}
	stored, err := (&service.FallbackService{}).GetByMaster(master.Id)
	if err != nil || len(stored) != 1 || stored[0].Dest != "127.0.0.1:8080" {
		t.Fatalf("stored fallbacks = %+v (err %v), want only the posted dest 127.0.0.1:8080", stored, err)
	}
	select {
	case got := <-hub.Nudges():
		if got != agent.Id {
			t.Fatalf("saving the fallbacks nudged node %d, want agent node %d", got, agent.Id)
		}
	case <-time.After(time.Second):
		t.Fatalf("saving an agent master's fallbacks never nudged agent node %d, so they wait for its next sync tick", agent.Id)
	}
}
