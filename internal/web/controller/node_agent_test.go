package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/agentproto"
	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/runtime"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type apiReply struct {
	Success bool            `json:"success"`
	Msg     string          `json:"msg"`
	Obj     json.RawMessage `json:"obj"`
}

func postJSON(t *testing.T, engine *gin.Engine, path string, body any) apiReply {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	var reply apiReply
	if err := json.Unmarshal(w.Body.Bytes(), &reply); err != nil {
		t.Fatalf("%s: status %d, body %s", path, w.Code, w.Body.String())
	}
	return reply
}

// An agent dials the panel only after it is installed with the secret minted for
// its node, so saving the node can never wait to reach it.
func TestNodeControllerSavesAgentsWithoutReachingThem(t *testing.T) {
	engine := newNodeCredentialTestEngine(t)

	reply := postJSON(t, engine, "/panel/api/nodes/add", map[string]any{
		"name": "lazycat", "kind": "agent", "address": "216.236.63.53", "enable": true,
	})
	if !reply.Success {
		t.Fatalf("add agent node: %s", reply.Msg)
	}
	var created model.Node
	if err := database.GetDB().Where("name = ?", "lazycat").First(&created).Error; err != nil {
		t.Fatal(err)
	}
	if !created.IsAgent() {
		t.Fatalf("stored kind = %q, want agent", created.Kind)
	}

	reply = postJSON(t, engine, "/panel/api/nodes/update/"+strconv.Itoa(created.Id), map[string]any{
		"name": "lazycat", "kind": "agent", "address": "216.236.63.54", "enable": true,
	})
	if !reply.Success {
		t.Fatalf("update agent node: %s", reply.Msg)
	}
}

// Converting a panel node that had an outbound bridge drops the bridge; the
// follow-up reachability probe must not then fail an update that already saved.
func TestNodeControllerConvertsABridgedPanelNodeToAnAgent(t *testing.T) {
	engine := newNodeCredentialTestEngine(t)
	panelNode := &model.Node{Name: "frontier", Scheme: "http", Address: "127.0.0.1", Port: 22606, ApiToken: "tok", Enable: true, OutboundTag: "warp"}
	if err := database.GetDB().Create(panelNode).Error; err != nil {
		t.Fatal(err)
	}
	reply := postJSON(t, engine, "/panel/api/nodes/update/"+strconv.Itoa(panelNode.Id), map[string]any{
		"name": "frontier", "kind": "agent", "address": "66.132.239.17", "enable": true,
	})
	if !reply.Success {
		t.Fatalf("convert to agent: %s", reply.Msg)
	}
}

func TestNodeControllerMintsAgentSecrets(t *testing.T) {
	engine := newNodeCredentialTestEngine(t)
	agent := &model.Node{Name: "lazycat", Kind: model.NodeKindAgent, Address: "216.236.63.53", Enable: true}
	panelNode := &model.Node{Name: "panel", Scheme: "https", Address: "example.com", Port: 2053, ApiToken: "tok", Enable: true}
	for _, n := range []*model.Node{agent, panelNode} {
		if err := database.GetDB().Create(n).Error; err != nil {
			t.Fatal(err)
		}
	}

	reply := postJSON(t, engine, "/panel/api/nodes/agentSecret/"+strconv.Itoa(agent.Id), nil)
	if !reply.Success {
		t.Fatalf("mint: %s", reply.Msg)
	}
	var minted service.AgentSecretView
	if err := json.Unmarshal(reply.Obj, &minted); err != nil {
		t.Fatal(err)
	}
	if n, err := (&service.NodeService{}).AgentNodeBySecret(minted.Secret); err != nil || n.Id != agent.Id {
		t.Fatalf("returned secret does not open the agent node: %v, %v", n, err)
	}

	if reply := postJSON(t, engine, "/panel/api/nodes/agentSecret/"+strconv.Itoa(panelNode.Id), nil); reply.Success {
		t.Fatal("a panel node must not get an agent secret")
	}
}

func agentConnectServer(t *testing.T) (*httptest.Server, *runtime.AgentHub) {
	t.Helper()
	newNodeCredentialTestEngine(t)
	hub := runtime.NewAgentHub()
	prev := runtime.GetAgentHub()
	runtime.SetAgentHub(hub)
	t.Cleanup(func() { runtime.SetAgentHub(prev) })
	engine := gin.New()
	NewAgentController(engine.Group("/base/"))
	srv := httptest.NewServer(engine)
	t.Cleanup(srv.Close)
	return srv, hub
}

func dialAgentEndpoint(srv *httptest.Server, secret string) (*websocket.Conn, *http.Response, error) {
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/base/" + agentproto.ConnectPath
	header := http.Header{}
	if secret != "" {
		header.Set("Authorization", "Bearer "+secret)
	}
	return websocket.DefaultDialer.Dial(url, header)
}

func TestAgentConnectRejectsUnknownSecrets(t *testing.T) {
	srv, hub := agentConnectServer(t)
	agent := &model.Node{Name: "off", Kind: model.NodeKindAgent, Address: "1.2.3.4", Enable: true}
	if err := database.GetDB().Create(agent).Error; err != nil {
		t.Fatal(err)
	}
	secret, err := (&service.NodeService{}).MintAgentSecret(agent.Id)
	if err != nil {
		t.Fatal(err)
	}
	if err := (&service.NodeService{}).SetEnable(agent.Id, false); err != nil {
		t.Fatal(err)
	}

	for name, s := range map[string]string{"none": "", "wrong": "not-a-secret", "disabled node": secret} {
		conn, resp, err := dialAgentEndpoint(srv, s)
		if err == nil {
			conn.Close()
			t.Fatalf("%s: connection accepted", name)
		}
		if resp == nil || resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("%s: response %v, want 401", name, resp)
		}
	}
	if len(hub.Connected()) != 0 {
		t.Fatalf("rejected agents reached the hub: %v", hub.Connected())
	}
}

func TestAgentConnectHandsTheSocketToTheHub(t *testing.T) {
	srv, hub := agentConnectServer(t)
	agent := &model.Node{Name: "lazycat", Kind: model.NodeKindAgent, Address: "1.2.3.4", Enable: true}
	if err := database.GetDB().Create(agent).Error; err != nil {
		t.Fatal(err)
	}
	secret, err := (&service.NodeService{}).MintAgentSecret(agent.Id)
	if err != nil {
		t.Fatal(err)
	}
	conn, _, err := dialAgentEndpoint(srv, secret)
	if err != nil {
		t.Fatalf("dial with the node's secret: %v", err)
	}
	defer conn.Close()
	if err := conn.WriteJSON(agentproto.Message{Type: agentproto.TypeHello, Hello: &agentproto.Hello{AgentVersion: "v1"}}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		if st, ok := hub.Session(agent.Id); ok && st.Hello.AgentVersion == "v1" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the agent's connection never reached the hub under its node")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
