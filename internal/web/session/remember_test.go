package session

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// A remembered login's cookie lasts RememberMaxAge through later saves, such as the CSRF
// token's, and is re-issued after a day of use; signing in unremembered goes back to the
// store's age, which the panel's session setting sets.
func TestRememberedLoginKeepsItsCookieFor30Days(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const storeAge = 360 * 60
	base := sessions.Options{Path: "/", HttpOnly: true, MaxAge: storeAge}
	store := cookie.NewStore([]byte("01234567890123456789012345678901"))
	store.Options(base)
	router := gin.New()
	router.Use(sessions.Sessions(sessionCookieName, store), KeepRemembered(base))
	start := time.Now()
	router.GET("/login", func(c *gin.Context) {
		if err := SetLoginUser(c, &model.User{Id: 7}, c.Query("remember") == "1"); err != nil {
			t.Fatal(err)
		}
	})
	router.GET("/csrf", func(c *gin.Context) {
		if _, err := EnsureCSRFToken(c); err != nil {
			t.Fatal(err)
		}
	})
	router.GET("/visit", func(c *gin.Context) {
		elapsed, _ := time.ParseDuration(c.Query("after"))
		RefreshRemembered(c, start.Add(elapsed))
	})

	var jar []*http.Cookie
	cookieAge := func(path string) (int, bool) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		for _, c := range jar {
			req.AddCookie(c)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		for _, c := range rec.Result().Cookies() {
			if c.Name == sessionCookieName {
				jar = []*http.Cookie{c}
				return c.MaxAge, true
			}
		}
		return 0, false
	}
	remembered := int(RememberMaxAge / time.Second)
	steps := []struct {
		path    string
		wantAge int // 0: no cookie sent
	}{
		{"/login?remember=1", remembered},
		{"/csrf", remembered},
		{"/visit?after=1h", 0},
		{"/visit?after=25h", remembered},
		{"/login", storeAge},
		{"/visit?after=49h", 0},
	}
	for _, step := range steps {
		age, sent := cookieAge(step.path)
		if step.wantAge == 0 && sent {
			t.Fatalf("%s re-issued the cookie (max-age %d)", step.path, age)
		}
		if step.wantAge != 0 && age != step.wantAge {
			t.Fatalf("%s: cookie max-age = %d (sent %v), want %d", step.path, age, sent, step.wantAge)
		}
	}
}
