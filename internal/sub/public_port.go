package sub

import (
	"encoding/json"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// withPublicPort returns the inbound links render for. Behind NAT (a share port and
// no externalProxy of its own) that is a copy advertising the public port instead.
func (s *SubService) withPublicPort(inbound *model.Inbound) *model.Inbound {
	if inbound.SharePort <= 0 || inbound.SharePort == inbound.Port {
		return inbound
	}
	stream := unmarshalStreamSettings(inbound.StreamSettings)
	if stream == nil {
		stream = map[string]any{}
	}
	if eps, ok := stream["externalProxy"].([]any); ok && len(eps) > 0 {
		return inbound
	}
	stream["externalProxy"] = []any{map[string]any{
		"forceTls": "same",
		"dest":     s.resolveInboundAddress(inbound),
		"port":     float64(inbound.SharePort),
		"remark":   "",
	}}
	b, err := json.Marshal(stream)
	if err != nil {
		return inbound
	}
	clone := *inbound
	clone.StreamSettings = string(b)
	return &clone
}

// endpointRemark is the remark for one endpoint's link, proxy or config entry: the
// standard composition, with the endpoint's own remark in the extra slot.
func (s *SubService) endpointRemark(inbound *model.Inbound, email string, ep map[string]any, transport string) string {
	var extra string
	if ep != nil {
		extra, _ = ep["remark"].(string)
	}
	return s.genRemark(inbound, email, extra, transport)
}
