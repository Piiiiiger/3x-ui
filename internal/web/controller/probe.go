package controller

import (
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"

	"github.com/gin-gonic/gin"
)

// ProbeController serves the admin Probe page: every server of the Lite
// monitor, the links to this panel's hosts, and where Lite is.
type ProbeController struct {
	probeService service.ProbeService
}

func NewProbeController(g *gin.RouterGroup) *ProbeController {
	a := &ProbeController{}
	a.initRouter(g)
	return a
}

func (a *ProbeController) initRouter(g *gin.RouterGroup) {
	g.GET("/servers", a.servers)
	g.GET("/links", a.links)
	g.POST("/links", a.setLinks)
	g.GET("/settings", a.settings)
	g.POST("/settings", a.saveSettings)
}

// servers answers with success even when Lite could not be read: the page
// shows that error next to the last good servers.
func (a *ProbeController) servers(c *gin.Context) {
	overview, err := a.probeService.Overview(c.Request.Context())
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, overview, nil)
}

func (a *ProbeController) links(c *gin.Context) {
	links, err := a.probeService.Links(c.Request.Context())
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, links, nil)
}

func (a *ProbeController) setLinks(c *gin.Context) {
	var in service.ProbeLinksInput
	if err := c.ShouldBindJSON(&in); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	if err := a.probeService.SetLinks(in); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	a.links(c)
}

func (a *ProbeController) settings(c *gin.Context) {
	settings, err := a.probeService.Settings()
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, settings, nil)
}

func (a *ProbeController) saveSettings(c *gin.Context) {
	var in service.ProbeSettings
	if err := c.ShouldBindJSON(&in); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	saved, err := a.probeService.SaveSettings(in)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, saved, nil)
}
