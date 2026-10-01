package agent

import (
	"sort"
	"time"
)

// onlineGrace matches the panel's own window: Xray often reports a zero delta for
// a live session across one poll, so a single quiet poll must not drop a client.
const onlineGrace = 20 * time.Second

// activity remembers when each client and inbound last carried traffic.
type activity struct {
	grace  time.Duration
	emails map[string]time.Time
	tags   map[string]time.Time
}

func newActivity(grace time.Duration) *activity {
	return &activity{grace: grace, emails: map[string]time.Time{}, tags: map[string]time.Time{}}
}

func (a *activity) observe(now time.Time, emails, tags []string) {
	for _, e := range emails {
		a.emails[e] = now
	}
	for _, t := range tags {
		a.tags[t] = now
	}
}

// current lists what was active within the grace window, forgetting the rest.
func (a *activity) current(now time.Time) (emails, tags []string) {
	return within(a.emails, now, a.grace), within(a.tags, now, a.grace)
}

func within(seen map[string]time.Time, now time.Time, grace time.Duration) []string {
	out := make([]string, 0, len(seen))
	for name, at := range seen {
		if now.Sub(at) < grace {
			out = append(out, name)
		} else {
			delete(seen, name)
		}
	}
	sort.Strings(out)
	return out
}
