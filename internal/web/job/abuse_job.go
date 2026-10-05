package job

import (
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

// AbuseJob runs the abuse checks of the panel's own core and refreshes the
// configs when a ban begins, ends or is lifted.
type AbuseJob struct {
	xrayService service.XrayService
	abuse       service.AbuseService
}

func NewAbuseJob() *AbuseJob {
	return new(AbuseJob)
}

func (j *AbuseJob) Run() {
	changed, err := j.abuse.RunAbuseChecks(time.Now())
	if err != nil {
		logger.Warning("abuse: the checks failed:", err)
	}
	if changed {
		// Agents take the change on their next config sync, the local core by a hot apply.
		j.xrayService.SetToNeedRestart()
	}
}
