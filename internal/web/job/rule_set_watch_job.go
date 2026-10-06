package job

import (
	"context"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

// RuleSetWatchJob fetches the upstream lists the rule sets follow; the bot tells the
// admins when what changed is worth a review.
type RuleSetWatchJob struct {
	ruleSets service.RuleSetService
}

func NewRuleSetWatchJob() *RuleSetWatchJob {
	return &RuleSetWatchJob{}
}

func (j *RuleSetWatchJob) Run() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := j.ruleSets.CheckUpstream(ctx, time.Now()); err != nil {
		logger.Warning("rule sets: the upstream check failed:", err)
	}
}

// RuleSetWatchSchedule fires daily at 10:00 UTC+8, the admins' morning, whatever the
// panel's own time zone; a fixed zone needs no tzdata on the host.
func RuleSetWatchSchedule() cron.Schedule {
	spec, err := cron.ParseStandard("0 10 * * *")
	if err != nil {
		panic(err)
	}
	spec.(*cron.SpecSchedule).Location = time.FixedZone("UTC+8", 8*60*60)
	return spec
}
