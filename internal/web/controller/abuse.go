package controller

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

// AbuseController serves the admin's abuse page: which servers detect, the
// thresholds, the bans running and the latest hits, and an account's history.
type AbuseController struct {
	abuseService service.AbuseService
	xrayService  service.XrayService
}

func NewAbuseController(g *gin.RouterGroup) *AbuseController {
	a := &AbuseController{}
	a.initRouter(g)
	return a
}

func (a *AbuseController) initRouter(g *gin.RouterGroup) {
	g.GET("/overview", a.overview)
	g.POST("/mode", a.setMode)
	g.POST("/settings", a.setSettings)
	g.GET("/history/:email", a.history)
	g.POST("/lift/:email", a.lift)
	g.POST("/forgive/:email", a.forgive)
}

func (a *AbuseController) overview(c *gin.Context) {
	overview, err := a.abuseService.Overview(time.Now())
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, overview, nil)
}

type abuseModeRequest struct {
	NodeId int    `json:"nodeId"`
	Mode   string `json:"mode"`
}

// setMode switches one server; the panel's own core changes its config at once,
// an agent on its next sync.
func (a *AbuseController) setMode(c *gin.Context) {
	var req abuseModeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	if err := a.abuseService.SetMode(req.NodeId, req.Mode); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	if req.NodeId == 0 {
		a.xrayService.SetToNeedRestart()
	}
	a.overview(c)
}

func (a *AbuseController) setSettings(c *gin.Context) {
	var settings service.AbuseSettings
	if err := c.ShouldBindJSON(&settings); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	if err := a.abuseService.SetSettings(settings); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	a.overview(c)
}

func (a *AbuseController) history(c *gin.Context) {
	history, err := a.abuseService.HistoryOf(c.Param("email"), time.Now())
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, history, nil)
}

// lift ends the account's running ban; a lock ends with its strikes cleared.
func (a *AbuseController) lift(c *gin.Context) {
	if err := a.abuseService.Lift(c.Param("email"), time.Now()); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	a.xrayService.SetToNeedRestart()
	a.history(c)
}

func (a *AbuseController) forgive(c *gin.Context) {
	if err := a.abuseService.Forgive(c.Param("email")); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	a.history(c)
}
