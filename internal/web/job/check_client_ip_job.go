package job

import (
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

// pruneStaleIpRows cadence; the scan itself cannot prune offline clients' rows.
const ipPruneIntervalSeconds = int64(5 * 60)

// CheckClientIpJob enforces clients' IP limits. Each scan reads the live source
// addresses from this panel's core and from every agent's latest report, records
// the local ones for the IP log, and lets IpLimitService ban what is over a limit.
type CheckClientIpJob struct {
	xrayService service.XrayService
	ipLimit     service.IpLimitService
	lastIpPrune int64
}

func NewCheckClientIpJob() *CheckClientIpJob {
	return new(CheckClientIpJob)
}

func (j *CheckClientIpJob) Run() {
	now := time.Now()
	j.pruneStaleIpRows(now)

	observed := j.ipLimit.AgentObservations(now)
	if local, ok := j.localObservations(); ok {
		if err := j.ipLimit.RecordLocal(local, now); err != nil {
			logger.Debug("[LimitIP] recording this panel's client IPs failed:", err)
		}
		observed = append(observed, local...)
	}
	changed, err := j.ipLimit.Enforce(now, observed)
	if err != nil {
		logger.Warning("[LimitIP] enforcing IP limits failed:", err)
	}
	if changed {
		// Agents pick the new bans up on their next config sync; the local core
		// gets them through a hot apply.
		j.xrayService.SetToNeedRestart()
	}
}

// localObservations reads this panel's core; ok=false when the core is down or
// predates the online-stats API, which leaves only the agents to count.
func (j *CheckClientIpJob) localObservations() ([]service.IpObservation, bool) {
	users, ok, err := j.xrayService.GetOnlineUsers()
	if err != nil || !ok {
		return nil, false
	}
	var out []service.IpObservation
	for _, u := range users {
		for _, ip := range u.IPs {
			out = append(out, service.IpObservation{Email: u.Email, IP: ip.IP, LastSeen: ip.LastSeen})
		}
	}
	return out, true
}

// Runs before anything else in the scan: retention must hold for stored rows
// even while nothing is being collected.
func (j *CheckClientIpJob) pruneStaleIpRows(now time.Time) {
	if now.Unix()-j.lastIpPrune < ipPruneIntervalSeconds {
		return
	}
	j.lastIpPrune = now.Unix()
	if err := (&service.InboundService{}).PruneStaleClientIps(); err != nil {
		logger.Warning("prune stale client ip rows failed:", err)
	}
}
