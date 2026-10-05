package tgbot

import (
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

// Regression: the admin's traffic reset zeroed the counters only, so a user the
// quota had cut off stayed disabled on every server; the panel's reset frees them.
func TestAdminTrafficResetBringsBackAUserTheQuotaCutOff(t *testing.T) {
	for _, tc := range []struct{ name, data string }{
		{"one user", "reset_traffic_c cut@x"},
		{"every user", "reset_all_traffics_c"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tb, _ := newPiggerBot(t)
			ib := seedVlessInbound(t, "a")
			seedClient(t, "cut@x", []int{ib}, nil)
			setUsage(t, "cut@x", 6<<30, 4<<30)
			if _, _, err := (&service.ClientService{}).SetClientEnableByEmail(&service.InboundService{}, "cut@x", false); err != nil {
				t.Fatal(err)
			}

			tb.tap(adminTgID, tc.data)

			if !clientRecord(t, "cut@x").Enable {
				t.Error("the user is still disabled after the admin reset their traffic")
			}
			if got := usageOf(t, "cut@x"); got != 0 {
				t.Errorf("usage = %d after the reset, want 0", got)
			}
		})
	}
}
