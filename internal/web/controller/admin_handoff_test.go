package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/web/session"
)

func TestAdminHandoffRequiresTheBoundBrowserAndCreatesASessionOnlyOnce(t *testing.T) {
	router := newProbeTestEngine(t)
	router.Use(sessions.Sessions("3x-ui", cookie.NewStore([]byte("01234567890123456789012345678901"))))
	NewIndexController(router.Group("/"))
	router.GET("/who", func(c *gin.Context) {
		user := session.GetLoginUser(c)
		if user == nil {
			c.Status(http.StatusUnauthorized)
			return
		}
		c.JSON(http.StatusOK, gin.H{"id": user.Id})
	})
	if err := (&service.SettingService{}).SetTwoFactorEnable(true); err != nil {
		t.Fatal(err)
	}
	var user model.User
	if err := database.GetDB().First(&user).Error; err != nil {
		t.Fatal(err)
	}
	token, err := service.IssueAdminHandoff(&user)
	if err != nil {
		t.Fatal(err)
	}
	post := func(bound string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/portal-admin", strings.NewReader(`{"token":"`+token+`"}`))
		req.Header.Set("Content-Type", "application/json")
		if bound != "" {
			req.AddCookie(&http.Cookie{Name: service.AdminHandoffCookie, Value: bound})
		}
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		return res
	}
	if res := post(""); res.Code != http.StatusUnauthorized {
		t.Fatalf("unbound browser = %d, want 401", res.Code)
	}
	if res := post("another-browser"); res.Code != http.StatusUnauthorized {
		t.Fatalf("wrong browser = %d, want 401", res.Code)
	}
	res := post(token)
	if res.Code != http.StatusOK {
		t.Fatalf("exchange = %d %s", res.Code, res.Body)
	}
	req := httptest.NewRequest(http.MethodGet, "/who", nil)
	for _, c := range res.Result().Cookies() {
		req.AddCookie(c)
	}
	who := httptest.NewRecorder()
	router.ServeHTTP(who, req)
	var loggedIn struct {
		Id int `json:"id"`
	}
	if err := json.Unmarshal(who.Body.Bytes(), &loggedIn); err != nil || who.Code != http.StatusOK || loggedIn.Id != user.Id {
		t.Fatalf("admin session = %d %s (%v)", who.Code, who.Body, err)
	}
	if replay := post(token); replay.Code != http.StatusUnauthorized {
		t.Fatalf("replay = %d, want 401", replay.Code)
	}
}
