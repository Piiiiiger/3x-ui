package service

import (
	"encoding/json"
	"fmt"
	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
	"net"
	"strconv"
	"strings"
)

// injectChainTransit creates non-billable relay identities. The destination hop
// retains the user's original identity and charges once. Direct use of a relay
// also retains its original identity. Transit credentials are destination-bound.
func injectChainTransit(cfg *xray.Config) error {
	db := database.GetDB()
	var chains []model.ProxyChain
	if err := db.Where("enabled = ?", true).Find(&chains).Error; err != nil {
		return err
	}
	var extraRules []any
	for _, chain := range chains {
		var relay, target model.Inbound
		if err := db.First(&relay, chain.RelayInboundId).Error; err != nil {
			return err
		}
		if err := db.First(&target, chain.TargetInboundId).Error; err != nil {
			return err
		}
		if !target.Enable || !relay.Enable {
			continue
		}
		var ib *xray.InboundConfig
		for i := range cfg.InboundConfigs {
			if cfg.InboundConfigs[i].Tag == relay.Tag {
				ib = &cfg.InboundConfigs[i]
				break
			}
		}
		if ib == nil {
			continue
		}
		if relay.Protocol != model.VLESS && relay.Protocol != model.VMESS && relay.Protocol != model.Trojan {
			return fmt.Errorf("chain %d: relay protocol %s cannot isolate transit accounting", chain.Id, relay.Protocol)
		}
		targets, err := (&ClientService{}).ListForInbound(nil, target.Id)
		if err != nil {
			return err
		}
		granted := map[string]bool{}
		for _, c := range targets {
			if c.Enable {
				granted[c.Email] = true
			}
		}
		var settings map[string]any
		if err := json.Unmarshal(ib.Settings, &settings); err != nil {
			return err
		}
		clients, _ := settings["clients"].([]any)
		users := []string{}
		// Iterate only original users; aliases from another chain cannot match grants.
		for _, v := range clients {
			entry, ok := v.(map[string]any)
			if !ok {
				continue
			}
			email, _ := entry["email"].(string)
			if !granted[email] {
				continue
			}
			field := "id"
			if relay.Protocol == model.Trojan {
				field = "password"
			}
			secret, _ := entry[field].(string)
			if secret == "" {
				continue
			}
			credential, alias := model.ChainTransitCredential(secret, chain.Id)
			clone := map[string]any{}
			for k, v := range entry {
				clone[k] = v
			}
			clone[field] = credential
			clone["email"] = alias
			delete(clone, "reverse")
			clients = append(clients, clone)
			users = append(users, alias)
		}
		if len(users) == 0 {
			continue
		}
		settings["clients"] = clients
		raw, err := json.Marshal(settings)
		if err != nil {
			return err
		}
		ib.Settings = raw
		endpoints, err := chainTransitEndpoints(&target)
		if err != nil {
			return err
		}
		for _, ep := range endpoints {
			rule := map[string]any{"type": "field", "user": users, "inboundTag": []string{relay.Tag}, "port": strconv.Itoa(ep.port), "outboundTag": "chain-transit-direct"}
			if net.ParseIP(ep.host) != nil {
				rule["ip"] = []string{ep.host}
			} else {
				rule["domain"] = []string{"full:" + ep.host}
			}
			extraRules = append(extraRules, rule)
		}
		extraRules = append(extraRules, map[string]any{"type": "field", "user": users, "inboundTag": []string{relay.Tag}, "outboundTag": "chain-transit-block"})
	}
	if len(extraRules) == 0 {
		return nil
	}
	var outbounds []map[string]any
	if err := json.Unmarshal(cfg.OutboundConfigs, &outbounds); err != nil {
		return err
	}
	for _, o := range outbounds {
		if o["tag"] == "chain-transit-direct" || o["tag"] == "chain-transit-block" {
			return fmt.Errorf("reserved chain transit outbound tag")
		}
	}
	outbounds = append(outbounds, map[string]any{"tag": "chain-transit-direct", "protocol": "freedom", "settings": map[string]any{}}, map[string]any{"tag": "chain-transit-block", "protocol": "blackhole", "settings": map[string]any{}})
	raw, err := json.Marshal(outbounds)
	if err != nil {
		return err
	}
	cfg.OutboundConfigs = raw
	routing := map[string]any{}
	if len(cfg.RouterConfig) > 0 {
		if err := json.Unmarshal(cfg.RouterConfig, &routing); err != nil {
			return err
		}
	}
	rules, _ := routing["rules"].([]any)
	routing["rules"] = append(extraRules, rules...)
	raw, err = json.Marshal(routing)
	if err != nil {
		return err
	}
	cfg.RouterConfig = raw
	return nil
}

type chainTransitEndpoint struct {
	host string
	port int
}

func chainTransitEndpoints(target *model.Inbound) ([]chainTransitEndpoint, error) {
	stream := map[string]any{}
	if err := json.Unmarshal([]byte(target.StreamSettings), &stream); strings.TrimSpace(target.StreamSettings) != "" && err != nil {
		return nil, err
	}
	var eps []chainTransitEndpoint
	if entries, ok := stream["externalProxy"].([]any); ok {
		for _, v := range entries {
			if e, ok := v.(map[string]any); ok {
				host, _ := e["dest"].(string)
				port, _ := e["port"].(float64)
				eps = append(eps, chainTransitEndpoint{strings.Trim(host, "[]"), int(port)})
			}
		}
	}
	if len(eps) == 0 {
		host := ""
		if target.NodeID != nil {
			var n model.Node
			if err := database.GetDB().First(&n, *target.NodeID).Error; err != nil {
				return nil, err
			}
			host = n.Address
		}
		listen := strings.Trim(target.Listen, "[]")
		ip := net.ParseIP(listen)
		usable := listen != "" && (ip == nil || (!ip.IsUnspecified() && !ip.IsLoopback()))
		if (host == "" || target.ShareAddrStrategy == "listen") && usable {
			host = listen
		}
		if target.ShareAddrStrategy == "custom" && strings.TrimSpace(target.ShareAddr) != "" {
			host = target.ShareAddr
		}
		port := target.Port
		if target.SharePort > 0 {
			port = target.SharePort
		}
		eps = append(eps, chainTransitEndpoint{strings.Trim(host, "[]"), port})
	}
	for _, ep := range eps {
		if ep.host == "" || ep.port <= 0 || ep.port > 65535 {
			return nil, fmt.Errorf("chain target %s requires a public endpoint", target.Remark)
		}
	}
	return eps, nil
}
