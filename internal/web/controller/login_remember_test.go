package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/eventbus"
	"github.com/mhsanaei/3x-ui/v3/internal/web/locale"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service/tgbot"
	"github.com/mhsanaei/3x-ui/v3/internal/web/session"
)

// "Remember me" gives the browser a 30-day cookie that survives a credential change made
// from it, and a successful sign-in remembers its network; a failed one does not.
func TestLoginRemembersTheBrowserAndTheNetwork(t *testing.T) {
	newControllerTestDB(t)
	base := sessions.Options{Path: "/", HttpOnly: true, MaxAge: 360 * 60}
	store := cookie.NewStore([]byte("01234567890123456789012345678901"))
	store.Options(base)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("I18n", func(_ locale.I18nType, key string, _ ...string) string { return key })
	})
	router.Use(sessions.Sessions("3x-ui", store), session.KeepRemembered(base))
	NewIndexController(router.Group("/"))
	NewSettingController(router.Group("/panel"))

	var jar []*http.Cookie
	call := func(method, path, body, from, csrf string) (apiEnvelope, int) {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(session.CSRFHeaderName, csrf)
		req.RemoteAddr = from + ":40000"
		for _, c := range jar {
			req.AddCookie(c)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		var env apiEnvelope
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			t.Fatalf("%s: %d %s", path, rec.Code, rec.Body)
		}
		age := 0
		for _, c := range rec.Result().Cookies() {
			if c.Name == "3x-ui" {
				jar, age = []*http.Cookie{c}, c.MaxAge
			}
		}
		return env, age
	}
	post := func(path, body, from string) (apiEnvelope, int) {
		t.Helper()
		env, _ := call(http.MethodGet, "/csrf-token", "", from, "")
		var token string
		if err := json.Unmarshal(env.Obj, &token); err != nil || token == "" {
			t.Fatalf("csrf token: %s (%v)", env.Obj, err)
		}
		return call(http.MethodPost, path, body, from, token)
	}
	remembered := int(session.RememberMaxAge / time.Second)
	bus, events := eventbus.New(16), make(chan *eventbus.LoginEventData, 16)
	bus.Subscribe("test", func(e eventbus.Event) { events <- e.Data.(*eventbus.LoginEventData) })
	prevBus := tgbot.EventBus
	tgbot.EventBus = bus
	t.Cleanup(func() { tgbot.EventBus = prevBus; bus.Stop() })
	expectEvent := func(status string, newIP bool) {
		t.Helper()
		select {
		case e := <-events:
			if e.Status != status || e.NewIP != newIP {
				t.Fatalf("login event %s new IP %v, want %s new IP %v", e.Status, e.NewIP, status, newIP)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("no %s login event", status)
		}
	}

	if env, _ := post("/login", `{"username":"admin","password":"wrong"}`, "203.0.113.9"); env.Success {
		t.Fatal("a wrong password signed in")
	}
	expectEvent("fail", true)
	if service.PanelLoginKnown("203.0.113.9") {
		t.Fatal("a failed sign-in made its network known")
	}
	env, age := post("/login", `{"username":"admin","password":"admin","rememberMe":true}`, "198.51.100.7")
	if !env.Success || age != remembered {
		t.Fatalf("remembered sign-in: success %v, cookie max-age %d, want %d", env.Success, age, remembered)
	}
	expectEvent("success", true)
	if !service.PanelLoginKnown("198.51.100.7") {
		t.Fatal("a successful sign-in left its network unknown")
	}
	env, age = post("/panel/setting/updateUser",
		`{"oldUsername":"admin","oldPassword":"admin","newUsername":"pigger","newPassword":"s3cret-pass"}`, "198.51.100.7")
	if !env.Success || age != remembered {
		t.Fatalf("credential change: success %v (%s), cookie max-age %d, want %d", env.Success, env.Msg, age, remembered)
	}
	jar = nil
	if env, age = post("/login", `{"username":"pigger","password":"s3cret-pass"}`, "198.51.100.7"); !env.Success || age != base.MaxAge {
		t.Fatalf("plain sign-in: success %v, cookie max-age %d, want the session setting's %d", env.Success, age, base.MaxAge)
	}
	expectEvent("success", false)
	post("/login", `{"username":"pigger","password":"typo"}`, "198.51.100.7")
	expectEvent("fail", false)
}
