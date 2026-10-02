package sub

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service/panel"
)

func (a *SUBController) portalAdminLogin(c *gin.Context) {
	setNoCacheHeaders(c)
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, portalBodyLimit)
	var form struct {
		Username      string `json:"username"`
		Password      string `json:"password"`
		TwoFactorCode string `json:"twoFactorCode"`
	}
	if err := c.ShouldBindJSON(&form); err != nil || form.Username == "" || form.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid"})
		return
	}
	enabled, err := a.settingService.GetTwoFactorEnable()
	if err != nil || !enabled {
		c.JSON(http.StatusForbidden, gin.H{"error": "twoFactorRequired"})
		return
	}
	ip := a.portalClientIP(c)
	if _, ok := a.portalLimiter.Allow(ip, "*admin*"); !ok {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "blocked"})
		return
	}
	if _, ok := a.portalUserCap.Allow(portalAnyAddress, "admin:"+form.Username); !ok {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "blocked"})
		return
	}
	user, err := (&panel.UserService{}).CheckUser(form.Username, form.Password, form.TwoFactorCode)
	if err != nil || user == nil {
		a.portalLimiter.RegisterFailure(ip, "*admin*")
		a.portalUserCap.RegisterFailure(portalAnyAddress, "admin:"+form.Username)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid"})
		return
	}
	token, err := service.IssueAdminHandoff(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server"})
		return
	}
	basePath, err := a.settingService.GetBasePath()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server"})
		return
	}
	secure := c.Request.TLS != nil || (a.subService.forwardedHeadersTrusted(c) && strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https"))
	http.SetCookie(c.Writer, &http.Cookie{Name: service.AdminHandoffCookie, Value: token, Path: "/", HttpOnly: true, Secure: secure, SameSite: http.SameSiteStrictMode, MaxAge: 60})
	a.portalLimiter.RegisterSuccess(ip, "*admin*")
	a.portalUserCap.RegisterSuccess(portalAnyAddress, "admin:"+form.Username)
	c.JSON(http.StatusOK, gin.H{"success": true, "token": token, "path": basePath + "portal-admin"})
}
