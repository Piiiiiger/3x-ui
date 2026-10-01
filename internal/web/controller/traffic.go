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
}

func NewTrafficController(g *gin.RouterGroup) *TrafficController {
	a := &TrafficController{}
	g.GET("/overview", a.overview)
	return a
}

func (a *TrafficController) overview(c *gin.Context) {
	loc, err := a.settingService.GetTimeLocation()
	if err != nil {
		loc = time.Local
	}
	ov, err := a.statsService.Overview(time.Now().In(loc), trafficOverviewDays)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, ov, nil)
}
