package controller

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/mhsanaei/3x-ui/v3/internal/agentproto"
	"github.com/mhsanaei/3x-ui/v3/internal/agentproto/agenttest"
	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/locale"
	"github.com/mhsanaei/3x-ui/v3/internal/web/runtime"
	"github.com/mhsanaei/3x-ui/v3/internal/web/session"
)

// The body nests the inbound because model.Inbound decodes itself: flattened,
// planIds and publicPort would be dropped without a word.
func TestInboundGenerateRouteBindsTheWholeRequest(t *testing.T) {
	newHostTestDB(t)
	prevManager := runtime.GetManager()
	runtime.SetManager(runtime.NewManager(runtime.LocalDeps{APIPort: func() int { return 0 }, SetNeedRestart: func() {}}))
	t.Cleanup(func() { runtime.SetManager(prevManager) })
	hub := runtime.NewAgentHub()
	prevHub := runtime.GetAgentHub()
	runtime.SetAgentHub(hub)
	t.Cleanup(func() { runtime.SetAgentHub(prevHub) })

	host := &model.Node{Name: "edge-hk", Kind: model.NodeKindAgent, Address: "203.0.113.53", Enable: true, Status: "online"}
	if err := database.GetDB().Create(host).Error; err != nil {
		t.Fatal(err)
	}
	url := agenttest.Server(t, func(conn *websocket.Conn) { hub.Attach(host.Id, conn) })
	fake := agenttest.Dial(t, url, agentproto.Hello{AgentVersion: "v1"})
	for deadline := time.Now().Add(3 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		if _, ok := hub.Session(host.Id); ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("hub never registered the agent")
		}
	}
	fake.AnswerApplies(true)

	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set("I18n", func(_ locale.I18nType, key string, _ ...string) string { return key })
		session.SetAPIAuthUser(c, &model.User{Id: 1})
		c.Next()
	})
	NewInboundController(engine.Group("/panel/api/inbounds"))

	body := func(publicPort int) map[string]any {
		return map[string]any{
			"inbound": map[string]any{
				"remark": "香港-Edge-2", "enable": true, "port": 81, "protocol": "vless", "nodeId": host.Id,
				"settings":       map[string]any{"clients": []any{}, "decryption": "none"},
				"streamSettings": map[string]any{"network": "tcp", "security": "none"},
				"sniffing":       map[string]any{},
			},
			"planIds":    []int{},
			"publicPort": publicPort,
		}
	}

	if reply := postJSON(t, engine, "/panel/api/inbounds/generate", body(70000)); reply.Success || reply.Msg != "request body failed validation" {
		t.Fatalf("public port 70000: success %v, msg %q, want a validation failure", reply.Success, reply.Msg)
	}
	var inbounds int64
	database.GetDB().Model(&model.Inbound{}).Count(&inbounds)
	if inbounds != 0 {
		t.Fatalf("%d inbounds written for a request that failed validation", inbounds)
	}

	reply := postJSON(t, engine, "/panel/api/inbounds/generate", body(20443))
	if !reply.Success {
		t.Fatalf("generate: %s", reply.Msg)
	}
	var created model.Inbound
	if err := json.Unmarshal(reply.Obj, &created); err != nil {
		t.Fatalf("reply obj %s: %v", reply.Obj, err)
	}
	var stored model.Inbound
	if err := database.GetDB().First(&stored, created.Id).Error; err != nil {
		t.Fatal(err)
	}
	if !stored.Enable || stored.NodeID == nil || *stored.NodeID != host.Id || stored.Port != 81 {
		t.Fatalf("stored node: enable %v, node %v, port %d; want enabled on agent %d at 81", stored.Enable, stored.NodeID, stored.Port, host.Id)
	}
	var entry model.Host
	if err := database.GetDB().Where("inbound_id = ?", created.Id).First(&entry).Error; err != nil || entry.Port != 20443 {
		t.Fatalf("entry %+v (err %v): publicPort 20443 never reached the service", entry, err)
	}
}
