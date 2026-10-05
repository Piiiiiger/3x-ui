package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// IpObservation is one live source address of a client on one server. LastSeen
// is when that address last opened a connection (unix seconds).
type IpObservation struct {
	Email    string
	IP       string
	LastSeen int64
}

// IpLimitService enforces clients' IP limits across this panel and its agents.
// A ban is a routing rule in the generated configs, so it needs no fail2ban.
type IpLimitService struct {
	settingService SettingService
}

// Why an address is left out of the IP limit, as the IP log labels it.
const (
	IpExemptHost      = "host"
	IpExemptAllowlist = "allowlist"
	IpExemptPrivate   = "private"
)

const (
	ipLimitBlockOutboundTag  = "iplimit-block"
	defaultIpLimitBanMinutes = 30
	ipLimitResolveTTL        = 10 * time.Minute
	ipLimitOwnersTTL         = 15 * time.Second
)

var (
	ipLimitLocalAddrs = localGlobalAddrs
	ipLimitLookupHost = lookupHostAddrs

	errIpNotBanned = errors.New("this network is not banned for this client")
	cgnatPrefix    = netip.MustParsePrefix("100.64.0.0/10")
)

// ipLimitNetwork maps a source address to the network the limit counts: the
// address itself for IPv4, its /64 for IPv6 (one home LAN or one mobile line).
func ipLimitNetwork(ip string) (string, netip.Addr, bool) {
	addr, err := netip.ParseAddr(strings.Trim(strings.TrimSpace(ip), "[]"))
	if err != nil {
		return "", netip.Addr{}, false
	}
	addr = addr.Unmap().WithZone("")
	if addr.Is4() {
		return addr.String(), addr, true
	}
	return netip.PrefixFrom(addr, 64).Masked().String(), addr, true
}

func isPublicAddr(a netip.Addr) bool {
	return a.IsGlobalUnicast() && !a.IsPrivate() && !cgnatPrefix.Contains(a)
}

func hostPrefix(a netip.Addr) netip.Prefix {
	if a.Is4() {
		return netip.PrefixFrom(a, 32)
	}
	return netip.PrefixFrom(a, 64).Masked()
}

type ipLimitHost struct {
	prefix netip.Prefix
	name   string
}

// ipLimitExemptions holds the addresses that are never counted or banned.
type ipLimitExemptions struct {
	hosts     []ipLimitHost
	allowlist ipLimitAllowlist
}

// reason says why addr is exempt; host names the server for a host address
// ("" for this panel's own host).
func (e *ipLimitExemptions) reason(addr netip.Addr) (kind, host string) {
	if !isPublicAddr(addr) {
		return IpExemptPrivate, ""
	}
	for _, h := range e.hosts {
		if h.prefix.Contains(addr) {
			return IpExemptHost, h.name
		}
	}
	if e.allowlist.contains(addr) {
		return IpExemptAllowlist, ""
	}
	return "", ""
}

// exemptions collects every Pigger server's addresses (any of them can relay
// for another), the operator's allowlist, and nothing else.
func (s *IpLimitService) exemptions(now time.Time) (*ipLimitExemptions, error) {
	var nodes []model.Node
	if err := database.GetDB().Select("id", "name", "address", "agent_remote_ip").Order("id").Find(&nodes).Error; err != nil {
		return nil, err
	}
	raw, err := s.settingService.GetIpLimitAllowlist()
	if err != nil {
		return nil, err
	}
	ex := &ipLimitExemptions{allowlist: parseIpLimitAllowlist(raw)}
	for _, a := range ipLimitLocalAddrs() {
		ex.hosts = append(ex.hosts, ipLimitHost{prefix: hostPrefix(a)})
	}
	for _, n := range nodes {
		for _, a := range nodeHostAddrs(n, now) {
			ex.hosts = append(ex.hosts, ipLimitHost{prefix: hostPrefix(a), name: n.Name})
		}
	}
	return ex, nil
}

func nodeHostAddrs(n model.Node, now time.Time) []netip.Addr {
	var out []netip.Addr
	host := strings.Trim(strings.TrimSpace(n.Address), "[]")
	if a, err := netip.ParseAddr(host); err == nil {
		out = append(out, a.Unmap())
	} else if host != "" {
		out = append(out, resolveHostCached(host, now)...)
	}
	if a, err := netip.ParseAddr(strings.TrimSpace(n.AgentRemoteIP)); err == nil {
		out = append(out, a.Unmap())
	}
	return out
}

type resolvedHost struct {
	addrs []netip.Addr
	at    time.Time
}

var ipLimitResolved = struct {
	sync.Mutex
	m map[string]resolvedHost
}{m: map[string]resolvedHost{}}

// resolveHostCached keeps the last good answer when a lookup fails, so a DNS
// hiccup cannot strip a relay of its exemption.
func resolveHostCached(host string, now time.Time) []netip.Addr {
	ipLimitResolved.Lock()
	cached, ok := ipLimitResolved.m[host]
	ipLimitResolved.Unlock()
	if ok && now.Sub(cached.at) < ipLimitResolveTTL {
		return cached.addrs
	}
	addrs, err := ipLimitLookupHost(host)
	if err != nil {
		logger.Debug("[LimitIP] resolving", host, "failed:", err)
		addrs = cached.addrs
	}
	for i := range addrs {
		addrs[i] = addrs[i].Unmap()
	}
	ipLimitResolved.Lock()
	ipLimitResolved.m[host] = resolvedHost{addrs: addrs, at: now}
	ipLimitResolved.Unlock()
	return addrs
}

func lookupHostAddrs(host string) ([]netip.Addr, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
}

func localGlobalAddrs() []netip.Addr {
	ifAddrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	var out []netip.Addr
	for _, a := range ifAddrs {
		ipNet, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		if addr, ok := netip.AddrFromSlice(ipNet.IP); ok && isPublicAddr(addr.Unmap()) {
			out = append(out, addr.Unmap())
		}
	}
	return out
}

var ipLimitOwners = struct {
	sync.Mutex
	owners map[string]string
	at     time.Time
}{}

// resetIpLimitCaches forgets cached lookups; tests change the data under them.
func resetIpLimitCaches() {
	ipLimitResolved.Lock()
	ipLimitResolved.m = map[string]resolvedHost{}
	ipLimitResolved.Unlock()
	ipLimitOwners.Lock()
	ipLimitOwners.owners, ipLimitOwners.at = nil, time.Time{}
	ipLimitOwners.Unlock()
}

// ChainTransitOwners maps every relay identity of an enabled chain to the client
// it stands in for, so a device that comes in through a relay counts as its owner's.
func (s *IpLimitService) ChainTransitOwners() (map[string]string, error) {
	routes, err := loadChainTransitRoutes()
	if err != nil {
		return nil, err
	}
	owners := map[string]string{}
	for _, r := range routes {
		if !chainTransitProtocol(r.relay.Protocol) {
			continue
		}
		clients, err := (&ClientService{}).ListForInbound(nil, r.relay.Id)
		if err != nil {
			return nil, err
		}
		for _, c := range clients {
			secret := c.ID
			if r.relay.Protocol == model.Trojan {
				secret = c.Password
			}
			if !r.granted[c.Email] || secret == "" {
				continue
			}
			_, alias := model.ChainTransitCredential(secret, r.chain.Id)
			owners[alias] = c.Email
		}
	}
	return owners, nil
}

// cachedChainTransitOwners serves the agents' frequent status reports without a
// chain walk on each one; a minted identity at worst shows unfolded for 15 s.
func (s *IpLimitService) cachedChainTransitOwners(now time.Time) (map[string]string, error) {
	ipLimitOwners.Lock()
	defer ipLimitOwners.Unlock()
	if ipLimitOwners.owners != nil && now.Sub(ipLimitOwners.at) < ipLimitOwnersTTL {
		return ipLimitOwners.owners, nil
	}
	owners, err := s.ChainTransitOwners()
	if err != nil {
		return nil, err
	}
	ipLimitOwners.owners, ipLimitOwners.at = owners, now
	return owners, nil
}

// Enforce applies every client's IP limit to one scan of live observations and
// reports whether the set of bans changed, i.e. the configs must be pushed.
func (s *IpLimitService) Enforce(now time.Time, observed []IpObservation) (bool, error) {
	db := database.GetDB()
	nowSec := now.Unix()
	lifted, err := liftLapsedIpBans(db, nowSec)
	if err != nil {
		return false, err
	}
	changed := len(lifted) > 0
	live, err := s.liveNetworks(now, observed)
	if err != nil || len(live) == 0 {
		return changed, err
	}
	emails := make([]string, 0, len(live))
	for email := range live {
		emails = append(emails, email)
	}
	sort.Strings(emails)
	limits, err := clientIpLimits(db, emails)
	if err != nil {
		return changed, err
	}
	banned, err := activeBanSet(db, nowSec)
	if err != nil {
		return changed, err
	}
	minutes, err := s.settingService.GetIpLimitBanMinutes()
	if err != nil {
		return changed, err
	}

	var fresh []model.ClientIpBan
	for _, email := range emails {
		limit := limits[email]
		if limit <= 0 {
			continue
		}
		for _, network := range networksOverLimit(live[email], banned[email], limit) {
			fresh = append(fresh, model.ClientIpBan{
				Email: email, Network: network, BannedAt: nowSec, ExpiresAt: nowSec + int64(minutes)*60,
			})
		}
	}
	if len(fresh) == 0 {
		return changed, nil
	}
	err = db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "email"}, {Name: "network"}},
		DoUpdates: clause.AssignmentColumns([]string{"banned_at", "expires_at"}),
	}).Create(&fresh).Error
	if err != nil {
		return changed, err
	}
	stamp := now.Format("2006/01/02 15:04:05")
	lines := make([]string, 0, len(fresh))
	for _, b := range fresh {
		logger.Infof("[LimitIP] %s is over its limit: %s banned for %d minutes", b.Email, b.Network, minutes)
		lines = append(lines, fmt.Sprintf("%s   BAN   [Email] = %s [IP] = %s banned for %d seconds.", stamp, b.Email, b.Network, minutes*60))
	}
	appendIpLimitLog(lines)
	return true, nil
}

// networksOverLimit keeps the limit's worth of most recently seen networks and
// returns the rest; banned networks are out of the count while their ban runs.
func networksOverLimit(seen map[string]int64, banned map[string]bool, limit int) []string {
	type network struct {
		key      string
		lastSeen int64
	}
	nets := make([]network, 0, len(seen))
	for key, at := range seen {
		if !banned[key] {
			nets = append(nets, network{key, at})
		}
	}
	if len(nets) <= limit {
		return nil
	}
	sort.Slice(nets, func(i, j int) bool {
		if nets[i].lastSeen != nets[j].lastSeen {
			return nets[i].lastSeen < nets[j].lastSeen
		}
		return nets[i].key < nets[j].key
	})
	out := make([]string, 0, len(nets)-limit)
	for _, n := range nets[:len(nets)-limit] {
		out = append(out, n.key)
	}
	return out
}

// liveNetworks folds relay identities into their owners and drops exempt
// addresses: owner email -> counted network -> most recent lastSeen.
func (s *IpLimitService) liveNetworks(now time.Time, observed []IpObservation) (map[string]map[string]int64, error) {
	if len(observed) == 0 {
		return nil, nil
	}
	owners, err := s.ChainTransitOwners()
	if err != nil {
		return nil, err
	}
	ex, err := s.exemptions(now)
	if err != nil {
		return nil, err
	}
	live := map[string]map[string]int64{}
	for _, o := range observed {
		email := o.Email
		if owner, ok := owners[email]; ok {
			email = owner
		}
		network, addr, ok := ipLimitNetwork(o.IP)
		if !ok || email == "" {
			continue
		}
		if kind, _ := ex.reason(addr); kind != "" {
			continue
		}
		if live[email] == nil {
			live[email] = map[string]int64{}
		}
		if cur, seen := live[email][network]; !seen || o.LastSeen > cur {
			live[email][network] = o.LastSeen
		}
	}
	return live, nil
}

func clientIpLimits(db *gorm.DB, emails []string) (map[string]int, error) {
	out := make(map[string]int, len(emails))
	for _, batch := range chunkStrings(emails, sqlInChunk) {
		var rows []struct {
			Email   string
			LimitIp int
		}
		err := db.Model(&model.ClientRecord{}).Select("email, limit_ip").
			Where("email IN ? AND limit_ip > 0 AND enable = ?", batch, true).Scan(&rows).Error
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			out[r.Email] = r.LimitIp
		}
	}
	return out, nil
}

func activeBanSet(db *gorm.DB, now int64) (map[string]map[string]bool, error) {
	var bans []model.ClientIpBan
	if err := db.Where("expires_at > ?", now).Find(&bans).Error; err != nil {
		return nil, err
	}
	out := map[string]map[string]bool{}
	for _, b := range bans {
		if out[b.Email] == nil {
			out[b.Email] = map[string]bool{}
		}
		out[b.Email][b.Network] = true
	}
	return out, nil
}

// activeIpLimitBans are the bans the configs carry: unexpired, on a client that
// still has a limit and is enabled.
func activeIpLimitBans(db *gorm.DB, now int64) ([]model.ClientIpBan, error) {
	var bans []model.ClientIpBan
	err := db.Table("client_ip_bans").Select("client_ip_bans.*").
		Joins("JOIN clients ON clients.email = client_ip_bans.email").
		Where("client_ip_bans.expires_at > ? AND clients.limit_ip > 0 AND clients.enable = ?", now, true).
		Order("client_ip_bans.email, client_ip_bans.network").
		Find(&bans).Error
	return bans, err
}

// liftLapsedIpBans deletes bans that expired or whose client no longer has a
// limit (or is gone, disabled or renamed), logging each one it lifts.
func liftLapsedIpBans(db *gorm.DB, now int64) ([]model.ClientIpBan, error) {
	var lapsed []model.ClientIpBan
	err := db.Where("expires_at <= ? OR email NOT IN (?)", now,
		db.Model(&model.ClientRecord{}).Select("email").Where("limit_ip > 0 AND enable = ?", true)).
		Find(&lapsed).Error
	if err != nil || len(lapsed) == 0 {
		return nil, err
	}
	ids := make([]int, 0, len(lapsed))
	for _, b := range lapsed {
		ids = append(ids, b.Id)
	}
	if err := db.Where("id IN ?", ids).Delete(&model.ClientIpBan{}).Error; err != nil {
		return nil, err
	}
	logUnbans(time.Unix(now, 0), lapsed)
	return lapsed, nil
}

func logUnbans(now time.Time, bans []model.ClientIpBan) {
	stamp := now.Format("2006/01/02 15:04:05")
	lines := make([]string, 0, len(bans))
	for _, b := range bans {
		lines = append(lines, fmt.Sprintf("%s   UNBAN   [Email] = %s [IP] = %s unbanned.", stamp, b.Email, b.Network))
	}
	appendIpLimitLog(lines)
}

// appendIpLimitLog keeps the wording the fail2ban action used, so the daily log
// rotation and the Telegram backup carry these lines unchanged.
func appendIpLimitLog(lines []string) {
	if len(lines) == 0 {
		return
	}
	f, err := os.OpenFile(xray.GetIPLimitBannedLogPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		logger.Warning("[LimitIP] could not open the ban log:", err)
		return
	}
	defer f.Close()
	if _, err := f.WriteString(strings.Join(lines, "\n") + "\n"); err != nil {
		logger.Warning("[LimitIP] could not write the ban log:", err)
	}
}

// IpLimitExemptHost is one Pigger server whose addresses the IP limit never
// counts or bans; Name is "" for this panel's own host.
type IpLimitExemptHost struct {
	Name      string   `json:"name" example:"hk-relay"`
	Addresses []string `json:"addresses" example:"[\"203.0.113.7\",\"2001:db8:1:2::/64\"]"`
}

// ExemptHosts lists the server addresses exempt from the IP limit, this panel
// first, so an operator can check that every relay is covered.
func (s *IpLimitService) ExemptHosts() ([]IpLimitExemptHost, error) {
	ex, err := s.exemptions(time.Now())
	if err != nil {
		return nil, err
	}
	var out []IpLimitExemptHost
	for _, h := range ex.hosts {
		addr := h.prefix.String()
		if h.prefix.Addr().Is4() {
			addr = h.prefix.Addr().String()
		}
		if n := len(out); n > 0 && out[n-1].Name == h.name {
			if !slices.Contains(out[n-1].Addresses, addr) {
				out[n-1].Addresses = append(out[n-1].Addresses, addr)
			}
			continue
		}
		out = append(out, IpLimitExemptHost{Name: h.name, Addresses: []string{addr}})
	}
	return out, nil
}

// BansForEmail lists a client's running bans, oldest network first.
func (s *IpLimitService) BansForEmail(email string, now time.Time) ([]model.ClientIpBan, error) {
	var bans []model.ClientIpBan
	err := database.GetDB().Where("email = ? AND expires_at > ?", email, now.Unix()).
		Order("network").Find(&bans).Error
	return bans, err
}

// Unban lifts one ban before it expires; the next config sync drops the rule.
func (s *IpLimitService) Unban(email, network string) error {
	db := database.GetDB()
	var ban model.ClientIpBan
	if err := db.Where("email = ? AND network = ?", email, network).First(&ban).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errIpNotBanned
		}
		return err
	}
	if err := db.Delete(&ban).Error; err != nil {
		return err
	}
	logUnbans(time.Now(), []model.ClientIpBan{ban})
	return nil
}

// injectIpLimitBans puts each banned client's networks first in routing, for the
// users this config serves, their relay identities included.
func injectIpLimitBans(cfg *xray.Config, transitOwners map[string]string, now time.Time) error {
	bans, err := activeIpLimitBans(database.GetDB(), now.Unix())
	if err != nil || len(bans) == 0 {
		return err
	}
	identities := map[string][]string{}
	for _, email := range servedEmails(cfg) {
		owner := email
		if o, ok := transitOwners[email]; ok {
			owner = o
		}
		identities[owner] = append(identities[owner], email)
	}
	networks := map[string][]string{}
	var owners []string
	for _, b := range bans {
		if _, served := identities[b.Email]; !served {
			continue
		}
		if networks[b.Email] == nil {
			owners = append(owners, b.Email)
		}
		networks[b.Email] = append(networks[b.Email], b.Network)
	}
	if len(owners) == 0 {
		return nil
	}
	rules := make([]any, 0, len(owners))
	for _, owner := range owners {
		users := slices.Clone(identities[owner])
		slices.Sort(users)
		users = slices.Compact(users)
		rules = append(rules, map[string]any{
			"type": "field", "user": users, "sourceIP": networks[owner], "outboundTag": ipLimitBlockOutboundTag,
		})
	}
	return prependRoutingWithOutbound(cfg, rules, map[string]any{
		"tag": ipLimitBlockOutboundTag, "protocol": "blackhole", "settings": map[string]any{},
	})
}

// servedEmails lists the client emails of every inbound in cfg.
func servedEmails(cfg *xray.Config) []string {
	var out []string
	for _, ib := range cfg.InboundConfigs {
		var settings struct {
			Clients []struct {
				Email string `json:"email"`
			} `json:"clients"`
		}
		if len(ib.Settings) == 0 || json.Unmarshal(ib.Settings, &settings) != nil {
			continue
		}
		for _, c := range settings.Clients {
			if c.Email != "" {
				out = append(out, c.Email)
			}
		}
	}
	return out
}
