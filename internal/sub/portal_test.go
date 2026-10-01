package sub

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

const portalTestStream = `{"network":"tcp","security":"none"}`

// seedPortal gives subscriptions s1 and s2 one client each (pa@e, pb@e) and lets
// pa@e sign in with alpha-pass.
func seedPortal(t *testing.T) (*gin.Engine, *SUBController) {
	t.Helper()
	seedSubDB(t)
	seedSubInbound(t, "s1", "pa", 4491, 1, portalTestStream)
	seedSubInbound(t, "s2", "pb", 4492, 1, portalTestStream)
	if err := (&service.ClientPortalService{}).SetPassword("pa@e", "alpha-pass"); err != nil {
		t.Fatalf("set portal password: %v", err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	return router, NewSUBController(router.Group("/"))
}

// portalTestToday is today's day number in the panel time zone the portal uses.
func portalTestToday() int {
	loc, err := (&service.SettingService{}).GetTimeLocation()
	if err != nil {
		loc = time.Local
	}
	now := time.Now().In(loc)
	return now.Year()*10000 + int(now.Month())*100 + now.Day()
}

func portalRequest(router *gin.Engine, method, path, body, ip string, cookie *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.RemoteAddr = ip + ":40000"
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func portalLogin(router *gin.Engine, user, pass, ip string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{"username": user, "password": pass})
	return portalRequest(router, http.MethodPost, "/sub/portal/login", string(body), ip, nil)
}

func sessionCookie(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == portalCookieName {
			return c
		}
	}
	t.Fatalf("no %s cookie in %v", portalCookieName, rec.Header().Values("Set-Cookie"))
	return nil
}

func clientIdOf(t *testing.T, email string) int {
	t.Helper()
	var rec model.ClientRecord
	if err := database.GetDB().Where("email = ?", email).First(&rec).Error; err != nil {
		t.Fatalf("load %s: %v", email, err)
	}
	return rec.Id
}

func TestPortalLoginGivesAScopedSessionForThatClientOnly(t *testing.T) {
	router, _ := seedPortal(t)
	db := database.GetDB()
	for _, d := range []model.ClientDailyTraffic{
		{Email: "pa@e", Day: portalTestToday(), Up: 7, Down: 70},
		{Email: "pb@e", Day: portalTestToday(), Up: 9000, Down: 9000},
	} {
		if err := db.Create(&d).Error; err != nil {
			t.Fatalf("seed daily: %v", err)
		}
	}

	login := portalLogin(router, "pa@e", "alpha-pass", "198.51.100.1")
	if login.Code != http.StatusOK {
		t.Fatalf("login = %d %s, want 200", login.Code, login.Body)
	}
	cookie := sessionCookie(t, login)
	if !cookie.HttpOnly || cookie.Path != "/sub/portal" || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("cookie = %+v, want HttpOnly, SameSite=Lax, scoped to /sub/portal", cookie)
	}

	res := portalRequest(router, http.MethodGet, "/sub/portal/data", "", "198.51.100.1", cookie)
	if res.Code != http.StatusOK {
		t.Fatalf("data = %d %s, want 200", res.Code, res.Body)
	}
	var data struct {
		Email string `json:"email"`
		Page  struct {
			SId string `json:"sId"`
		} `json:"page"`
		Daily []service.TrafficDay `json:"daily"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &data); err != nil {
		t.Fatalf("decode %s: %v", res.Body, err)
	}
	last := data.Daily[len(data.Daily)-1]
	if data.Email != "pa@e" || data.Page.SId != "s1" || last.Up != 7 || last.Down != 70 {
		t.Fatalf("data = %+v, want pa@e's own subscription and traffic only", data)
	}
	if strings.Contains(res.Body.String(), "pb@e") {
		t.Fatalf("pa@e's portal data mentions pb@e:\n%s", res.Body)
	}
}

func TestPortalRejectsTamperedAndExpiredSessions(t *testing.T) {
	router, a := seedPortal(t)
	cookie := sessionCookie(t, portalLogin(router, "pa@e", "alpha-pass", "198.51.100.1"))

	paId, pbId := strconv.Itoa(clientIdOf(t, "pa@e")), strconv.Itoa(clientIdOf(t, "pb@e"))
	forged := *cookie
	forged.Value = strings.Replace(cookie.Value, "."+paId+".", "."+pbId+".", 1)
	if forged.Value == cookie.Value {
		t.Fatalf("cookie %q does not carry the client id %s where expected", cookie.Value, paId)
	}
	if res := portalRequest(router, http.MethodGet, "/sub/portal/data", "", "198.51.100.1", &forged); res.Code != http.StatusUnauthorized {
		t.Fatalf("forged session = %d %s, want 401", res.Code, res.Body)
	}

	session, ok := a.readPortalSession(cookie.Value, time.Now())
	if !ok {
		t.Fatalf("own cookie %q does not verify", cookie.Value)
	}
	session.Expires = time.Now().Add(-time.Minute).Unix()
	expired, err := a.signPortalSession(session)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	stale := &http.Cookie{Name: portalCookieName, Value: expired}
	if res := portalRequest(router, http.MethodGet, "/sub/portal/data", "", "198.51.100.1", stale); res.Code != http.StatusUnauthorized {
		t.Fatalf("expired session = %d %s, want 401", res.Code, res.Body)
	}

	// Only the signature stops a stolen, expired cookie from being re-dated.
	oldExpiry := "." + strconv.FormatInt(session.Expires, 10) + "."
	newExpiry := "." + strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10) + "."
	redated := &http.Cookie{Name: portalCookieName, Value: strings.Replace(expired, oldExpiry, newExpiry, 1)}
	if redated.Value == expired {
		t.Fatalf("cookie %q does not carry the expiry where expected", expired)
	}
	if res := portalRequest(router, http.MethodGet, "/sub/portal/data", "", "198.51.100.1", redated); res.Code != http.StatusUnauthorized {
		t.Fatalf("re-dated session = %d %s, want 401", res.Code, res.Body)
	}
}

func TestPortalSessionEndsWhenTheAdminChangesThePassword(t *testing.T) {
	router, _ := seedPortal(t)
	cookie := sessionCookie(t, portalLogin(router, "pa@e", "alpha-pass", "198.51.100.1"))
	if err := (&service.ClientPortalService{}).SetPassword("pa@e", "beta-pass"); err != nil {
		t.Fatalf("change password: %v", err)
	}
	if res := portalRequest(router, http.MethodGet, "/sub/portal/data", "", "198.51.100.1", cookie); res.Code != http.StatusUnauthorized {
		t.Fatalf("session after a password change = %d, want 401", res.Code)
	}
}

func TestPortalLoginIsRateLimitedPerAddress(t *testing.T) {
	router, _ := seedPortal(t)
	for i := range 5 {
		if res := portalLogin(router, "pa@e", "guess-"+strconv.Itoa(i), "203.0.113.9"); res.Code != http.StatusUnauthorized {
			t.Fatalf("wrong password %d = %d, want 401", i+1, res.Code)
		}
	}
	if res := portalLogin(router, "pa@e", "alpha-pass", "203.0.113.9"); res.Code != http.StatusTooManyRequests {
		t.Fatalf("right password from a blocked address = %d %s, want 429", res.Code, res.Body)
	}
	if res := portalLogin(router, "pa@e", "alpha-pass", "203.0.113.10"); res.Code != http.StatusOK {
		t.Fatalf("right password from another address = %d %s, want 200", res.Code, res.Body)
	}
}

func TestPortalLogoutClearsTheSession(t *testing.T) {
	router, _ := seedPortal(t)
	cookie := sessionCookie(t, portalLogin(router, "pa@e", "alpha-pass", "198.51.100.1"))
	res := portalRequest(router, http.MethodPost, "/sub/portal/logout", "", "198.51.100.1", cookie)
	cleared := sessionCookie(t, res)
	if res.Code != http.StatusOK || cleared.Value != "" || cleared.MaxAge >= 0 || cleared.Path != "/sub/portal" {
		t.Fatalf("logout = %d with cookie %+v, want 200 and the cookie deleted", res.Code, cleared)
	}
}

// Behind a proxy the address comes from forwarded headers a client can vary, so
// a guesser rotating addresses still meets a cap per username.
func TestPortalLoginCapsGuessesPerUsernameAcrossAddresses(t *testing.T) {
	router, _ := seedPortal(t)
	for i := range 10 {
		ip := "203.0.113." + strconv.Itoa(20+i)
		if res := portalLogin(router, "pa@e", "guess-"+strconv.Itoa(i), ip); res.Code != http.StatusUnauthorized {
			t.Fatalf("guess %d from %s = %d, want 401", i+1, ip, res.Code)
		}
	}
	if res := portalLogin(router, "pa@e", "alpha-pass", "203.0.113.99"); res.Code != http.StatusTooManyRequests {
		t.Fatalf("right password after 10 guesses from rotating addresses = %d %s, want 429", res.Code, res.Body)
	}
	if res := portalLogin(router, "pb@e", "anything", "203.0.113.99"); res.Code != http.StatusUnauthorized {
		t.Fatalf("another username from the same address = %d, want 401 (not blocked)", res.Code)
	}
}
