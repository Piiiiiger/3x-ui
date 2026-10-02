package service

import (
	"context"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/web/service/probetest"
)

// Shapes copied from a Lite 2.3.6 answer, including the fields the probe must
// ignore (addresses are never present in the guest view, billing fields are).
const liteTestNodes = `{
  "srv-online": {"uuid":"srv-online","name":"0 first by name","cpu_name":"Xeon","virtualization":"kvm","arch":"amd64",
    "cpu_cores":2,"cpu_physical_cores":1,"os":"Debian GNU/Linux 13 (trixie)","kernel_version":"6.12","gpu_name":"None",
    "region":"🇭🇰","region_override":"","mem_total":2069692416,"swap_total":0,"disk_total":21046689792,"weight":2,
    "price":188,"billing_cycle":30,"auto_renewal":true,"currency":"¥","expired_at":"2026-10-27T16:00:00Z","group":"main",
    "tags":"fast","bandwidth":"","hidden":false,"traffic_limit":1610612736000,"traffic_limit_type":"sum",
    "effective_traffic_limit":1610612736000,"effective_traffic_type":"sum","traffic_reset_day":27},
  "srv-offline": {"uuid":"srv-offline","name":"b offline","virtualization":"lxc","arch":"arm64","cpu_cores":1,
    "os":"Alpine Linux v3.23","region":"🇬🇧","mem_total":268435456,"disk_total":1083179008,"weight":1,
    "traffic_limit":0,"traffic_limit_type":"max"},
  "srv-unknown": {"uuid":"srv-unknown","name":"a unknown","virtualization":"none","arch":"amd64","cpu_cores":22,
    "os":"Linux","region":"🇸🇬","mem_total":33128554496,"disk_total":338308288512,"weight":1,
    "traffic_limit":429496729600,"traffic_limit_type":"max"},
  "srv-bare": {"uuid":"srv-bare","name":"z bare","virtualization":"kvm","arch":"amd64","cpu_cores":4,
    "os":"Ubuntu 24.04.4 LTS","region":"🇺🇸","mem_total":936235008,"disk_total":66478366208,"weight":2,
    "traffic_limit":0,"traffic_limit_type":"sum"}
}`

const liteTestStatuses = `{
  "srv-online": {"client":"srv-online","time":"2026-10-01T19:50:35.101724431Z","cpu":12.5,"gpu":0,
    "ram":858993459,"ram_total":2147483648,"swap":0,"swap_total":0,"load":0.31,"load5":0.22,"load15":0.18,"temp":0,
    "disk":8589934592,"disk_total":42949672960,"net_in":5678,"net_out":1234,"net_total_up":1000000,
    "net_total_down":2000000,"process":12,"connections":17,"connections_udp":0,"online":true,"uptime":86400,
    "ping":{
      "10":{"name":"Shanghai Unicom","latest":40,"avg":38,"tail":0.1,"loss":0.5,"min":30,"max":60},
      "2":{"name":"China Telecom","latest":29,"avg":31.6,"tail":0.12,"loss":25,"min":29,"max":35},
      "7":{"name":"Beijing Mobile","latest":-1,"avg":0,"tail":0,"loss":100,"min":0,"max":0}}},
  "srv-offline": {"client":"srv-offline","time":"2026-10-01T19:40:00Z","cpu":77,"ram":100,"ram_total":200,
    "load":3,"load5":2,"load15":1,"disk":300,"disk_total":400,"net_in":5,"net_out":6,"net_total_up":7,
    "net_total_down":8,"online":false,"uptime":99288,
    "ping":{"2":{"name":"China Telecom","latest":30,"avg":30,"loss":0}}},
  "srv-bare": {"client":"srv-bare","time":"2026-10-01T19:50:35.101724431Z","cpu":0.5,"ram":1024,"ram_total":0,
    "disk":2048,"disk_total":0,"online":true,"uptime":60,"ping":{}}
}`

// liteTestServers is what the fixtures above must map to, in weight-then-name order.
func liteTestServers() []ProbeServer {
	return []ProbeServer{
		{
			Id: "srv-unknown", Name: "a unknown", Region: "🇸🇬", OS: "Linux", Arch: "amd64", Virtualization: "none",
			CpuCores: 22, Status: "unknown", TrafficLimit: 429496729600, Pings: []ProbePing{},
		},
		{
			Id: "srv-offline", Name: "b offline", Region: "🇬🇧", OS: "Alpine Linux v3.23", Arch: "arm64",
			Virtualization: "lxc", CpuCores: 1, Status: "offline", UpdatedAt: 1790883600000, Pings: []ProbePing{},
		},
		{
			Id: "srv-online", Name: "0 first by name", Region: "🇭🇰", OS: "Debian GNU/Linux 13 (trixie)", Arch: "amd64",
			Virtualization: "kvm", CpuCores: 2, Status: "online", UpdatedAt: 1790884235101,
			Cpu: 12.5, MemUsed: 858993459, MemTotal: 2147483648, DiskUsed: 8589934592, DiskTotal: 42949672960,
			Load1: 0.31, Load5: 0.22, Load15: 0.18, NetIn: 5678, NetOut: 1234,
			NetTotalUp: 1000000, NetTotalDown: 2000000, Uptime: 86400,
			TrafficLimit: 1610612736000, TrafficUsed: 3000000, TrafficResetDay: 27,
			Pings: []ProbePing{
				{Id: 2, Name: "China Telecom", Latency: 32, Loss: 25, Blocks: []ProbePingBlock{}},
				{Id: 7, Name: "Beijing Mobile", Latency: -1, Loss: 100, Blocks: []ProbePingBlock{}},
				{Id: 10, Name: "Shanghai Unicom", Latency: 38, Loss: 0.5, Blocks: []ProbePingBlock{}},
			},
		},
		{
			Id: "srv-bare", Name: "z bare", Region: "🇺🇸", OS: "Ubuntu 24.04.4 LTS", Arch: "amd64",
			Virtualization: "kvm", CpuCores: 4, Status: "online", UpdatedAt: 1790884235101,
			Cpu: 0.5, MemUsed: 1024, MemTotal: 936235008, DiskUsed: 2048, DiskTotal: 66478366208, Uptime: 60,
			Pings: []ProbePing{},
		},
	}
}

func fetchTestLite(t *testing.T, url string) ([]ProbeServer, error) {
	t.Helper()
	servers, _, err := fetchLite(context.Background(), url, 3*time.Second, nil, true)
	return servers, err
}

// The hour that ends at 19:52:10 for task 2 of srv-online: a point older than
// the twelve blocks, a gap of forty minutes, and the open point Lite appends.
const liteTestLossHistory = `{"start":"2026-10-01T18:52:10Z","end":"2026-10-01T19:52:10Z","series":[
  {"metric_key":"ping.loss","entity_id":"srv-online","tags":{"task_id":"2"},"interval_seconds":300,"points":[
    {"time":"2026-10-01T18:50:00Z","value":1,"count":3},
    {"time":"2026-10-01T18:55:00Z","value":0,"count":5},
    {"time":"2026-10-01T19:00:00Z","value":0.2,"count":5},
    {"time":"2026-10-01T19:45:00Z","value":1,"count":5},
    {"time":"2026-10-01T19:50:00Z","value":0.5,"count":2},
    {"time":"2026-10-01T19:52:10Z","value":null}]},
  {"metric_key":"ping.latency_ms","entity_id":"srv-online","tags":{"task_id":"7"},"points":[
    {"time":"2026-10-01T19:50:00Z","value":40,"count":5}]}
]}`

func lossBlock(hour, minute, checks int, loss float64) ProbePingBlock {
	start := time.Date(2026, 10, 1, hour, minute, 0, 0, time.UTC)
	return ProbePingBlock{Start: start.UnixMilli(), End: start.Add(5 * time.Minute).UnixMilli(), Checks: checks, Loss: loss}
}

func assertProbeServers(t *testing.T, got, want []ProbeServer) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d servers, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if !reflect.DeepEqual(got[i], want[i]) {
			t.Errorf("server %d:\n got  %+v\n want %+v", i, got[i], want[i])
		}
	}
}

// One server per status, plus an online one whose report carries no totals:
// a wrong field, unit, status rule or order shows up as a differing server.
// This Lite does not know public:queryMetrics, which must cost only the blocks.
func TestFetchLiteMapsEveryServerStatusAndPing(t *testing.T) {
	lite := probetest.NewLite(t)
	lite.Answer(liteTestNodes, liteTestStatuses)

	got, err := fetchTestLite(t, lite.URL)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	assertProbeServers(t, got, liteTestServers())
}

// Twelve blocks of five minutes that end with the running one: a shifted
// window, a kept old point or a skipped gap shows up as a differing block.
func TestFetchLiteCutsTheHourOfAPingTaskIntoBlocks(t *testing.T) {
	lite := probetest.NewLite(t)
	lite.Answer(liteTestNodes, liteTestStatuses)
	lite.AnswerMetrics(liteTestLossHistory)

	got, err := fetchTestLite(t, lite.URL)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	want := liteTestServers()
	want[2].Pings[0].Blocks = []ProbePingBlock{
		lossBlock(18, 55, 5, 0), lossBlock(19, 0, 5, 20),
		lossBlock(19, 5, 0, 0), lossBlock(19, 10, 0, 0), lossBlock(19, 15, 0, 0), lossBlock(19, 20, 0, 0),
		lossBlock(19, 25, 0, 0), lossBlock(19, 30, 0, 0), lossBlock(19, 35, 0, 0), lossBlock(19, 40, 0, 0),
		lossBlock(19, 45, 5, 100), lossBlock(19, 50, 2, 50),
	}
	assertProbeServers(t, got, want)
}

// JSON-RPC lets a server answer a batch in any order; taking the first reply
// as the node list would silently swap the two results.
func TestFetchLiteMatchesBatchRepliesById(t *testing.T) {
	lite := probetest.NewLite(t)
	lite.Answer(liteTestNodes, liteTestStatuses)
	lite.AnswerBackward()

	got, err := fetchTestLite(t, lite.URL)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	assertProbeServers(t, got, liteTestServers())
}

// Lite sends the ping tasks as an object, and Go map order is random: without
// the numeric sort the tags on the card would reshuffle on every poll.
func TestFetchLiteKeepsPingTasksInIdOrderOnEveryFetch(t *testing.T) {
	lite := probetest.NewLite(t)
	lite.Answer(liteTestNodes, liteTestStatuses)

	for attempt := range 20 {
		got, err := fetchTestLite(t, lite.URL)
		if err != nil {
			t.Fatalf("fetch %d: %v", attempt, err)
		}
		if len(got) != 4 {
			t.Fatalf("fetch %d: got %d servers, want 4", attempt, len(got))
		}
		var ids []int
		for _, ping := range got[2].Pings {
			ids = append(ids, ping.Id)
		}
		if !reflect.DeepEqual(ids, []int{2, 7, 10}) {
			t.Fatalf("fetch %d: ping task order = %v, want [2 7 10]", attempt, ids)
		}
	}
}

func TestFetchLiteCountsUsedTrafficByTheLimitType(t *testing.T) {
	for kind, want := range map[string]int64{"sum": 800, "max": 500, "min": 300, "up": 300, "down": 500} {
		t.Run(kind, func(t *testing.T) {
			lite := probetest.NewLite(t)
			lite.Answer(
				`{"s":{"name":"s","traffic_limit":1000,"traffic_limit_type":"`+kind+`"}}`,
				`{"s":{"time":"2026-10-01T19:40:00Z","online":true,"net_total_up":300,"net_total_down":500,"ping":{}}}`,
			)
			got, err := fetchTestLite(t, lite.URL)
			if err != nil {
				t.Fatalf("fetch: %v", err)
			}
			if len(got) != 1 || got[0].TrafficUsed != want {
				t.Fatalf("used with type %s = %+v, want %d", kind, got, want)
			}
		})
	}
}

func TestFetchLiteReportsEachFailureWithAFixedPhrase(t *testing.T) {
	answer := func(status int, body string) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(body))
		}
	}
	tests := []struct {
		name      string
		breakLite func(lite *probetest.Lite)
		want      string
	}{
		{
			name:      "a method Lite does not know",
			breakLite: func(lite *probetest.Lite) { lite.Forget("common:getNodesLatestStatus") },
			want:      "Lite answered JSON-RPC error -32601",
		},
		{
			name: "a server error with a body",
			breakLite: func(lite *probetest.Lite) {
				lite.Override(answer(http.StatusInternalServerError, "panic: secret-internal-detail"))
			},
			want: "Lite answered HTTP 500",
		},
		{
			name: "a private site",
			breakLite: func(lite *probetest.Lite) {
				lite.Override(answer(http.StatusUnauthorized, `{"status":"error","message":"Private site is enabled, please login first."}`))
			},
			want: "the Lite site is private",
		},
		{
			name:      "a page instead of JSON",
			breakLite: func(lite *probetest.Lite) { lite.Override(answer(http.StatusOK, "<html>hello</html>")) },
			want:      "Lite sent an unexpected answer",
		},
		{
			name: "a batch reply without the status result",
			breakLite: func(lite *probetest.Lite) {
				lite.Override(answer(http.StatusOK, `[{"jsonrpc":"2.0","id":1,"result":{}}]`))
			},
			want: "Lite sent an unexpected answer",
		},
		{
			// Valid JSON followed by endless padding: reading it all never ends, and
			// cutting it at exactly the cap would parse as a normal answer.
			name: "an endless body that starts valid",
			breakLite: func(lite *probetest.Lite) {
				lite.Override(func(w http.ResponseWriter, _ *http.Request) {
					_, _ = w.Write([]byte(`[{"jsonrpc":"2.0","id":1,"result":{}},{"jsonrpc":"2.0","id":2,"result":{}}]`))
					padding := []byte(strings.Repeat(" ", 64<<10))
					for {
						if _, err := w.Write(padding); err != nil {
							return
						}
					}
				})
			},
			want: "the Lite answer is larger than 4 MiB",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lite := probetest.NewLite(t)
			tc.breakLite(lite)
			servers, err := fetchTestLite(t, lite.URL)
			if err == nil || err.Error() != tc.want {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
			if servers != nil {
				t.Fatalf("a failed fetch still returned servers: %+v", servers)
			}
		})
	}
}

// A hung Lite must cost one short wait, not the 30 s the HTTP servers allow.
func TestFetchLiteGivesUpAfterTheTimeout(t *testing.T) {
	lite := probetest.NewLite(t)
	lite.Override(func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() })

	started := time.Now()
	_, _, err := fetchLite(context.Background(), lite.URL, 50*time.Millisecond, nil, true)
	if err == nil || err.Error() != "Lite did not answer in 50ms" {
		t.Fatalf("err = %v, want the timeout phrase", err)
	}
	if waited := time.Since(started); waited > 2*time.Second {
		t.Fatalf("the fetch took %s, want it cut at the 50ms timeout", waited)
	}
}

func TestFetchLiteReportsAStoppedLiteAsUnreachable(t *testing.T) {
	lite := probetest.NewLite(t)
	url := lite.URL
	lite.Stop()

	if _, err := fetchTestLite(t, url); err == nil || err.Error() != "Lite is not reachable" {
		t.Fatalf("err = %v, want %q", err, "Lite is not reachable")
	}
}

// Following a redirect would let whatever answers on the Lite port send the
// panel's request to another address.
func TestFetchLiteDoesNotFollowARedirect(t *testing.T) {
	target := probetest.NewLite(t)
	lite := probetest.NewLite(t)
	lite.Override(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/api/rpc2", http.StatusTemporaryRedirect)
	})

	if _, err := fetchTestLite(t, lite.URL); err == nil || err.Error() != "Lite answered HTTP 307" {
		t.Fatalf("err = %v, want %q", err, "Lite answered HTTP 307")
	}
	if n := target.Requests(); n != 0 {
		t.Fatalf("the redirect target received %d requests, want 0", n)
	}
}

// The setting is validated on save, but a value written to the database some
// other way must still never make the panel connect off the loopback interface.
func TestFetchLiteRefusesAnAddressThatIsNotLiteralLoopback(t *testing.T) {
	lite := probetest.NewLite(t)
	for name, url := range map[string]string{
		"a hostname that resolves to loopback": strings.Replace(lite.URL, "127.0.0.1", "localhost", 1),
		"another address":                      "http://192.0.2.1:27777",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := fetchTestLite(t, url)
			if err == nil || err.Error() != "the Lite address is not a loopback address" {
				t.Fatalf("fetch %s: err = %v, want the loopback refusal", url, err)
			}
		})
	}
	if n := lite.Requests(); n != 0 {
		t.Fatalf("Lite received %d requests through a hostname, want 0", n)
	}
}

// Reachable only through a value written straight to the settings table; it
// must fail as a bad address, not as a request to some other URL.
func TestFetchLiteReportsAStoredAddressItCannotParse(t *testing.T) {
	if _, err := fetchTestLite(t, "http://127.0.0.1:27777/%zz"); err == nil || err.Error() != "the Lite address is not valid" {
		t.Fatalf("err = %v, want %q", err, "the Lite address is not valid")
	}
}
