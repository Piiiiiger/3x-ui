package job

import (
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/sub"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

// RemoteRoutingJob keeps remote Happ and Clash/Mihomo routing URLs warm: all
// network work runs here (cron + startup warm), never in a request handler.
type RemoteRoutingJob struct {
	settingService service.SettingService
	templates      service.RuleTemplateService
}

func NewRemoteRoutingJob() *RemoteRoutingJob {
	return &RemoteRoutingJob{}
}

func (j *RemoteRoutingJob) Run() {
	happ, err := j.settingService.GetSubRoutingRules()
	if err != nil {
		logger.Warning("Could not read Happ routing source:", err)
		return
	}
	jsonRouting, err := j.settingService.GetSubJsonRoutingRules()
	if err != nil {
		logger.Warning("Could not read JSON subscription routing source:", err)
		return
	}
	clash, err := j.templates.Contents()
	if err != nil {
		logger.Warning("Could not read the rule templates' Clash routing sources:", err)
	}
	sub.RefreshRemoteRoutingSources(happ, clash, jsonRouting)
}
