package model

import (
	"fmt"
	"strconv"
	"strings"
)

func PlanNodeKey(inboundID int, relay bool) string {
	mode := "direct"
	if relay {
		mode = "relay"
	}
	return fmt.Sprintf("%d:%s", inboundID, mode)
}

func PlanChainKey(targetID, chainID int) string {
	return fmt.Sprintf("%d:relay:%d", targetID, chainID)
}

func ParsePlanNodeKey(key string) (id int, relay bool, err error) {
	parts := strings.Split(key, ":")
	if (len(parts) != 2 && len(parts) != 3) || (parts[1] != "direct" && parts[1] != "relay") {
		return 0, false, fmt.Errorf("invalid plan node: %s", key)
	}
	id, err = strconv.Atoi(parts[0])
	if err != nil || id <= 0 || strconv.Itoa(id) != parts[0] {
		return 0, false, fmt.Errorf("invalid plan node: %s", key)
	}
	if len(parts) == 3 {
		chainID, e := strconv.Atoi(parts[2])
		if e != nil || chainID <= 0 || PlanChainKey(id, chainID) != key {
			return 0, false, fmt.Errorf("invalid plan node: %s", key)
		}
	}
	return id, parts[1] == "relay", nil
}
