package session

import (
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"
)

// RememberMaxAge is how long "remember me" keeps the admin signed in; the cookie
// codec rejects sessions older than 30 days, so it cannot be longer.
const RememberMaxAge = 30 * 24 * time.Hour

const (
	rememberedAtKey      = "LOGIN_REMEMBERED_AT"
	rememberRefreshAfter = 24 * time.Hour
	cookieAgesKey        = "session_cookie_ages"
)

type cookieAges struct {
	base, remembered sessions.Options
}

// KeepRemembered makes every save of a remembered session keep RememberMaxAge;
// any other session gets base, the store's options.
func KeepRemembered(base sessions.Options) gin.HandlerFunc {
	ages := cookieAges{base: base, remembered: base}
	ages.remembered.MaxAge = int(RememberMaxAge / time.Second)
	return func(c *gin.Context) {
		c.Set(cookieAgesKey, ages)
		if s := sessions.Default(c); s.Get(rememberedAtKey) != nil {
			s.Options(ages.remembered)
		}
		c.Next()
	}
}

// Remembered says whether this browser's login was made with "remember me".
func Remembered(c *gin.Context) bool {
	return sessions.Default(c).Get(rememberedAtKey) != nil
}

func setRemembered(c *gin.Context, s sessions.Session, remember bool, now time.Time) {
	value, _ := c.Get(cookieAgesKey)
	ages, ok := value.(cookieAges)
	if !ok {
		return
	}
	if remember {
		s.Set(rememberedAtKey, now.Unix())
		s.Options(ages.remembered)
		return
	}
	s.Delete(rememberedAtKey)
	s.Options(ages.base)
}

// RefreshRemembered re-issues a remembered login's cookie after a day of use, so the
// 30 days run from the last visit rather than from signing in.
func RefreshRemembered(c *gin.Context, now time.Time) {
	s := sessions.Default(c)
	at, ok := s.Get(rememberedAtKey).(int64)
	if !ok || now.Sub(time.Unix(at, 0)) < rememberRefreshAfter {
		return
	}
	s.Set(rememberedAtKey, now.Unix())
	if err := s.Save(); err != nil {
		logger.Warning("session: failed to refresh a remembered login:", err)
	}
}
