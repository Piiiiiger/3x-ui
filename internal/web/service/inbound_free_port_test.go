package service

import (
	"fmt"
	"net"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func startAt(offset int) func(int) int { return func(int) int { return offset } }

// A suggested port must be free for both transports on that host, or a node
// generated there would fail to start next to an inbound already on it.
func TestFreePortSkipsPortsTheHostAlreadyUses(t *testing.T) {
	setupConflictDB(t)
	agent := seedAgentNodeRow(t, "edge-hk")
	seedInboundConflictNode(t, "n1-tcp", "", 20000, model.VLESS, `{"network":"tcp"}`, `{}`, &agent.Id)
	seedInboundConflictNode(t, "n1-udp", "", 20001, model.Hysteria, `{}`, `{}`, &agent.Id)
	seedInboundConflictNode(t, "local", "", 20002, model.VLESS, `{"network":"tcp"}`, `{}`, nil)

	for start := range 3 {
		got, err := freePortIn(database.GetDB(), &agent.Id, 20000, 20002, startAt(start))
		if err != nil {
			t.Fatalf("scan from offset %d: %v", start, err)
		}
		if got.Port != 20002 {
			t.Fatalf("scan from offset %d suggested %d; 20000 (TCP) and 20001 (UDP) are taken on this host, 20002 only on another", start, got.Port)
		}
	}
}

// The template's API and metrics listeners are no inbound rows, so only their
// reservation keeps a suggestion off them on an agent.
func TestFreePortSkipsTheTemplateListeners(t *testing.T) {
	setupConflictDB(t)
	agent := seedAgentNodeRow(t, "edge-hk")
	for _, held := range []int{defaultXrayAPIPort, 11111} {
		got, err := freePortIn(database.GetDB(), &agent.Id, held, held+1, startAt(0))
		if err != nil {
			t.Fatalf("range %d-%d: %v", held, held+1, err)
		}
		if got.Port != held+1 {
			t.Fatalf("suggested %d, which the template's own listener holds on every agent", got.Port)
		}
	}
}

// Programs outside the panel, such as haproxy or sing-box, hold ports the database
// knows nothing about; only the local panel's own machine can be asked.
func TestFreePortOnTheLocalPanelSkipsAPortTheMachineHolds(t *testing.T) {
	setupConflictDB(t)
	held, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	port := held.Addr().(*net.TCPAddr).Port

	_, err = freePortIn(database.GetDB(), nil, port, port, startAt(0))
	if want := fmt.Sprintf("no free port between %d and %d on this host; enter one by hand", port, port); err == nil || err.Error() != want {
		t.Fatalf("local panel with %d held by another program: error %v, want %q", port, err, want)
	}

	agent := seedAgentNodeRow(t, "edge-hk")
	got, err := freePortIn(database.GetDB(), &agent.Id, port, port, startAt(0))
	if err != nil || got.Port != port {
		t.Fatalf("agent host: got %d, %v; this machine's sockets say nothing about an agent's", got.Port, err)
	}
}

// A UDP-only program, such as a WireGuard or Hysteria server, blocks the port as
// surely as a TCP listener does.
func TestFreePortOnTheLocalPanelSkipsAUDPPortTheMachineHolds(t *testing.T) {
	setupConflictDB(t)
	held, err := net.ListenPacket("udp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	port := held.LocalAddr().(*net.UDPAddr).Port

	_, err = freePortIn(database.GetDB(), nil, port, port, startAt(0))
	if want := fmt.Sprintf("no free port between %d and %d on this host; enter one by hand", port, port); err == nil || err.Error() != want {
		t.Fatalf("local panel with UDP %d held by another program: error %v, want %q", port, err, want)
	}
}

// Suggestions are spread over the range like the form's own pick, so two nodes
// generated in a row don't both land on its first port.
func TestFreePortStartsAtTheGivenOffset(t *testing.T) {
	setupConflictDB(t)
	agent := seedAgentNodeRow(t, "edge-hk")
	got, err := freePortIn(database.GetDB(), &agent.Id, 20000, 20002, startAt(1))
	if err != nil || got.Port != 20001 {
		t.Fatalf("scan from offset 1 of a free range: got %d, %v; want 20001", got.Port, err)
	}
}

// A scan that starts near the top of the range must come back round to a free
// port below its start instead of walking past the range.
func TestFreePortWrapsAroundTheRange(t *testing.T) {
	setupConflictDB(t)
	agent := seedAgentNodeRow(t, "edge-hk")
	seedInboundConflictNode(t, "n1-b", "", 20001, model.VLESS, `{"network":"tcp"}`, `{}`, &agent.Id)
	seedInboundConflictNode(t, "n1-c", "", 20002, model.VLESS, `{"network":"tcp"}`, `{}`, &agent.Id)

	got, err := freePortIn(database.GetDB(), &agent.Id, 20000, 20002, startAt(2))
	if err != nil || got.Port != 20000 {
		t.Fatalf("scan from the last port: got %d, %v; want 20000, the one free port in range", got.Port, err)
	}
}

func TestFreePortReportsAFullRange(t *testing.T) {
	setupConflictDB(t)
	agent := seedAgentNodeRow(t, "edge-hk")
	seedInboundConflictNode(t, "n1-a", "", 20000, model.VLESS, `{"network":"tcp"}`, `{}`, &agent.Id)

	_, err := freePortIn(database.GetDB(), &agent.Id, 20000, 20000, startAt(0))
	if want := "no free port between 20000 and 20000 on this host; enter one by hand"; err == nil || err.Error() != want {
		t.Fatalf("full range: error %v, want %q", err, want)
	}
}

func TestFreePortRefusesAnUnknownHost(t *testing.T) {
	setupConflictDB(t)
	missing := 999
	_, err := (&InboundService{}).FreePort(&missing)
	if want := "node not found: 999"; err == nil || err.Error() != want {
		t.Fatalf("unknown host: error %v, want %q", err, want)
	}
}
