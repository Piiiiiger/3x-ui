package service

import (
	"context"
	"math/rand/v2"
	"net"
	"strconv"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"

	"gorm.io/gorm"
)

// FreePortView is a port a new inbound on a host can listen on.
type FreePortView struct {
	Port int `json:"port" example:"34567"`
}

// Suggestions come from the same range the inbound form picks from; the scan
// stops after freePortTries ports so a crowded range answers quickly.
const (
	freePortLow   = 10000
	freePortHigh  = 60000
	freePortTries = 256
)

// FreePort suggests a port no inbound on the host holds for TCP or UDP on any
// interface, so a new inbound there can take it whatever its transport.
func (s *InboundService) FreePort(nodeID *int) (FreePortView, error) {
	return freePortIn(database.GetDB(), nodeID, freePortLow, freePortHigh, rand.IntN)
}

func freePortIn(db *gorm.DB, nodeID *int, low, high int, offset func(int) int) (FreePortView, error) {
	if nodeID != nil {
		var found int64
		if err := db.Model(&model.Node{}).Where("id = ?", *nodeID).Count(&found).Error; err != nil {
			return FreePortView{}, err
		}
		if found == 0 {
			return FreePortView{}, common.NewErrorf("node not found: %d", *nodeID)
		}
	}
	span := high - low + 1
	start := offset(span)
	for i := range min(span, freePortTries) {
		port := low + (start+i)%span
		probe := &model.Inbound{NodeID: nodeID, Port: port, Protocol: model.Shadowsocks, Settings: `{"network":"tcp,udp"}`}
		conflict, err := checkPortConflictTx(db, probe, 0)
		if err != nil {
			return FreePortView{}, err
		}
		if conflict != nil || (nodeID == nil && !machineCanBind("", port, transportTCP|transportUDP)) {
			continue
		}
		return FreePortView{Port: port}, nil
	}
	return FreePortView{}, common.NewErrorf("no free port between %d and %d on this host; enter one by hand", low, high)
}

// machineCanBind asks this machine, which only the local panel's Xray shares, as
// haproxy or sing-box hold ports no inbound row records; a socket path is no port.
func machineCanBind(listen string, port int, bits transportBits) bool {
	if strings.HasPrefix(listen, "/") || strings.HasPrefix(listen, "@") {
		return true
	}
	var lc net.ListenConfig
	addr := net.JoinHostPort(listen, strconv.Itoa(port))
	if bits&transportTCP != 0 {
		tcp, err := lc.Listen(context.Background(), "tcp", addr)
		if err != nil {
			return false
		}
		_ = tcp.Close()
	}
	if bits&transportUDP != 0 {
		udp, err := lc.ListenPacket(context.Background(), "udp", addr)
		if err != nil {
			return false
		}
		_ = udp.Close()
	}
	return true
}
