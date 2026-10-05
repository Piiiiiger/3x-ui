package agent

import (
	"context"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/agentproto"
)

func probeTCP(p *agentproto.Probe) agentproto.Result {
	if p == nil || strings.TrimSpace(p.Host) == "" || p.Port < 1 || p.Port > 65535 {
		return agentproto.Result{Error: "invalid TCP probe endpoint"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(strings.Trim(p.Host, "[]"), strconv.Itoa(p.Port)))
	if err != nil {
		return agentproto.Result{Error: err.Error()}
	}
	conn.Close()
	return agentproto.Result{OK: true}
}
