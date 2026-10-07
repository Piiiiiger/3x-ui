package controller

import (
	"crypto/subtle"
	"net/http"
	"strings"
	"text/template"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/util/loginlimit"
	"github.com/mhsanaei/3x-ui/v3/internal/web/middleware"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service/panel"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service/tgbot"
	"github.com/mhsanaei/3x-ui/v3/internal/web/session"

	"github.com/gin-gonic/gin"
)

var defaultLoginLimiter = loginlimit.New(loginlimit.MaxFailures, loginlimit.Window, loginlimit.Cooldown)

// LoginForm represents the login request structure.
type LoginForm struct {
	Username      string `json:"username" form:"username"`
	Password      string `json:"password" form:"password"`
	TwoFactorCode string `json:"twoFactorCode" form:"twoFactorCode"`
	RememberMe    bool   `json:"rememberMe" form:"rememberMe"`
}

// IndexController handles the main index and login-related routes.
type IndexController struct {
	BaseController

	settingService service.SettingService
	userService    panel.UserService
	tgbot          tgbot.Tgbot
}

// NewIndexController creates a new IndexController and initializes its routes.
func NewIndexController(g *gin.RouterGroup) *IndexController {
	a := &IndexController{}
	a.initRouter(g)
	return a
}

// initRouter sets up the routes for index, login, logout, and two-factor authentication.
func (a *IndexController) initRouter(g *gin.RouterGroup) {
	g.GET("/", a.index)
	g.GET("/csrf-token", a.csrfToken)

	g.POST("/login", middleware.CSRFMiddleware(), a.login)
	g.POST("/logout", middleware.CSRFMiddleware(), a.logout)
	g.POST("/getTwoFactorEnable", middleware.CSRFMiddleware(), a.getTwoFactorEnable)
	g.POST("/portal-admin", a.portalAdminHandoff)
}

func (a *IndexController) portalAdminHandoff(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	var form struct {
		Token string `json:"token"`
	}
	if err := c.ShouldBindJSON(&form); err != nil || form.Token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid"})
		return
	}
	bound, err := c.Cookie(service.AdminHandoffCookie)
	if err != nil || subtle.ConstantTimeCompare([]byte(bound), []byte(form.Token)) != 1 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	user, err := service.TakeAdminHandoff(form.Token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	if err := session.SetLoginUser(c, user, false); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server"})
		return
	}
	http.SetCookie(c.Writer, &http.Cookie{Name: service.AdminHandoffCookie, Path: "/", MaxAge: -1, HttpOnly: true, Secure: c.Request.TLS != nil, SameSite: http.SameSiteStrictMode})
	c.JSON(http.StatusOK, gin.H{"success": true, "redirect": c.GetString("base_path") + "panel/"})
}

// index handles the root route, redirecting logged-in users to the panel or showing the login page.
func (a *IndexController) index(c *gin.Context) {
	if session.IsLogin(c) {
		c.Header("Cache-Control", "no-store")
		c.Redirect(http.StatusTemporaryRedirect, c.GetString("base_path")+"panel/")
		return
	}
	if enabled, err := a.settingService.GetTwoFactorEnable(); err == nil && enabled {
		portal, err := a.settingService.GetSubURI()
		if err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		if portal == "" {
			portal, err = a.settingService.GetSubPath()
		}
		if err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.Redirect(http.StatusTemporaryRedirect, strings.TrimRight(portal, "/")+"/portal")
		return
	}
	serveDistPage(c, "login.html")
}

// login handles user authentication and session creation.
func (a *IndexController) login(c *gin.Context) {
	var form LoginForm

	if err := c.ShouldBind(&form); err != nil {
		pureJsonMsg(c, http.StatusOK, false, I18nWeb(c, "pages.login.toasts.invalidFormData"))
		return
	}
	if form.Username == "" {
		pureJsonMsg(c, http.StatusOK, false, I18nWeb(c, "pages.login.toasts.emptyUsername"))
		return
	}
	if form.Password == "" {
		pureJsonMsg(c, http.StatusOK, false, I18nWeb(c, "pages.login.toasts.emptyPassword"))
		return
	}

	remoteIP := getRemoteIp(c)
	safeUser := template.HTMLEscapeString(form.Username)
	timeStr := time.Now().Format("2006-01-02 15:04:05")
	if blockedUntil, ok := defaultLoginLimiter.Allow(remoteIP, form.Username); !ok {
		reason := "too many failed attempts"
		logger.Warningf("failed login: username=%q, IP=%q, reason=%q, blocked_until=%s", form.Username, remoteIP, reason, blockedUntil.Format(time.RFC3339))
		a.tgbot.UserLoginNotify(tgbot.LoginAttempt{
			Username: safeUser,
			IP:       remoteIP,
			Time:     timeStr,
			Status:   tgbot.LoginFail,
			Reason:   reason,
			NewIP:    !service.PanelLoginKnown(remoteIP),
		})
		pureJsonMsg(c, http.StatusOK, false, I18nWeb(c, "pages.login.toasts.wrongUsernameOrPassword"))
		return
	}

	user, checkErr := a.userService.CheckUser(form.Username, form.Password, form.TwoFactorCode)

	if user == nil {
		reason := loginFailureReason(checkErr)
		if blockedUntil, blocked := defaultLoginLimiter.RegisterFailure(remoteIP, form.Username); blocked {
			logger.Warningf("failed login: username=%q, IP=%q, reason=%q, blocked_until=%s", form.Username, remoteIP, reason, blockedUntil.Format(time.RFC3339))
		} else {
			logger.Warningf("failed login: username=%q, IP=%q, reason=%q", form.Username, remoteIP, reason)
		}
		a.tgbot.UserLoginNotify(tgbot.LoginAttempt{
			Username: safeUser,
			IP:       remoteIP,
			Time:     timeStr,
			Status:   tgbot.LoginFail,
			Reason:   reason,
			NewIP:    !service.PanelLoginKnown(remoteIP),
		})
		pureJsonMsg(c, http.StatusOK, false, I18nWeb(c, "pages.login.toasts.wrongUsernameOrPassword"))
		return
	}

	defaultLoginLimiter.RegisterSuccess(remoteIP, form.Username)
	logger.Infof("logged in successfully: username=%q, IP=%q", form.Username, remoteIP)
	newIP, err := service.RecordPanelLogin(remoteIP, time.Now())
	if err != nil {
		logger.Warning("Unable to remember the login network:", err)
		newIP = true
	}
	a.tgbot.UserLoginNotify(tgbot.LoginAttempt{
		Username: safeUser,
		IP:       remoteIP,
		Time:     timeStr,
		Status:   tgbot.LoginSuccess,
		NewIP:    newIP,
	})

	if err := session.SetLoginUser(c, user, form.RememberMe); err != nil {
		logger.Warning("Unable to save session:", err)
		return
	}

	jsonMsg(c, I18nWeb(c, "pages.login.toasts.successLogin"), nil)
}

func loginFailureReason(err error) string {
	if err != nil && err.Error() == "invalid 2fa code" {
		return "invalid 2FA code"
	}
	return "invalid credentials"
}

func (a *IndexController) logout(c *gin.Context) {
	user := session.GetLoginUser(c)
	if user != nil {
		logger.Infof("logged out successfully: username=%q", user.Username)
	}
	if err := session.ClearSession(c); err != nil {
		logger.Warning("Unable to clear session on logout:", err)
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// csrfToken returns the session CSRF token. Public — the login page
// needs a token before authenticating.
func (a *IndexController) csrfToken(c *gin.Context) {
	token, err := session.EnsureCSRFToken(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "obj": token})
}

// getTwoFactorEnable retrieves the current status of two-factor authentication.
func (a *IndexController) getTwoFactorEnable(c *gin.Context) {
	status, err := a.settingService.GetTwoFactorEnable()
	jsonObj(c, status, err)
}
