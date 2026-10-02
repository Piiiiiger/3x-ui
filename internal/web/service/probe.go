package service

import (
	"context"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
)

// Package variables so tests can move the clock and shorten the timeout.
var (
	probeNow          = time.Now
	probeFetchTimeout = 3 * time.Second
)

const (
	// probeFreshFor also applies to a failure, so a hung Lite is not retried by every poll.
	probeFreshFor = 2 * time.Second
	probeStaleFor = 90 * time.Second
	// Pings run once a minute, and their history costs Lite more than the rest of the batch.
	probeBlocksFreshFor = 30 * time.Second

	probeBlockSpan  = 5 * time.Minute
	probeBlockCount = 12

	probeServerIdMaxLen = 64
)

const (
	probeStatusOnline      = "online"
	probeStatusOffline     = "offline"
	probeStatusUnknown     = "unknown"
	probeStatusUnmonitored = "unmonitored"
)

// ProbePingBlock is one span of a ping task's hour, bounded in unix ms: the
// checks that ran in it and the share of them lost, in percent.
type ProbePingBlock struct {
	Start  int64   `json:"start" example:"1735689600000"`
	End    int64   `json:"end" example:"1735689900000"`
	Checks int     `json:"checks" example:"5"`
	Loss   float64 `json:"loss" example:"20"`
}

// ProbePing is one ping task of a server over the last hour: latency is the
// average in ms (-1 when no reply came back at all), loss is a percentage.
// Blocks is that hour oldest first, empty when Lite gave no history.
type ProbePing struct {
	Id      int              `json:"id" example:"2"`
	Name    string           `json:"name" example:"China Telecom"`
	Latency int              `json:"latency" example:"31"`
	Loss    float64          `json:"loss" example:"0.4"`
	Blocks  []ProbePingBlock `json:"blocks"`
}

// ProbeServer is one server of the Lite monitor. Its metrics are zero unless
// status is online; linked with nodeId 0 means the panel's own host.
type ProbeServer struct {
	Id             string      `json:"id" example:"00000000-0000-4000-8000-000000000001"`
	Name           string      `json:"name" example:"hk-1"`
	Region         string      `json:"region" example:"🇭🇰"`
	OS             string      `json:"os" example:"Debian GNU/Linux 13 (trixie)"`
	Arch           string      `json:"arch" example:"amd64"`
	Virtualization string      `json:"virtualization" example:"kvm"`
	CpuCores       int         `json:"cpuCores" example:"2"`
	Status         string      `json:"status" validate:"oneof=online offline unknown" example:"online"`
	UpdatedAt      int64       `json:"updatedAt" example:"1735689600000"`
	Cpu            float64     `json:"cpu" example:"12.5"`
	MemUsed        int64       `json:"memUsed" example:"858993459"`
	MemTotal       int64       `json:"memTotal" example:"2147483648"`
	DiskUsed       int64       `json:"diskUsed" example:"8589934592"`
	DiskTotal      int64       `json:"diskTotal" example:"42949672960"`
	Load1          float64     `json:"load1" example:"0.31"`
	Load5          float64     `json:"load5" example:"0.22"`
	Load15         float64     `json:"load15" example:"0.18"`
	NetIn          int64       `json:"netIn" example:"5678"`
	NetOut         int64       `json:"netOut" example:"1234"`
	NetTotalUp     int64       `json:"netTotalUp" example:"1000000"`
	NetTotalDown   int64       `json:"netTotalDown" example:"2000000"`
	Uptime         int64       `json:"uptime" example:"86400"`
	TrafficLimit   int64       `json:"trafficLimit" example:"107374182400"`
	TrafficUsed    int64       `json:"trafficUsed" example:"3000000"`
	Pings          []ProbePing `json:"pings"`
	Linked         bool        `json:"linked" example:"true"`
	NodeId         int         `json:"nodeId" example:"2"`
	NodeName       string      `json:"nodeName" example:"edge-hk"`
}

// ProbeOverview is the admin page's data: every server Lite lists. A failed
// fetch is reported in error, next to the last good servers while stale is set.
type ProbeOverview struct {
	Configured bool          `json:"configured" example:"true"`
	PublicURL  string        `json:"publicUrl" example:"https://probe.example.com"`
	FetchedAt  int64         `json:"fetchedAt" example:"1735689600000"`
	Stale      bool          `json:"stale" example:"false"`
	Error      string        `json:"error" example:""`
	Servers    []ProbeServer `json:"servers"`
}

// ProbeLinkView is one host of this panel (node id 0 is the panel itself) and
// the Lite server linked to it; serverName is empty when Lite no longer lists it.
type ProbeLinkView struct {
	NodeId     int    `json:"nodeId" example:"2"`
	NodeName   string `json:"nodeName" example:"edge-hk"`
	Address    string `json:"address" example:"203.0.113.7"`
	ServerId   string `json:"serverId" example:"00000000-0000-4000-8000-000000000001"`
	ServerName string `json:"serverName" example:"hk-1"`
}

// ProbeLinkInput links one host to a Lite server; an empty serverId unlinks it.
type ProbeLinkInput struct {
	NodeId   int    `json:"nodeId" example:"2"`
	ServerId string `json:"serverId" example:"00000000-0000-4000-8000-000000000001"`
}

// ProbeLinksInput is the whole set of links; saving it replaces the stored set.
type ProbeLinksInput struct {
	Links []ProbeLinkInput `json:"links"`
}

// PortalProbeServer is one host of the signed-in client, named by its inbound
// remarks. The field list is the privacy whitelist: nothing else of Lite is sent.
type PortalProbeServer struct {
	Id           int         `json:"id" example:"2"`
	Name         string      `json:"name" example:"Hong Kong"`
	Status       string      `json:"status" validate:"oneof=online offline unknown unmonitored" example:"online"`
	Region       string      `json:"region" example:"🇭🇰"`
	UpdatedAt    int64       `json:"updatedAt" example:"1735689600000"`
	Cpu          float64     `json:"cpu" example:"12.5"`
	MemUsed      int64       `json:"memUsed" example:"858993459"`
	MemTotal     int64       `json:"memTotal" example:"2147483648"`
	DiskUsed     int64       `json:"diskUsed" example:"8589934592"`
	DiskTotal    int64       `json:"diskTotal" example:"42949672960"`
	Load1        float64     `json:"load1" example:"0.31"`
	Load5        float64     `json:"load5" example:"0.22"`
	Load15       float64     `json:"load15" example:"0.18"`
	NetIn        int64       `json:"netIn" example:"5678"`
	NetOut       int64       `json:"netOut" example:"1234"`
	NetTotalUp   int64       `json:"netTotalUp" example:"1000000"`
	NetTotalDown int64       `json:"netTotalDown" example:"2000000"`
	Uptime       int64       `json:"uptime" example:"86400"`
	Pings        []ProbePing `json:"pings"`
}

// PortalProbe is what the client portal shows: the client's own hosts. enabled
// is false while no Lite address is set; stale marks figures from fetchedAt.
type PortalProbe struct {
	Enabled   bool                `json:"enabled" example:"true"`
	FetchedAt int64               `json:"fetchedAt" example:"1735689600000"`
	Stale     bool                `json:"stale" example:"false"`
	Servers   []PortalProbeServer `json:"servers"`
}

// ProbeSnapshot is what Lite said at one moment. Its servers carry no link
// data and are shared between callers, so they must never be modified.
type ProbeSnapshot struct {
	Servers   []ProbeServer
	FetchedAt int64
	Stale     bool
	Error     string
}

// probeFlight is one fetch that every caller arriving meanwhile waits for. It
// reuses the blocks it was given unless askBlocks sends it for new ones.
type probeFlight struct {
	url       string
	done      chan struct{}
	snap      ProbeSnapshot
	blocks    liteBlocks
	askBlocks bool
}

// probeCache is shared by the panel and the subscription server, which run in
// one process. It holds the last answer of one Lite address, good or failed.
var probeCache struct {
	sync.Mutex
	url      string
	snap     ProbeSnapshot
	at       time.Time
	flight   *probeFlight
	blocks   liteBlocks
	blocksAt time.Time
}

// dropProbeCache also disowns a fetch in the air, so an answer asked for under
// the old settings is never stored.
func dropProbeCache() {
	probeCache.Lock()
	defer probeCache.Unlock()
	probeCache.url, probeCache.snap, probeCache.at, probeCache.flight = "", ProbeSnapshot{}, time.Time{}, nil
	probeCache.blocks, probeCache.blocksAt = nil, time.Time{}
}

// ProbeService reads server status from a Lite monitor on the panel's host
// and scopes it to the hosts a client's own inbounds run on.
type ProbeService struct {
	settingService SettingService
}

func (s *ProbeService) Settings() (ProbeSettings, error) {
	return s.settingService.GetProbeSettings()
}

func (s *ProbeService) SaveSettings(in ProbeSettings) (ProbeSettings, error) {
	saved, err := s.settingService.SaveProbeSettings(in)
	if err != nil {
		return ProbeSettings{}, err
	}
	dropProbeCache()
	return saved, nil
}

// Snapshot returns Lite's answer, at most probeFreshFor old. configured is
// false, and Lite is not contacted, while no Lite address is set.
func (s *ProbeService) Snapshot(ctx context.Context) (snap ProbeSnapshot, configured bool, err error) {
	liteURL, err := s.settingService.getString(settingProbeLiteURL)
	if err != nil || liteURL == "" {
		return ProbeSnapshot{}, false, err
	}
	snap, err = probeSnapshotFrom(ctx, liteURL)
	return snap, true, err
}

func probeSnapshotFrom(ctx context.Context, liteURL string) (ProbeSnapshot, error) {
	probeCache.Lock()
	if probeCache.url == liteURL && !probeCache.at.IsZero() && probeNow().Sub(probeCache.at) < probeFreshFor {
		snap := probeCache.snap
		probeCache.Unlock()
		return snap, nil
	}
	flight := probeCache.flight
	if flight == nil || flight.url != liteURL {
		flight = &probeFlight{url: liteURL, done: make(chan struct{}), askBlocks: true}
		if probeCache.url == liteURL && probeNow().Sub(probeCache.blocksAt) < probeBlocksFreshFor {
			flight.blocks, flight.askBlocks = probeCache.blocks, false
		}
		probeCache.flight = flight
		// The fetch outlives the caller that started it: others may be waiting.
		fetchCtx, timeout := context.WithoutCancel(ctx), probeFetchTimeout
		common.GoRecover("probe-fetch", func() { flight.run(fetchCtx, timeout) })
	}
	probeCache.Unlock()
	select {
	case <-flight.done:
		return flight.snap, nil
	case <-ctx.Done():
		return ProbeSnapshot{}, ctx.Err()
	}
}

func (f *probeFlight) run(ctx context.Context, timeout time.Duration) {
	// Settled in a defer so that even a panic wakes the waiters and frees the slot.
	err := errLiteUnexpected
	var servers []ProbeServer
	blocks := f.blocks
	defer func() { f.settle(servers, blocks, err) }()
	servers, blocks, err = fetchLite(ctx, f.url, timeout, f.blocks, f.askBlocks)
}

// settle stores the answer unless the cache was dropped or moved to another
// address meanwhile. A failure keeps the last good servers while they are young.
func (f *probeFlight) settle(servers []ProbeServer, blocks liteBlocks, err error) {
	now := probeNow()
	probeCache.Lock()
	defer probeCache.Unlock()
	defer close(f.done)
	f.snap = ProbeSnapshot{Servers: servers, FetchedAt: now.UnixMilli()}
	if err != nil {
		f.snap = ProbeSnapshot{Servers: []ProbeServer{}, Error: err.Error()}
		last := probeCache.snap
		if probeCache.url == f.url && last.FetchedAt != 0 && now.UnixMilli()-last.FetchedAt < probeStaleFor.Milliseconds() {
			f.snap.Servers, f.snap.FetchedAt, f.snap.Stale = last.Servers, last.FetchedAt, true
		}
	}
	if probeCache.flight == f {
		probeCache.url, probeCache.snap, probeCache.at, probeCache.flight = f.url, f.snap, now, nil
		// A Lite without history is not asked again by the next poll either.
		if f.askBlocks && err == nil {
			probeCache.blocks, probeCache.blocksAt = blocks, now
		}
	}
}

// probeHost is a place inbounds run: the master (node id 0) or a node.
type probeHost struct {
	nodeId  int
	name    string
	address string
}

// probeHosts lists the master first, then every node by id, named by its
// remark when it has one.
func probeHosts(db *gorm.DB) ([]probeHost, error) {
	var nodes []model.Node
	if err := db.Model(&model.Node{}).Select("id", "name", "remark", "address").Order("id ASC").Find(&nodes).Error; err != nil {
		return nil, err
	}
	hosts := make([]probeHost, 1, len(nodes)+1)
	for _, node := range nodes {
		name := strings.TrimSpace(node.Remark)
		if name == "" {
			name = node.Name
		}
		hosts = append(hosts, probeHost{nodeId: node.Id, name: name, address: node.Address})
	}
	return hosts, nil
}

// probeLinkMap reads every link keyed by node id. GORM treats a zero value in
// a struct condition as unset, so the master's row is only ever found this way.
func probeLinkMap(db *gorm.DB) (map[int]string, error) {
	var rows []model.ProbeLink
	if err := db.Find(&rows).Error; err != nil {
		return nil, err
	}
	links := make(map[int]string, len(rows))
	for _, row := range rows {
		links[row.NodeId] = row.ServerId
	}
	return links, nil
}

// Links lists the master and every node with the Lite server each is linked to.
func (s *ProbeService) Links(ctx context.Context) ([]ProbeLinkView, error) {
	db := database.GetDB()
	hosts, err := probeHosts(db)
	if err != nil {
		return nil, err
	}
	links, err := probeLinkMap(db)
	if err != nil {
		return nil, err
	}
	snap, _, err := s.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	names := make(map[string]string, len(snap.Servers))
	for _, server := range snap.Servers {
		names[server.Id] = server.Name
	}
	views := make([]ProbeLinkView, 0, len(hosts))
	for _, host := range hosts {
		serverId := links[host.nodeId]
		views = append(views, ProbeLinkView{
			NodeId:     host.nodeId,
			NodeName:   host.name,
			Address:    host.address,
			ServerId:   serverId,
			ServerName: names[serverId],
		})
	}
	return views, nil
}

// SetLinks replaces the whole set in one transaction, so two hosts can swap
// servers in one save without tripping the unique server id.
func (s *ProbeService) SetLinks(in ProbeLinksInput) error {
	return database.GetDB().Transaction(func(tx *gorm.DB) error {
		rows, err := probeLinkRows(tx, in.Links)
		if err != nil {
			return err
		}
		if err := tx.Where("1 = 1").Delete(&model.ProbeLink{}).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		return tx.Create(&rows).Error
	})
}

func probeLinkRows(tx *gorm.DB, links []ProbeLinkInput) ([]model.ProbeLink, error) {
	var nodeIds []int
	if err := tx.Model(&model.Node{}).Pluck("id", &nodeIds).Error; err != nil {
		return nil, err
	}
	exists := map[int]bool{0: true}
	for _, id := range nodeIds {
		exists[id] = true
	}
	listed := make(map[int]bool, len(links))
	taken := make(map[string]bool, len(links))
	rows := make([]model.ProbeLink, 0, len(links))
	for _, link := range links {
		serverId := strings.TrimSpace(link.ServerId)
		switch {
		case !exists[link.NodeId]:
			return nil, common.NewErrorf("node %d does not exist", link.NodeId)
		case listed[link.NodeId]:
			return nil, common.NewErrorf("node %d is listed twice", link.NodeId)
		case utf8.RuneCountInString(serverId) > probeServerIdMaxLen:
			return nil, common.NewErrorf("a server id is longer than %d characters", probeServerIdMaxLen)
		case taken[serverId]:
			return nil, common.NewErrorf("server %s is linked to more than one node", serverId)
		}
		listed[link.NodeId] = true
		if serverId == "" {
			continue
		}
		taken[serverId] = true
		rows = append(rows, model.ProbeLink{NodeId: link.NodeId, ServerId: serverId})
	}
	return rows, nil
}

// Overview returns every server Lite lists. Link data is joined onto copies:
// the cached servers are shared and hold what Lite said, nothing else.
func (s *ProbeService) Overview(ctx context.Context) (ProbeOverview, error) {
	settings, err := s.settingService.GetProbeSettings()
	if err != nil {
		return ProbeOverview{}, err
	}
	overview := ProbeOverview{Configured: settings.URL != "", PublicURL: settings.PublicURL, Servers: []ProbeServer{}}
	if !overview.Configured {
		return overview, nil
	}
	snap, err := probeSnapshotFrom(ctx, settings.URL)
	if err != nil {
		return ProbeOverview{}, err
	}
	overview.FetchedAt, overview.Stale, overview.Error = snap.FetchedAt, snap.Stale, snap.Error
	db := database.GetDB()
	hosts, err := probeHosts(db)
	if err != nil {
		return ProbeOverview{}, err
	}
	links, err := probeLinkMap(db)
	if err != nil {
		return ProbeOverview{}, err
	}
	linkedTo := make(map[string]probeHost, len(links))
	for _, host := range hosts {
		if serverId, linked := links[host.nodeId]; linked {
			linkedTo[serverId] = host
		}
	}
	overview.Servers = make([]ProbeServer, 0, len(snap.Servers))
	for _, server := range snap.Servers {
		if host, linked := linkedTo[server.Id]; linked {
			server.Linked, server.NodeId, server.NodeName = true, host.nodeId, host.name
		}
		overview.Servers = append(overview.Servers, server)
	}
	return overview, nil
}

// clientSubInbounds narrows a query to the inbounds the client's subscription
// lists: attached, enabled, not excluded, and of a protocol with a share link.
func clientSubInbounds(db *gorm.DB, clientId int) *gorm.DB {
	return db.Model(&model.Inbound{}).
		Joins("JOIN client_inbounds ON client_inbounds.inbound_id = inbounds.id").
		Where("client_inbounds.client_id = ? AND inbounds.enable = ? AND inbounds.exclude_from_sub = ? AND inbounds.protocol IN ?",
			clientId, true, false, model.SubscriptionProtocols())
}

// clientProbeHosts groups those inbounds by host in subscription order and
// names each host by its inbound remarks.
func clientProbeHosts(db *gorm.DB, clientId int) ([]probeHost, error) {
	var inbounds []model.Inbound
	if err := clientSubInbounds(db, clientId).
		Select("inbounds.node_id", "inbounds.remark").
		Order("inbounds.sub_sort_index ASC, inbounds.id ASC").
		Find(&inbounds).Error; err != nil {
		return nil, err
	}
	var hosts []probeHost
	position := make(map[int]int, len(inbounds))
	for _, inbound := range inbounds {
		nodeId := 0
		if inbound.NodeID != nil {
			nodeId = *inbound.NodeID
		}
		at, seen := position[nodeId]
		if !seen {
			at = len(hosts)
			position[nodeId] = at
			hosts = append(hosts, probeHost{nodeId: nodeId})
		}
		if remark := strings.TrimSpace(inbound.Remark); remark != "" {
			if hosts[at].name != "" {
				hosts[at].name += " / "
			}
			hosts[at].name += remark
		}
	}
	return hosts, nil
}

// ClientServers is the portal's view and fails closed: a client sees the hosts
// behind its own subscription, and of Lite only the server linked to each.
func (s *ProbeService) ClientServers(ctx context.Context, client *model.ClientRecord) (PortalProbe, error) {
	probe := PortalProbe{Servers: []PortalProbeServer{}}
	liteURL, err := s.settingService.getString(settingProbeLiteURL)
	if err != nil || liteURL == "" {
		return probe, err
	}
	probe.Enabled = true
	if !client.Enable {
		return probe, nil
	}
	db := database.GetDB()
	hosts, err := clientProbeHosts(db, client.Id)
	if err != nil {
		return PortalProbe{}, err
	}
	links, err := probeLinkMap(db)
	if err != nil {
		return PortalProbe{}, err
	}
	snap, err := probeSnapshotFrom(ctx, liteURL)
	if err != nil {
		return PortalProbe{}, err
	}
	// The portal has no error field: stale is its only sign that Lite failed.
	failed := snap.Error != ""
	probe.FetchedAt, probe.Stale = snap.FetchedAt, failed
	listed := make(map[string]ProbeServer, len(snap.Servers))
	for _, server := range snap.Servers {
		listed[server.Id] = server
	}
	for _, host := range hosts {
		entry := PortalProbeServer{Id: host.nodeId, Name: host.name, Status: probeStatusUnmonitored, Pings: []ProbePing{}}
		if serverId, linked := links[host.nodeId]; linked {
			if server, found := listed[serverId]; found {
				entry = portalProbeServer(host, server)
			} else if failed && !snap.Stale {
				// Nothing usable from Lite: a linked host is unknown, not unmonitored.
				entry.Status = probeStatusUnknown
			}
		}
		probe.Servers = append(probe.Servers, entry)
	}
	return probe, nil
}

// portalProbeServer copies the whitelisted fields. The figures of a server
// that is not online are already zero in the snapshot.
func portalProbeServer(host probeHost, server ProbeServer) PortalProbeServer {
	return PortalProbeServer{
		Id:           host.nodeId,
		Name:         host.name,
		Status:       server.Status,
		Region:       server.Region,
		UpdatedAt:    server.UpdatedAt,
		Cpu:          server.Cpu,
		MemUsed:      server.MemUsed,
		MemTotal:     server.MemTotal,
		DiskUsed:     server.DiskUsed,
		DiskTotal:    server.DiskTotal,
		Load1:        server.Load1,
		Load5:        server.Load5,
		Load15:       server.Load15,
		NetIn:        server.NetIn,
		NetOut:       server.NetOut,
		NetTotalUp:   server.NetTotalUp,
		NetTotalDown: server.NetTotalDown,
		Uptime:       server.Uptime,
		Pings:        server.Pings,
	}
}

// ClientHasLinkedHost tells the portal whether to offer the probe view: only
// to a client who would see more than "not monitored". It never asks Lite.
func (s *ProbeService) ClientHasLinkedHost(client *model.ClientRecord) (bool, error) {
	liteURL, err := s.settingService.getString(settingProbeLiteURL)
	if err != nil || liteURL == "" || !client.Enable {
		return false, err
	}
	var linked int64
	err = clientSubInbounds(database.GetDB(), client.Id).
		Joins("JOIN probe_links ON probe_links.node_id = COALESCE(inbounds.node_id, 0)").
		Count(&linked).Error
	return linked > 0, err
}
