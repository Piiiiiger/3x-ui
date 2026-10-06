package job

import (
	"testing"
	"time"
)

// The upstream lists are fetched at 10:00 in the admins' UTC+8 morning, whatever
// time zone the panel runs in, once a day.
func TestRuleSetWatchRunsAtTenUTC8(t *testing.T) {
	sched := RuleSetWatchSchedule()
	cases := []struct{ after, want time.Time }{
		{time.Date(2026, 10, 6, 1, 59, 0, 0, time.UTC), time.Date(2026, 10, 6, 2, 0, 0, 0, time.UTC)},
		{time.Date(2026, 10, 6, 2, 0, 0, 0, time.UTC), time.Date(2026, 10, 7, 2, 0, 0, 0, time.UTC)},
	}
	for _, c := range cases {
		if got := sched.Next(c.after); !got.Equal(c.want) {
			t.Errorf("after %s the next check is %s, want %s", c.after, got.UTC(), c.want)
		}
	}
}
