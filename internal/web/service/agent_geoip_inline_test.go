package service

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/xtls/xray-core/common/geodata"
	"google.golang.org/protobuf/proto"

	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

func TestGetAgentXrayConfig_InlinesPrivateNetworkPolicy(t *testing.T) {
	setupSettingTestDB(t)
	bin := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", bin)
	data, err := proto.Marshal(&geodata.GeoIPList{Entry: []*geodata.GeoIP{{
		Code: "PRIVATE",
		Cidr: []*geodata.CIDR{
			{Ip: net.ParseIP("10.0.0.0").To4(), Prefix: 8},
			{Ip: net.ParseIP("fc00::").To16(), Prefix: 7},
		},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "geoip.dat"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	node := seedAgentNodeRow(t, "without-geodata")
	cfg, err := (&XrayService{}).GetAgentXrayConfig(node.Id)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "geoip:private") {
		t.Fatal("agent still depends on geoip.dat")
	}
	var routing struct {
		Rules []struct {
			IP          []string `json:"ip"`
			OutboundTag string   `json:"outboundTag"`
		}
	}
	if err := json.Unmarshal(cfg.RouterConfig, &routing); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, rule := range routing.Rules {
		if slices.Equal(rule.IP, []string{"10.0.0.0/8", "fc00::/7"}) && rule.OutboundTag == "blocked" {
			found = true
		}
	}
	if !found {
		t.Fatal("private network routing block was lost")
	}
	var outbounds []struct {
		Tag      string
		Settings struct {
			FinalRules []struct {
				Action string
				IP     []string `json:"ip"`
			}
		}
	}
	if err := json.Unmarshal(cfg.OutboundConfigs, &outbounds); err != nil {
		t.Fatal(err)
	}
	found = false
	for _, outbound := range outbounds {
		for _, rule := range outbound.Settings.FinalRules {
			if outbound.Tag == "direct" && rule.Action == "block" && slices.Equal(rule.IP, []string{"10.0.0.0/8", "fc00::/7"}) {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("post-resolution private network block was lost")
	}
}

func TestAgentPrivateGeoIPCacheRefreshesAndSupportsConcurrentNodes(t *testing.T) {
	bin := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", bin)
	write := func(ip string) {
		t.Helper()
		data, err := proto.Marshal(&geodata.GeoIPList{Entry: []*geodata.GeoIP{{Code: "PRIVATE", Cidr: []*geodata.CIDR{{Ip: net.ParseIP(ip).To4(), Prefix: 8}}}}})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(bin, "next.dat"), data, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(filepath.Join(bin, "next.dat"), filepath.Join(bin, "geoip.dat")); err != nil {
			t.Fatal(err)
		}
	}
	write("10.0.0.0")
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cidrs, err := loadAgentPrivateGeoIP()
			if err != nil || len(cidrs) != 1 || cidrs[0] != "10.0.0.0/8" {
				t.Errorf("concurrent load = %v, %v", cidrs, err)
			}
		}()
	}
	wg.Wait()
	write("127.0.0.0")
	cidrs, err := loadAgentPrivateGeoIP()
	if err != nil || len(cidrs) != 1 || cidrs[0] != "127.0.0.0/8" {
		t.Fatalf("replacement was not loaded: %v, %v", cidrs, err)
	}
}

func largeAgentGeoIPFixture(t testing.TB) []byte {
	t.Helper()
	other := &geodata.GeoIP{Code: "OTHER"}
	for i := 0; i < 20000; i++ {
		other.Cidr = append(other.Cidr, &geodata.CIDR{Ip: []byte{203, 0, 113, byte(i)}, Prefix: 32})
	}
	data, err := proto.Marshal(&geodata.GeoIPList{Entry: []*geodata.GeoIP{other, {Code: "PRIVATE", Cidr: []*geodata.CIDR{{Ip: []byte{10, 0, 0, 0}, Prefix: 8}}}}})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestAgentPrivateGeoIPDoesNotAllocateOtherCountries(t *testing.T) {
	data := largeAgentGeoIPFixture(t)
	allocs := testing.AllocsPerRun(3, func() {
		cidrs, err := decodeAgentPrivateGeoIP(data)
		if err != nil || len(cidrs) != 1 || cidrs[0] != "10.0.0.0/8" {
			t.Fatalf("decode = %v, %v", cidrs, err)
		}
	})
	if allocs > 100 {
		t.Fatalf("allocated other-country entries: %.0f allocations", allocs)
	}
}

func BenchmarkAgentPrivateGeoIPDecode(b *testing.B) {
	data := largeAgentGeoIPFixture(b)
	b.Run("private-only", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := decodeAgentPrivateGeoIP(data); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("previous-full-database", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var all geodata.GeoIPList
			if err := proto.Unmarshal(data, &all); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func TestInlineAgentPrivateGeoIP_UnavailableDataKeepsPolicy(t *testing.T) {
	bin := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", bin)
	cfg := &xray.Config{RouterConfig: []byte(`{"rules":[{"ip":["geoip:private"],"outboundTag":"blocked"}]}`)}
	before, _ := json.Marshal(cfg)
	if err := inlineAgentPrivateGeoIP(cfg); err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(cfg)
	if string(before) != string(after) {
		t.Fatal("missing panel geodata changed the policy")
	}
	if err := os.WriteFile(filepath.Join(bin, "geoip.dat"), []byte("invalid protobuf"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := inlineAgentPrivateGeoIP(cfg); err == nil {
		t.Fatal("corrupt geodata was accepted")
	}
}
