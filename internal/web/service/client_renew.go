package service

import (
	"math"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

// Renew gives each user days more from the later of now and their expiry: a user
// without an expiry keeps none, one not yet started gets a longer first period.
func (s *ClientService) Renew(inboundSvc *InboundService, emails []string, days int, resetUsage bool) (bool, error) {
	if _, err := RenewedExpiry(0, days, time.Now()); err != nil {
		return false, err
	}
	needRestart := false
	for _, email := range emails {
		nr, err := s.renewOne(inboundSvc, email, days, resetUsage)
		needRestart = needRestart || nr
		if err != nil {
			return needRestart, err
		}
	}
	return needRestart, nil
}

// RenewedExpiry is the expiry a renewal of days leaves, so a preview shows exactly
// what Renew will write.
func RenewedExpiry(expiry int64, days int, now time.Time) (int64, error) {
	if days < 1 {
		return 0, common.NewError("a renewal adds at least one day")
	}
	if int64(days) > math.MaxInt64/planDayMillis {
		return 0, common.NewError("the renewal exceeds the supported expiry range")
	}
	add := int64(days) * planDayMillis
	switch {
	case expiry > 0:
		base := max(expiry, now.UnixMilli())
		if base > math.MaxInt64-add {
			return 0, common.NewError("the renewal exceeds the supported expiry range")
		}
		return base + add, nil
	case expiry < 0:
		if expiry < math.MinInt64+add {
			return 0, common.NewError("the renewal exceeds the supported expiry range")
		}
		return expiry - add, nil
	}
	return 0, nil
}

func (s *ClientService) renewOne(inboundSvc *InboundService, email string, days int, resetUsage bool) (bool, error) {
	rec, err := s.GetRecordByEmail(nil, email)
	if err != nil {
		return false, err
	}
	cutOff := false
	if !rec.Enable {
		if cutOff, err = depletedNow(email); err != nil {
			return false, err
		}
	}
	client := rec.ToClient()
	if client.ExpiryTime, err = RenewedExpiry(rec.ExpiryTime, days, time.Now()); err != nil {
		return false, err
	}
	needRestart, err := s.Update(inboundSvc, rec.Id, *client, rec.LimitHwid)
	if err != nil {
		return needRestart, err
	}
	if resetUsage {
		nr, err := s.ResetTrafficByEmail(inboundSvc, email)
		return needRestart || nr, err
	}
	// As bulk adjust: only a user the traffic job cut off, and whom the renewal frees.
	if !cutOff {
		return needRestart, nil
	}
	if still, err := depletedNow(email); err != nil || still {
		return needRestart, err
	}
	_, nr, err := s.BulkSetEnable(inboundSvc, []string{email}, true)
	return needRestart || nr, err
}

// depletedNow reports whether the user is past their expiry or quota, by the test the
// traffic job disables users with.
func depletedNow(email string) (bool, error) {
	db := database.GetDB()
	cond, args := depletedCond(db)
	var found int64
	err := db.Model(xray.ClientTraffic{}).Where(cond+" AND email = ?", append(args, email)...).Count(&found).Error
	return found > 0, err
}
