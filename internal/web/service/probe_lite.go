package service

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	liteRPCPath = "/api/rpc2"
	// One batch for the static server list and the live status, as Lite's own page asks.
	liteBatch    = `[{"jsonrpc":"2.0","id":1,"method":"common:getNodes"},{"jsonrpc":"2.0","id":2,"method":"common:getNodesLatestStatus"}]`
	liteNodesID  = 1
	liteStatusID = 2
	liteMaxBody  = 4 << 20
)

// Failures are fixed phrases plus a status or code, never bytes Lite sent: the
// text is shown in the admin page as it is.
var (
	errLiteAddress     = errors.New("the Lite address is not valid")
	errLiteNotLoopback = errors.New("the Lite address is not a loopback address")
	errLiteUnreachable = errors.New("Lite is not reachable")
	errLitePrivate     = errors.New("the Lite site is private")
	errLiteTooLarge    = errors.New("the Lite answer is larger than 4 MiB")
	errLiteUnexpected  = errors.New("Lite sent an unexpected answer")
)

// liteHTTPClient carries no credential and never leaves the host: no proxy, a
// dialer that accepts loopback only, and redirects are answers, not followed.
var liteHTTPClient = &http.Client{
	Transport: &http.Transport{
		Proxy:       nil,
		DialContext: dialLoopbackOnly,
		// A new connection per fetch: one Lite closed while idle is never reused.
		DisableKeepAlives: true,
	},
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

// dialLoopbackOnly refuses hostnames and other addresses without resolving
// them, so a setting that bypassed validation still cannot be reached.
func dialLoopbackOnly(ctx context.Context, network, address string) (net.Conn, error) {
	target, err := netip.ParseAddrPort(address)
	if err != nil || !target.Addr().IsLoopback() {
		return nil, errLiteNotLoopback
	}
	var dialer net.Dialer
	return dialer.DialContext(ctx, network, address)
}

type liteReply struct {
	ID     int             `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code int `json:"code"`
	} `json:"error"`
}

type liteNode struct {
	Name             string `json:"name"`
	Region           string `json:"region"`
	OS               string `json:"os"`
	Arch             string `json:"arch"`
	Virtualization   string `json:"virtualization"`
	CPUCores         int    `json:"cpu_cores"`
	MemTotal         int64  `json:"mem_total"`
	DiskTotal        int64  `json:"disk_total"`
	TrafficLimit     int64  `json:"traffic_limit"`
	TrafficLimitType string `json:"traffic_limit_type"`
	Weight           int    `json:"weight"`
}

type liteStatus struct {
	Online       bool                `json:"online"`
	Time         time.Time           `json:"time"`
	CPU          float64             `json:"cpu"`
	RAM          int64               `json:"ram"`
	RAMTotal     int64               `json:"ram_total"`
	Disk         int64               `json:"disk"`
	DiskTotal    int64               `json:"disk_total"`
	Load1        float64             `json:"load"`
	Load5        float64             `json:"load5"`
	Load15       float64             `json:"load15"`
	NetIn        int64               `json:"net_in"`
	NetOut       int64               `json:"net_out"`
	NetTotalUp   int64               `json:"net_total_up"`
	NetTotalDown int64               `json:"net_total_down"`
	Uptime       int64               `json:"uptime"`
	Ping         map[string]litePing `json:"ping"`
}

type litePing struct {
	Name   string  `json:"name"`
	Latest float64 `json:"latest"`
	Avg    float64 `json:"avg"`
	Loss   float64 `json:"loss"`
}

// fetchLite reads Lite's guest view: every server it lists, with the latest
// report of those that reported since Lite started.
func fetchLite(ctx context.Context, baseURL string, timeout time.Duration) ([]ProbeServer, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+liteRPCPath, strings.NewReader(liteBatch))
	if err != nil {
		return nil, errLiteAddress
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := liteHTTPClient.Do(req)
	if err != nil {
		return nil, liteTransportError(err, timeout)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, errLitePrivate
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Lite answered HTTP %d", resp.StatusCode)
	}
	// One byte past the cap tells an oversized answer from a truncated one.
	body, err := io.ReadAll(io.LimitReader(resp.Body, liteMaxBody+1))
	if err != nil {
		return nil, liteTransportError(err, timeout)
	}
	if len(body) > liteMaxBody {
		return nil, errLiteTooLarge
	}
	return decodeLiteBatch(body)
}

func liteTransportError(err error, timeout time.Duration) error {
	switch {
	case errors.Is(err, errLiteNotLoopback):
		return errLiteNotLoopback
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("Lite did not answer in %s", timeout)
	default:
		return errLiteUnreachable
	}
}

func decodeLiteBatch(body []byte) ([]ProbeServer, error) {
	var replies []liteReply
	if err := json.Unmarshal(body, &replies); err != nil {
		return nil, errLiteUnexpected
	}
	var nodes map[string]liteNode
	var statuses map[string]liteStatus
	for _, reply := range replies {
		if reply.Error != nil {
			return nil, fmt.Errorf("Lite answered JSON-RPC error %d", reply.Error.Code)
		}
		var err error
		switch reply.ID {
		case liteNodesID:
			err = json.Unmarshal(reply.Result, &nodes)
		case liteStatusID:
			err = json.Unmarshal(reply.Result, &statuses)
		}
		if err != nil {
			return nil, errLiteUnexpected
		}
	}
	if nodes == nil || statuses == nil {
		return nil, errLiteUnexpected
	}
	return mapLiteServers(nodes, statuses), nil
}

// mapLiteServers orders servers by Lite's weight, then name. A server without
// a status entry has not reported since Lite started, which is not an outage.
func mapLiteServers(nodes map[string]liteNode, statuses map[string]liteStatus) []ProbeServer {
	ids := slices.SortedFunc(maps.Keys(nodes), func(a, b string) int {
		return cmp.Or(
			cmp.Compare(nodes[a].Weight, nodes[b].Weight),
			cmp.Compare(nodes[a].Name, nodes[b].Name),
			cmp.Compare(a, b),
		)
	})
	servers := make([]ProbeServer, 0, len(ids))
	for _, id := range ids {
		node := nodes[id]
		server := ProbeServer{
			Id:             id,
			Name:           node.Name,
			Region:         node.Region,
			OS:             node.OS,
			Arch:           node.Arch,
			Virtualization: node.Virtualization,
			CpuCores:       node.CPUCores,
			Status:         probeStatusUnknown,
			TrafficLimit:   node.TrafficLimit,
			Pings:          []ProbePing{},
		}
		if status, reported := statuses[id]; reported {
			server.Status = probeStatusOffline
			if !status.Time.IsZero() {
				server.UpdatedAt = status.Time.UnixMilli()
			}
			if status.Online {
				server.Status = probeStatusOnline
				copyLiteMetrics(&server, node, status)
			}
		}
		servers = append(servers, server)
	}
	return servers
}

func copyLiteMetrics(server *ProbeServer, node liteNode, status liteStatus) {
	server.Cpu = status.CPU
	server.MemUsed = status.RAM
	server.MemTotal = cmp.Or(status.RAMTotal, node.MemTotal)
	server.DiskUsed = status.Disk
	server.DiskTotal = cmp.Or(status.DiskTotal, node.DiskTotal)
	server.Load1 = status.Load1
	server.Load5 = status.Load5
	server.Load15 = status.Load15
	server.NetIn = status.NetIn
	server.NetOut = status.NetOut
	server.NetTotalUp = status.NetTotalUp
	server.NetTotalDown = status.NetTotalDown
	server.Uptime = status.Uptime
	server.TrafficUsed = liteTrafficUsed(node.TrafficLimitType, status.NetTotalUp, status.NetTotalDown)
	server.Pings = mapLitePings(status.Ping)
}

// liteTrafficUsed counts the way Lite does for the quota: by the limit type,
// and as up + down for a type it does not know.
func liteTrafficUsed(limitType string, up, down int64) int64 {
	switch limitType {
	case "up":
		return up
	case "down":
		return down
	case "min":
		return min(up, down)
	case "max":
		return max(up, down)
	default:
		return up + down
	}
}

// mapLitePings sorts by numeric task id. Lite omits a task without samples in
// the hour and sends latest -1 with avg 0 when every sample was lost.
func mapLitePings(stats map[string]litePing) []ProbePing {
	pings := make([]ProbePing, 0, len(stats))
	for key, stat := range stats {
		id, err := strconv.Atoi(key)
		if err != nil {
			continue
		}
		latency := -1
		if stat.Latest >= 0 {
			latency = int(math.Round(stat.Avg))
		}
		pings = append(pings, ProbePing{Id: id, Name: stat.Name, Latency: latency, Loss: stat.Loss})
	}
	slices.SortFunc(pings, func(a, b ProbePing) int { return cmp.Compare(a.Id, b.Id) })
	return pings
}
