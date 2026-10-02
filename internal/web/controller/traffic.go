package controller

import (
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/web/service"

	"github.com/gin-gonic/gin"
)

const trafficOverviewDays = 30

// TrafficController serves the traffic overview on the panel's home page.
type TrafficController struct {
	statsService   service.TrafficStatsService
	settingService service.SettingService
	probeService   service.ProbeService
}

func NewTrafficController(g *gin.RouterGroup) *TrafficController {
	a := &TrafficController{}
	g.GET("/overview", a.overview)
	return a
}

func (a *TrafficController) overview(c *gin.Context) {
	period, err := service.ParseTrafficPeriod(c.Query("period"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	loc, err := a.settingService.GetTimeLocation()
	if err != nil {
		loc = time.Local
	}
	// Without the probe the page still shows its rankings, with the reason the quotas are missing.
	probe, err := a.probeService.Overview(c.Request.Context())
	if err != nil {
		probe = service.ProbeOverview{Configured: true, Error: err.Error()}
	}
	ov, err := a.statsService.Overview(time.Now().In(loc), trafficOverviewDays, period, probe)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, ov, nil)
}
