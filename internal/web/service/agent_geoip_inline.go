package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/xtls/xray-core/common/geodata"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"

	"github.com/mhsanaei/3x-ui/v3/internal/config"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

var agentPrivateGeoIPCache struct {
	sync.Mutex
	path  string
	info  os.FileInfo
	cidrs []any
}

// Agents synchronize concurrently and frequently. Decode only PRIVATE, and
// retain only its CIDRs; decoding every country per agent exhausts small VPSes.
func loadAgentPrivateGeoIP() ([]any, error) {
	path := filepath.Join(config.GetBinFolderPath(), "geoip.dat")
	agentPrivateGeoIPCache.Lock()
	defer agentPrivateGeoIPCache.Unlock()
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c := &agentPrivateGeoIPCache
	if c.path == path && c.info != nil && os.SameFile(c.info, info) && c.info.Size() == info.Size() && c.info.ModTime().Equal(info.ModTime()) {
		return c.cidrs, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cidrs, err := decodeAgentPrivateGeoIP(data)
	if err != nil {
		return nil, err
	}
	c.path, c.info, c.cidrs = path, info, cidrs
	return cidrs, nil
}

func decodeAgentPrivateGeoIP(data []byte) ([]any, error) {
	var cidrs []any
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			return nil, protowire.ParseError(n)
		}
		data = data[n:]
		length := protowire.ConsumeFieldValue(num, typ, data)
		if length < 0 {
			return nil, protowire.ParseError(length)
		}
		if num == 1 && typ == protowire.BytesType {
			entry, _ := protowire.ConsumeBytes(data)
			code := ""
			for fields := entry; len(fields) > 0; {
				field, kind, size := protowire.ConsumeTag(fields)
				if size < 0 {
					return nil, protowire.ParseError(size)
				}
				fields = fields[size:]
				size = protowire.ConsumeFieldValue(field, kind, fields)
				if size < 0 {
					return nil, protowire.ParseError(size)
				}
				if field == 1 && kind == protowire.BytesType {
					value, _ := protowire.ConsumeBytes(fields)
					code = string(value)
				}
				fields = fields[size:]
			}
			if strings.EqualFold(code, "private") {
				var group geodata.GeoIP
				if err := proto.Unmarshal(entry, &group); err != nil {
					return nil, err
				}
				if group.ReverseMatch {
					return nil, fmt.Errorf("PRIVATE GeoIP entry has unsupported reverse match")
				}
				for _, subnet := range group.Cidr {
					if (len(subnet.Ip) != net.IPv4len && len(subnet.Ip) != net.IPv6len) || subnet.Prefix > uint32(len(subnet.Ip)*8) {
						return nil, fmt.Errorf("PRIVATE GeoIP entry has invalid CIDR")
					}
					cidrs = append(cidrs, fmt.Sprintf("%s/%d", net.IP(subnet.Ip).String(), subnet.Prefix))
				}
			}
		}
		data = data[length:]
	}
	if len(cidrs) == 0 {
		return nil, fmt.Errorf("geoip.dat lacks PRIVATE entry")
	}
	return cidrs, nil
}

// Inline the panel's exact private-network list for agents whose filesystem
// has no geoip.dat. This preserves the block policy instead of dropping it.
func inlineAgentPrivateGeoIP(cfg *xray.Config) error {
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	if !strings.Contains(string(raw), `"geoip:private"`) {
		return nil
	}
	cidrs, err := loadAgentPrivateGeoIP()
	if err != nil {
		return err
	}
	if len(cidrs) == 0 {
		return nil
	}
	var root any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&root); err != nil {
		return err
	}
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, value := range x {
				if k == "ip" {
					if ips, ok := value.([]any); ok {
						out := []any{}
						for _, ip := range ips {
							if name, ok := ip.(string); ok && name == "geoip:private" {
								out = append(out, cidrs...)
							} else {
								out = append(out, ip)
							}
						}
						x[k] = out
					}
				}
				walk(x[k])
			}
		case []any:
			for _, item := range x {
				walk(item)
			}
		}
	}
	walk(root)
	raw, err = json.Marshal(root)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, cfg)
}
