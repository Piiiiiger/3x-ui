package controller

import (
	"strconv"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/web/service"

	"github.com/gin-gonic/gin"
)

// AiUsageController receives the Claude Code / Codex usage Pigger Switch
// reports and serves the AI usage page.
type AiUsageController struct {
	aiUsageService service.AiUsageService
	settingService service.SettingService
}

func NewAiUsageController(g *gin.RouterGroup) *AiUsageController {
	a := &AiUsageController{}
	g.POST("/ingest", a.ingest)
	g.GET("/overview", a.overview)
	g.POST("/devices/delete/:id", a.deleteDevice)
	return a
}

func (a *AiUsageController) ingest(c *gin.Context) {
	report := &service.AiUsageReport{}
	if err := c.ShouldBindJSON(report); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	result, err := a.aiUsageService.Ingest(report, time.Now())
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, result, nil)
}

func (a *AiUsageController) overview(c *gin.Context) {
	period, err := service.ParseAiUsagePeriod(c.Query("period"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	deviceId, _ := strconv.Atoi(c.Query("deviceId"))
	app := c.Query("app")
	if app == "all" {
		app = ""
	}
	loc, err := a.settingService.GetTimeLocation()
	if err != nil {
		loc = time.Local
	}
	ov, err := a.aiUsageService.Overview(time.Now().In(loc), period, deviceId, app)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, ov, nil)
}

func (a *AiUsageController) deleteDevice(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonMsg(c, I18nWeb(c, "pages.aiUsage.deviceDeleted"), a.aiUsageService.DeleteDevice(id))
}
