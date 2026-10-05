package sub

import (
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/database"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// ShareEndpoint is one render target for a link: the address and port to dial, from
// an externalProxy entry (kept in ep for its TLS keys) or the inbound itself.
type ShareEndpoint struct {
	Address string
	Port    int
	Remark  string // extra remark slot fed to genRemark, not a rendered remark
	// ForceTls is the raw "same"/"tls"/"none"/"" value: three behaviors branch on it.
	ForceTls string

	// ep is the source externalProxy entry, nil for the inbound's default endpoint.
	ep map[string]any
}

// externalProxyToEndpoint maps one externalProxy entry to an endpoint that
// carries the entry for delegated, provably-identical TLS application.
func externalProxyToEndpoint(ep map[string]any) ShareEndpoint {
	e := ShareEndpoint{ep: ep}
	e.Address, _ = ep["dest"].(string)
	if p, ok := ep["port"].(float64); ok {
		e.Port = int(p)
	}
	e.Remark, _ = ep["remark"].(string)
	e.ForceTls, _ = ep["forceTls"].(string)
	return e
}

// inboundDefaultEndpoint is the endpoint for an inbound's own resolved
// address/port (the no-externalProxy default). forceTls "same" keeps the base
// security; no per-endpoint TLS override.
func (s *SubService) inboundDefaultEndpoint(inbound *model.Inbound) ShareEndpoint {
	return ShareEndpoint{
		Address:  s.resolveInboundAddress(inbound),
		Port:     inbound.Port,
		ForceTls: "same",
	}
}

// advertisedEndpoints is every endpoint a stream-less link (mtproto, wireguard,
// amneziawg) must fan out over: the externalProxy/Host entries, else the default.
func (s *SubService) advertisedEndpoints(inbound *model.Inbound) []ShareEndpoint {
	stream := unmarshalStreamSettings(inbound.StreamSettings)
	if externalProxies, ok := stream["externalProxy"].([]any); ok {
		endpoints := make([]ShareEndpoint, 0, len(externalProxies))
		for _, raw := range externalProxies {
			if ep, ok := raw.(map[string]any); ok {
				endpoints = append(endpoints, externalProxyToEndpoint(ep))
			}
		}
		if len(endpoints) > 0 {
			return endpoints
		}
	}
	return []ShareEndpoint{s.inboundDefaultEndpoint(inbound)}
}

// applyEndpointTLSParams applies an endpoint's TLS overrides onto a URL-param
// map. External-proxy endpoints delegate to the unchanged helper; host/default
// endpoints carry no override yet (Phase 4).
func applyEndpointTLSParams(e ShareEndpoint, params map[string]string, security string) {
	if e.ep != nil {
		applyExternalProxyTLSParams(e.ep, params, security)
	}
}

// applyEndpointTLSObj is applyEndpointTLSParams for the VMess base64-JSON form.
func applyEndpointTLSObj(e ShareEndpoint, obj map[string]any, security string) {
	if e.ep != nil {
		applyExternalProxyTLSObj(e.ep, obj, security)
	}
}

// dropBaseRealityParams removes the parameters that only mean something on a
// reality link once a host forces the endpoint to plain TLS or no TLS.
func dropBaseRealityParams(params map[string]string, baseSecurity, securityToApply string) {
	if baseSecurity != "reality" || securityToApply == "reality" {
		return
	}
	// sni and fp name the master's reality dest, not this endpoint's own
	// certificate; the host's values are re-applied right after this.
	for _, k := range []string{"pbk", "sid", "spx", "pqv", "sni", "fp"} {
		delete(params, k)
	}
}

// buildEndpointLinks renders one URL-param link per endpoint (vless/trojan/ss).
// securityToApply mirrors the legacy externalProxy loop: "same" keeps the base
// security, otherwise the endpoint's forceTls wins; "none" strips TLS hint
// fields at emit time.
func (s *SubService) buildEndpointLinks(
	eps []ShareEndpoint,
	params map[string]string,
	baseSecurity string,
	makeLink func(e ShareEndpoint) string,
	makeRemark func(e ShareEndpoint) string,
) string {
	links := make([]string, 0, len(eps))
	for _, e := range eps {
		securityToApply := baseSecurity
		if e.ForceTls != "same" {
			securityToApply = e.ForceTls
		}
		nextParams := cloneStringMap(params)
		dropBaseRealityParams(nextParams, baseSecurity, securityToApply)
		applyEndpointTLSParams(e, nextParams, securityToApply)
		links = append(links, buildLinkWithParamsAndSecurity(
			makeLink(e),
			nextParams,
			makeRemark(e),
			securityToApply,
			e.ForceTls == "none",
		))
	}
	return strings.Join(links, "\n")
}

// buildEndpointVmessLinks renders one VMess base64-JSON link per endpoint.
func (s *SubService) buildEndpointVmessLinks(eps []ShareEndpoint, baseObj map[string]any, inbound *model.Inbound, email string, transport string) string {
	var links strings.Builder
	for index, e := range eps {
		securityToApply, _ := baseObj["tls"].(string)
		if e.ForceTls != "same" {
			securityToApply = e.ForceTls
		}
		newObj := cloneVmessShareObj(baseObj, e.ForceTls)
		newObj["ps"] = s.endpointRemark(inbound, email, e.ep, transport)
		newObj["add"] = e.Address
		newObj["port"] = e.Port
		if e.ForceTls != "same" {
			newObj["tls"] = e.ForceTls
		}
		applyEndpointTLSObj(e, newObj, securityToApply)
		if index > 0 {
			links.WriteString("\n")
		}
		links.WriteString(buildVmessLink(newObj))
	}
	return links.String()
}

// ChainEndpoints shares subscription address/NAT selection with chain checks.
func ChainEndpoints(inbound *model.Inbound) ([]ShareEndpoint, error) {
	s := NewSubService("")
	var nodes []*model.Node
	if err := database.GetDB().Find(&nodes).Error; err != nil {
		return nil, err
	}
	s.nodesByID = map[int]*model.Node{}
	for _, n := range nodes {
		s.nodesByID[n.Id] = n
	}
	return s.advertisedEndpoints(s.withPublicPort(inbound)), nil
}
