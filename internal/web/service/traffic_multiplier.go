package service

import (
	"errors"
	"math"
	"strconv"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
)

// maxTrafficMultiplier keeps a typo such as 1000 for 1.000 from billing users
// a thousandfold.
const maxTrafficMultiplier = 100

var (
	errTrafficMultiplierRange = errors.New("the traffic multiplier must be a number from 0 to 100")
	errPanelTrafficMultiplier = errors.New("a 3x-ui child panel limits users on the bytes it counts itself, so its traffic multiplier stays 1")
)

func validateTrafficMultiplier(multiplier float64) error {
	if math.IsNaN(multiplier) || multiplier < 0 || multiplier > maxTrafficMultiplier {
		return errTrafficMultiplierRange
	}
	return nil
}

// chargedBytes is what moved bytes count as toward a user's quota on a host
// with this multiplier.
func chargedBytes(moved int64, multiplier float64) int64 {
	if multiplier == 1 {
		return moved
	}
	return clampedBytes(math.Round(float64(moved) * multiplier))
}

// sidecarQuota is the limit, in the bytes a sidecar counts itself, at which a
// quota charged at this multiplier runs out; 0 means no limit.
func sidecarQuota(quota int64, multiplier float64) int64 {
	if quota <= 0 || multiplier == 1 {
		return quota
	}
	if multiplier == 0 {
		return 0
	}
	return clampedBytes(math.Ceil(float64(quota) / multiplier))
}

func clampedBytes(n float64) int64 {
	if n >= float64(database.TrafficMax) {
		return database.TrafficMax
	}
	return int64(n)
}

// TrafficMultiplierView is a host's multiplier on its own, as the panel's own
// host reads and saves it.
type TrafficMultiplierView struct {
	Multiplier float64 `json:"multiplier" example:"0.1"`
}

// GetLocalTrafficMultiplier is the multiplier of the panel's own host.
func (s *SettingService) GetLocalTrafficMultiplier() (float64, error) {
	raw, err := s.getString("localTrafficMultiplier")
	if err != nil {
		return 0, err
	}
	multiplier, err := strconv.ParseFloat(effectiveSettingValue("localTrafficMultiplier", raw), 64)
	if err != nil {
		return 0, err
	}
	if err := validateTrafficMultiplier(multiplier); err != nil {
		return 0, err
	}
	return multiplier, nil
}

func (s *SettingService) SetLocalTrafficMultiplier(multiplier float64) error {
	if err := validateTrafficMultiplier(multiplier); err != nil {
		return err
	}
	return s.setString("localTrafficMultiplier", strconv.FormatFloat(multiplier, 'f', -1, 64))
}
