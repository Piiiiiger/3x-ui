package model

import "strings"

// The target inbound name is the direct subscription name. Only the relay
// variant has a separate display name.
func (c ProxyChain) DirectProxyName(target string) string {
	return proxyChainTargetName(target)
}

func (c ProxyChain) RelayProxyName(target, relay string) string {
	if name := strings.TrimSpace(c.RelayName); name != "" {
		return name
	}
	if relay = strings.TrimSpace(relay); relay != "" {
		return proxyChainTargetName(target) + "（中转·" + relay + "）"
	}
	return proxyChainTargetName(target) + "（中转）"
}

func proxyChainTargetName(name string) string {
	if name = strings.TrimSpace(name); name != "" {
		return name
	}
	return "目标节点"
}
