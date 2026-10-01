package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/op/go-logging"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	xuilogger "github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/locale"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service/probetest"
)

func newProbeTestEngine(t *testing.T) *gin.Engine {
	t.Helper()
	xuilogger.InitLogger(logging.ERROR)
	gin.SetMode(gin.TestMode)
	dbDir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dbDir)
	dbtest.InitDB(t, filepath.Join(dbDir, "x-ui.db"))

	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set("I18n", func(_ locale.I18nType, key string, _ ...string) string { return key })
		c.Next()
	})
	NewProbeController(engine.Group("/panel/api/probe"))
	return engine
}

func getProbe(t *testing.T, engine *gin.Engine, path string) apiReply {
	t.Helper()
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	var reply apiReply
	if err := json.Unmarshal(w.Body.Bytes(), &reply); err != nil || w.Code != http.StatusOK {
		t.Fatalf("GET %s: status %d, body %s", path, w.Code, w.Body.String())
	}
	return reply
}

// saveProbeSettings points the probe at a fake Lite; saving also empties the
// answer cached by an earlier test of this package.
func saveProbeSettings(t *testing.T, engine *gin.Engine, settings service.ProbeSettings) {
	t.Helper()
	if reply := postJSON(t, engine, "/panel/api/probe/settings", settings); !reply.Success {
		t.Fatalf("save probe settings %+v: %s", settings, reply.Msg)
	}
}

// The page polls this route before anything is configured: it must answer with
// an empty array (the generated schema rejects null) and without trying Lite.
func TestProbeServersIsEmptyAndQuietUntilALiteAddressIsSet(t *testing.T) {
	engine := newProbeTestEngine(t)

	reply := getProbe(t, engine, "/panel/api/probe/servers")

	const want = `{"configured":false,"publicUrl":"","fetchedAt":0,"stale":false,"error":"","servers":[]}`
	if !reply.Success || string(reply.Obj) != want {
		t.Fatalf("servers = success %v, obj %s\nwant %s", reply.Success, reply.Obj, want)
	}
}

// A Lite that is down is a state of the page, not a failed request: the error
// travels next to the (here empty) server array.
func TestProbeServersReportsAFetchErrorInsideASuccessfulAnswer(t *testing.T) {
	engine := newProbeTestEngine(t)
	lite := probetest.NewLite(t)
	lite.Override(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) })
	saveProbeSettings(t, engine, service.ProbeSettings{URL: lite.URL, PublicURL: "https://probe.example.com"})

	reply := getProbe(t, engine, "/panel/api/probe/servers")

	const want = `{"configured":true,"publicUrl":"https://probe.example.com","fetchedAt":0,"stale":false,"error":"Lite answered HTTP 502","servers":[]}`
	if !reply.Success || string(reply.Obj) != want {
		t.Fatalf("servers = success %v, obj %s\nwant %s", reply.Success, reply.Obj, want)
	}
}

func TestProbeServersListsEveryLiteServerWithItsLinkAndPingsAsAnArray(t *testing.T) {
	engine := newProbeTestEngine(t)
	lite := probetest.NewLite(t)
	lite.Answer(`{"uuid-a":{"name":"lite a","region":"🇭🇰","weight":1},"uuid-b":{"name":"lite b","weight":2}}`, `{}`)
	saveProbeSettings(t, engine, service.ProbeSettings{URL: lite.URL})
	if reply := postJSON(t, engine, "/panel/api/probe/links", service.ProbeLinksInput{
		Links: []service.ProbeLinkInput{{NodeId: 0, ServerId: "uuid-b"}},
	}); !reply.Success {
		t.Fatalf("link the master: %s", reply.Msg)
	}

	reply := getProbe(t, engine, "/panel/api/probe/servers")

	var overview service.ProbeOverview
	if err := json.Unmarshal(reply.Obj, &overview); err != nil {
		t.Fatalf("decode %s: %v", reply.Obj, err)
	}
	if len(overview.Servers) != 2 || overview.Servers[0].Name != "lite a" || overview.Servers[0].Linked ||
		overview.Servers[1].Name != "lite b" || !overview.Servers[1].Linked || overview.Error != "" {
		t.Fatalf("servers = %+v, want lite a (not linked) then lite b (linked to the master)", overview)
	}
	if n := strings.Count(string(reply.Obj), `"pings":[]`); n != 2 {
		t.Fatalf("%d servers carry \"pings\":[] in %s, want both", n, reply.Obj)
	}
}

// The address is what the panel will connect to from inside the host, so the
// route must refuse anything the service's rule refuses and store nothing.
func TestProbeSettingsRejectAnAddressThatIsNotLoopback(t *testing.T) {
	engine := newProbeTestEngine(t)

	reply := postJSON(t, engine, "/panel/api/probe/settings", service.ProbeSettings{URL: "http://lite.example.com:27777"})
	const wantMsg = "somethingWentWrong (the Lite address must be a loopback URL such as http://127.0.0.1:27777)"
	if reply.Success || reply.Msg != wantMsg {
		t.Fatalf("save = success %v, msg %q; want failure with %q", reply.Success, reply.Msg, wantMsg)
	}
	if stored := getProbe(t, engine, "/panel/api/probe/settings"); string(stored.Obj) != `{"url":"","publicUrl":""}` {
		t.Fatalf("settings after a rejected save = %s, want both empty", stored.Obj)
	}
}

func TestProbeSettingsRoundTripAsStored(t *testing.T) {
	engine := newProbeTestEngine(t)

	saved := postJSON(t, engine, "/panel/api/probe/settings", map[string]string{
		"url": "http://127.0.0.1:27777/", "publicUrl": "https://probe.example.com",
	})
	const want = `{"url":"http://127.0.0.1:27777","publicUrl":"https://probe.example.com"}`
	if !saved.Success || string(saved.Obj) != want {
		t.Fatalf("save = success %v, obj %s; want %s", saved.Success, saved.Obj, want)
	}
	if stored := getProbe(t, engine, "/panel/api/probe/settings"); string(stored.Obj) != want {
		t.Fatalf("settings read back = %s, want %s", stored.Obj, want)
	}
}

// The frontend must send JSON explicitly; a form-encoded body (its default)
// has to fail loudly instead of saving empty settings.
func TestProbeSettingsRefuseABodyThatIsNotJSON(t *testing.T) {
	engine := newProbeTestEngine(t)
	saveProbeSettings(t, engine, service.ProbeSettings{URL: "http://127.0.0.1:27777"})

	req := httptest.NewRequest(http.MethodPost, "/panel/api/probe/settings", strings.NewReader("url=&publicUrl="))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	var reply apiReply
	if err := json.Unmarshal(w.Body.Bytes(), &reply); err != nil || reply.Success {
		t.Fatalf("form-encoded save = %s, want a failure", w.Body.String())
	}
	if stored := getProbe(t, engine, "/panel/api/probe/settings"); string(stored.Obj) != `{"url":"http://127.0.0.1:27777","publicUrl":""}` {
		t.Fatalf("settings after a form-encoded save = %s, want them unchanged", stored.Obj)
	}
}

func TestProbeLinksRoundTripThroughTheRoutes(t *testing.T) {
	engine := newProbeTestEngine(t)
	lite := probetest.NewLite(t)
	lite.Answer(`{"uuid-a":{"name":"lite a"},"uuid-b":{"name":"lite b"}}`, `{}`)
	saveProbeSettings(t, engine, service.ProbeSettings{URL: lite.URL})
	node := &model.Node{Name: "edge-hk", Remark: "Hong Kong", Address: "203.0.113.11", Kind: model.NodeKindAgent, Enable: true}
	if err := database.GetDB().Create(node).Error; err != nil {
		t.Fatalf("seed node: %v", err)
	}

	saved := postJSON(t, engine, "/panel/api/probe/links", map[string]any{"links": []map[string]any{
		{"nodeId": 0, "serverId": "uuid-a"},
		{"nodeId": node.Id, "serverId": "uuid-b"},
	}})
	want := []service.ProbeLinkView{
		{NodeId: 0, ServerId: "uuid-a", ServerName: "lite a"},
		{NodeId: node.Id, NodeName: "Hong Kong", Address: "203.0.113.11", ServerId: "uuid-b", ServerName: "lite b"},
	}
	wantJSON, _ := json.Marshal(want)
	if !saved.Success || string(saved.Obj) != string(wantJSON) {
		t.Fatalf("save links = success %v, obj %s\nwant %s", saved.Success, saved.Obj, wantJSON)
	}
	if listed := getProbe(t, engine, "/panel/api/probe/links"); string(listed.Obj) != string(wantJSON) {
		t.Fatalf("links read back = %s\nwant %s", listed.Obj, wantJSON)
	}

	rejected := postJSON(t, engine, "/panel/api/probe/links", map[string]any{"links": []map[string]any{
		{"nodeId": 0, "serverId": "uuid-b"},
		{"nodeId": node.Id, "serverId": "uuid-b"},
	}})
	if rejected.Success || rejected.Msg != "somethingWentWrong (server uuid-b is linked to more than one node)" {
		t.Fatalf("an inconsistent set = success %v, msg %q; want the service's refusal", rejected.Success, rejected.Msg)
	}
	if listed := getProbe(t, engine, "/panel/api/probe/links"); string(listed.Obj) != string(wantJSON) {
		t.Fatalf("links after the rejected save = %s\nwant them unchanged: %s", listed.Obj, wantJSON)
	}
}

// With no node and nothing linked the modal still gets the master row, and the
// answer is an array.
func TestProbeLinksListTheMasterEvenWhenNothingIsLinked(t *testing.T) {
	engine := newProbeTestEngine(t)

	listed := getProbe(t, engine, "/panel/api/probe/links")

	const want = `[{"nodeId":0,"nodeName":"","address":"","serverId":"","serverName":""}]`
	if !listed.Success || string(listed.Obj) != want {
		t.Fatalf("links = success %v, obj %s; want %s", listed.Success, listed.Obj, want)
	}
}
