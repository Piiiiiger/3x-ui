package service

import (
	"encoding/json"
	"net/netip"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/runtime"
)

// AgentObservations reads the live addresses in every connected agent's latest
// report.
func (s *IpLimitService) AgentObservations(now time.Time) []IpObservation {
	hub := runtime.GetAgentHub()
	if hub == nil {
		return nil
	}
	var out []IpObservation
	for _, id := range hub.Connected() {
		if state, ok := hub.Session(id); ok {
			out = append(out, observationsFromAgentStatus(id, state, now)...)
		}
	}
	return out
}

// An agent that stopped reporting says nothing about who is online now.
func observationsFromAgentStatus(nodeID int, state runtime.AgentState, now time.Time) []IpObservation {
	if state.StatusAt.IsZero() || now.Sub(state.StatusAt) > agentStatusStaleAfter {
		return nil
	}
	var out []IpObservation
	for email, entries := range state.Status.IPs {
		for _, e := range entries {
			out = append(out, IpObservation{Email: email, IP: e.IP, LastSeen: e.Timestamp, Server: nodeID})
		}
	}
	return out
}

// RecordLocal stores this panel's own live addresses for the IP log.
func (s *IpLimitService) RecordLocal(observed []IpObservation, now time.Time) error {
	guid, err := s.settingService.GetPanelGuid()
	if err != nil {
		return err
	}
	return s.recordLive(guid, observed, now)
}

// recordLive stores one server's live addresses for the IP log under key, each
// under the client a relay identity stands in for, stamped now: they are live now.
func (s *IpLimitService) recordLive(key string, observed []IpObservation, now time.Time) error {
	if len(observed) == 0 {
		return nil
	}
	owners, err := s.cachedChainTransitOwners(now)
	if err != nil {
		return err
	}
	perEmail := map[string][]model.ClientIpEntry{}
	seen := map[string]bool{}
	for _, o := range observed {
		email := o.Email
		if owner, ok := owners[email]; ok {
			email = owner
		}
		ip := strings.Trim(strings.TrimSpace(o.IP), "[]")
		if addr, err := netip.ParseAddr(ip); err == nil {
			ip = addr.Unmap().String()
		}
		if email == "" || ip == "" || seen[email+"\x00"+ip] {
			continue
		}
		seen[email+"\x00"+ip] = true
		perEmail[email] = append(perEmail[email], model.ClientIpEntry{IP: ip, Timestamp: now.Unix()})
	}
	flat := make([]model.InboundClientIps, 0, len(perEmail))
	for email, entries := range perEmail {
		raw, err := json.Marshal(entries)
		if err != nil {
			return err
		}
		flat = append(flat, model.InboundClientIps{ClientEmail: email, Ips: string(raw)})
	}
	if err := (&InboundService{}).MergeInboundClientIps(flat); err != nil {
		return err
	}
	return upsertNodeClientIps(key, perEmail)
}
