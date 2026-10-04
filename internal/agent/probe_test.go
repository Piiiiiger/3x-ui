package agent

import (
	"github.com/mhsanaei/3x-ui/v3/internal/agentproto"
	"net"
	"testing"
)

func TestProbeTCP(t *testing.T) {
	for _, p := range []*agentproto.Probe{nil, {}, {Host: "localhost", Port: 65536}} {
		if probeTCP(p).OK {
			t.Fatal("invalid probe accepted")
		}
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p := &agentproto.Probe{Host: "127.0.0.1", Port: l.Addr().(*net.TCPAddr).Port}
	if got := probeTCP(p); !got.OK {
		t.Fatal(got.Error)
	}
	l.Close()
	if got := probeTCP(p); got.OK || got.Error == "" {
		t.Fatal("closed port reported reachable")
	}
}
