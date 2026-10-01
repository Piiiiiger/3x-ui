package job

import (
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

// DailyTrafficJob adds each client's latest usage to today's row of the
// traffic history behind the overview chart.
type DailyTrafficJob struct {
	settingService service.SettingService
	statsService   service.TrafficStatsService
}

func NewDailyTrafficJob() *DailyTrafficJob {
	return new(DailyTrafficJob)
}

func (j *DailyTrafficJob) Run() {
	loc, err := j.settingService.GetTimeLocation()
	if err != nil {
		loc = time.Local
	}
	if err := j.statsService.RecordDaily(time.Now().In(loc)); err != nil {
		logger.Warning("record daily traffic failed:", err)
	}
}
