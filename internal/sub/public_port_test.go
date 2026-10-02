package sub

import (
	"regexp"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

const realityStream = `{"network":"tcp","security":"reality","realitySettings":{"show":false,"target":"www.example.com:443","serverNames":["www.example.com"],"privateKey":"aN8LmSAfqx8q9g7_Vt4-3kDqHHB6VZAn2eW2sCJ3wGE","shortIds":["ab12"],"settings":{"publicKey":"Kp8F4ydBu6x4jnU6Z0pD8oV7kq0pD3W1qQ8m5bZ2t0w","fingerprint":"chrome"}},"tcpSettings":{"header":{"type":"none"}}}`

func setSharePort(t *testing.T, ib *model.Inbound, port int) {
	t.Helper()
	if err := database.GetDB().Model(&model.Inbound{}).Where("id = ?", ib.Id).Update("share_port", port).Error; err != nil {
		t.Fatalf("set share port: %v", err)
	}
}

// subscriptionOutputs renders one subscription in every format people fetch.
func subscriptionOutputs(t *testing.T, subID string) map[string]string {
	t.Helper()
	links, _, _, _, err := NewSubService("").GetSubs(subID, "req.example.com")
	if err != nil {
		t.Fatalf("GetSubs: %v", err)
	}
	clash, _, err := NewSubClashService(NewSubService("")).GetClash(subID, "req.example.com")
	if err != nil {
		t.Fatalf("GetClash: %v", err)
	}
	js, _, err := NewSubJsonService("", "", "", "", NewSubService("")).GetJson(subID, "req.example.com", false)
	if err != nil {
		t.Fatalf("GetJson: %v", err)
	}
	return map[string]string{"raw": strings.Join(links, "\n"), "clash": clash, "json": js}
}

// portDigits matches the inbound port as a whole number, not inside the client's UUID,
// which the fixture derives from the port.
var portDigits = regexp.MustCompile(`([^0-9a-f])4431([^0-9])`)

// A node behind NAT advertises its public port and nothing else about it changes:
// every format renders byte for byte what it did, with the port swapped.
func TestSub_PublicPortOnlyChangesThePort(t *testing.T) {
	seedSubDB(t)
	ib := seedSubInbound(t, "s1", "nat", 4431, 1, realityStream)
	before := subscriptionOutputs(t, "s1")

	setSharePort(t, ib, 20443)
	after := subscriptionOutputs(t, "s1")

	for format, out := range before {
		if !portDigits.MatchString(out) {
			t.Fatalf("%s output never carried the inbound port, the test cannot tell ports apart:\n%s", format, out)
		}
		if want := portDigits.ReplaceAllString(out, "${1}20443${2}"); after[format] != want {
			t.Errorf("%s output with public port 20443:\n%s\nwant the same output with only the port swapped:\n%s", format, after[format], want)
		}
	}
}

// An inbound that lists its own externalProxy endpoints keeps them: those carry
// their own ports, so a share port must not replace them.
func TestSub_PublicPortLeavesExternalProxyEndpoints(t *testing.T) {
	seedSubDB(t)
	stream := `{"network":"tcp","security":"none","externalProxy":[{"forceTls":"same","dest":"cdn.example.com","port":8443,"remark":""}]}`
	ib := seedSubInbound(t, "s1", "cdn", 4432, 1, stream)
	setSharePort(t, ib, 20443)

	out := subscriptionOutputs(t, "s1")["raw"]
	if !strings.Contains(out, "cdn.example.com:8443") || strings.Contains(out, "20443") {
		t.Fatalf("raw link = %s, want the externalProxy endpoint cdn.example.com:8443 and no share port", out)
	}
}
