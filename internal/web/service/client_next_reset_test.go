package service

import (
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func TestNextClientReset(t *testing.T) {
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	at := func(s string) time.Time {
		t.Helper()
		v, err := time.ParseInLocation("2006-01-02 15:04", s, shanghai)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	monthly := func(day int) model.ClientRecord {
		return model.ClientRecord{TrafficReset: "monthly", TrafficResetDay: day}
	}

	tests := []struct {
		name       string
		rec        model.ClientRecord
		resetCount int
		now        string
		want       string
	}{
		{name: "monthly comes on its day at midnight", rec: monthly(22), now: "2026-10-02 10:00", want: "2026-10-22 00:00"},
		{name: "monthly that already ran today waits a month", rec: monthly(22), now: "2026-10-22 00:30", want: "2026-11-22 00:00"},
		{name: "a day past the month's end runs on its last day", rec: monthly(31), now: "2026-11-05 09:00", want: "2026-11-30 00:00"},
		{name: "day 0 runs on the 1st, as the job treats it", rec: monthly(0), now: "2026-10-02 10:00", want: "2026-11-01 00:00"},
		{name: "daily runs at the next midnight", rec: model.ClientRecord{TrafficReset: "daily"}, now: "2026-10-02 10:00", want: "2026-10-03 00:00"},
		{name: "weekly runs at midnight going into Sunday", rec: model.ClientRecord{TrafficReset: "weekly"}, now: "2026-10-02 10:00", want: "2026-10-04 00:00"},
		{name: "weekly on a Sunday after midnight waits a week", rec: model.ClientRecord{TrafficReset: "weekly"}, now: "2026-10-04 08:00", want: "2026-10-11 00:00"},
		{name: "hourly runs at the next full hour", rec: model.ClientRecord{TrafficReset: "hourly"}, now: "2026-10-02 10:20", want: "2026-10-02 11:00"},
		{name: "never has no reset", rec: model.ClientRecord{TrafficReset: "never"}, now: "2026-10-02 10:00"},
		{
			name: "auto-renew resets at expiry when that comes first",
			rec:  model.ClientRecord{TrafficReset: "monthly", TrafficResetDay: 22, Reset: 30, ExpiryTime: at("2026-10-10 12:00").UnixMilli()},
			now:  "2026-10-02 10:00", want: "2026-10-10 12:00",
		},
		{
			name: "the cycle wins when it comes before the auto-renewal",
			rec:  model.ClientRecord{TrafficReset: "monthly", TrafficResetDay: 5, ResetDay: 1, ExpiryTime: at("2026-11-01 00:00").UnixMilli()},
			now:  "2026-10-02 10:00", want: "2026-10-05 00:00",
		},
		{
			name:       "auto-renew stops once its cap has run",
			rec:        model.ClientRecord{Reset: 30, ResetMax: 2, ExpiryTime: at("2026-10-10 12:00").UnixMilli()},
			resetCount: 2, now: "2026-10-02 10:00",
		},
		{
			name: "an expiry without auto-renew does not reset usage",
			rec:  model.ClientRecord{ExpiryTime: at("2026-10-10 12:00").UnixMilli()},
			now:  "2026-10-02 10:00",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := nextClientReset(tc.rec, tc.resetCount, at(tc.now))
			var want int64
			if tc.want != "" {
				want = at(tc.want).UnixMilli()
			}
			if got != want {
				t.Fatalf("next reset = %v, want %q", time.UnixMilli(got).In(shanghai), tc.want)
			}
		})
	}
}
