package runtime

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/agentproto"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"

	"github.com/gorilla/websocket"
)

// ErrAgentNotConnected means the node's agent has no live connection.
var ErrAgentNotConnected = errors.New("agent is not connected")

const (
	agentPingInterval = 20 * time.Second
	// agentReadTimeout outlasts two missed pings, so a dead link is noticed
	// even when the TCP connection never reports an error.
	agentReadTimeout  = 60 * time.Second
	agentWriteTimeout = 10 * time.Second
	agentMaxMessage   = 16 << 20
	agentNudgeBuffer  = 64
)

// AgentHandlers are the panel-side consumers of what agents send. Traffic must
// return nil only once the report is stored, since nil is what acks it.
type AgentHandlers struct {
	Traffic func(nodeID int, t *agentproto.Traffic) error
	Status  func(nodeID int, s *agentproto.Status)
	Gone    func(nodeID int)
}

// AgentState is what the panel knows about a connected agent.
type AgentState struct {
	Hello       agentproto.Hello
	Status      agentproto.Status
	StatusAt    time.Time
	ConnectedAt time.Time
	// AppliedHash is the config the agent runs: from its hello, then from every
	// apply it accepted.
	AppliedHash string
	LatencyMs   int
}

// AgentHub keeps one live session per agent node.
type AgentHub struct {
	mu       sync.Mutex
	sessions map[int]*agentSession
	handlers AgentHandlers
	nudges   chan int
}

func NewAgentHub() *AgentHub {
	return &AgentHub{
		sessions: make(map[int]*agentSession),
		nudges:   make(chan int, agentNudgeBuffer),
	}
}

func (h *AgentHub) SetHandlers(handlers AgentHandlers) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.handlers = handlers
}

func (h *AgentHub) currentHandlers() AgentHandlers {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.handlers
}

func (h *AgentHub) session(nodeID int) *agentSession {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.sessions[nodeID]
}

// Attach runs conn as nodeID's session until it ends, replacing any older one.
func (h *AgentHub) Attach(nodeID int, conn *websocket.Conn) {
	s := &agentSession{
		hub:     h,
		nodeID:  nodeID,
		conn:    conn,
		pending: make(map[uint64]chan *agentproto.Result),
		done:    make(chan struct{}),
		state:   AgentState{ConnectedAt: time.Now()},
	}
	h.mu.Lock()
	old := h.sessions[nodeID]
	h.sessions[nodeID] = s
	h.mu.Unlock()
	if old != nil {
		old.close()
	}

	go s.pingLoop()
	s.readLoop()
	s.close()

	h.mu.Lock()
	current := h.sessions[nodeID] == s
	if current {
		delete(h.sessions, nodeID)
	}
	h.mu.Unlock()
	if gone := h.currentHandlers().Gone; current && gone != nil {
		gone(nodeID)
	}
}

// Session reports the state of nodeID's agent while it is connected.
func (h *AgentHub) Session(nodeID int) (AgentState, bool) {
	s := h.session(nodeID)
	if s == nil {
		return AgentState{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state, true
}

// Connected lists the nodes with a live agent session.
func (h *AgentHub) Connected() []int {
	h.mu.Lock()
	defer h.mu.Unlock()
	ids := make([]int, 0, len(h.sessions))
	for id := range h.sessions {
		ids = append(ids, id)
	}
	return ids
}

// Disconnect ends nodeID's session; its agent reconnects only if still allowed.
func (h *AgentHub) Disconnect(nodeID int) {
	if s := h.session(nodeID); s != nil {
		s.close()
	}
}

// Apply hands nodeID's agent a whole config and waits for its verdict.
func (h *AgentHub) Apply(ctx context.Context, nodeID int, apply agentproto.Apply) (*agentproto.Result, error) {
	s := h.session(nodeID)
	if s == nil {
		return nil, ErrAgentNotConnected
	}
	res, err := s.request(ctx, agentproto.Message{Type: agentproto.TypeApply, Apply: &apply})
	if err != nil {
		return nil, err
	}
	if res.OK {
		s.mu.Lock()
		s.state.AppliedHash = apply.Hash
		s.mu.Unlock()
	}
	return res, nil
}

// Restart asks nodeID's agent to restart its Xray core.
func (h *AgentHub) Restart(ctx context.Context, nodeID int) error {
	s := h.session(nodeID)
	if s == nil {
		return ErrAgentNotConnected
	}
	res, err := s.request(ctx, agentproto.Message{Type: agentproto.TypeRestart})
	if err != nil {
		return err
	}
	if !res.OK {
		return errors.New("agent restart failed: " + res.Error)
	}
	return nil
}

// Nudge asks the sync loop to look at nodeID now rather than on its next tick;
// a full queue drops the nudge because that tick comes anyway.
func (h *AgentHub) Nudge(nodeID int) {
	select {
	case h.nudges <- nodeID:
	default:
	}
}

func (h *AgentHub) Nudges() <-chan int { return h.nudges }

type agentSession struct {
	hub    *AgentHub
	nodeID int
	conn   *websocket.Conn

	writeMu sync.Mutex

	mu      sync.Mutex
	state   AgentState
	pending map[uint64]chan *agentproto.Result
	nextID  uint64

	done      chan struct{}
	closeOnce sync.Once
}

func (s *agentSession) close() {
	s.closeOnce.Do(func() {
		close(s.done)
		_ = s.conn.Close()
	})
}

func (s *agentSession) send(msg agentproto.Message) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_ = s.conn.SetWriteDeadline(time.Now().Add(agentWriteTimeout))
	return s.conn.WriteJSON(msg)
}

func (s *agentSession) request(ctx context.Context, msg agentproto.Message) (*agentproto.Result, error) {
	ch := make(chan *agentproto.Result, 1)
	s.mu.Lock()
	s.nextID++
	msg.ID = s.nextID
	s.pending[msg.ID] = ch
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.pending, msg.ID)
		s.mu.Unlock()
	}()

	if err := s.send(msg); err != nil {
		s.close()
		return nil, ErrAgentNotConnected
	}
	select {
	case res := <-ch:
		return res, nil
	case <-s.done:
		return nil, ErrAgentNotConnected
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *agentSession) pingLoop() {
	ticker := time.NewTicker(agentPingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			sent := strconv.FormatInt(time.Now().UnixNano(), 10)
			if err := s.conn.WriteControl(websocket.PingMessage, []byte(sent), time.Now().Add(agentWriteTimeout)); err != nil {
				s.close()
				return
			}
		}
	}
}

func (s *agentSession) readLoop() {
	s.conn.SetReadLimit(agentMaxMessage)
	_ = s.conn.SetReadDeadline(time.Now().Add(agentReadTimeout))
	s.conn.SetPongHandler(func(data string) error {
		_ = s.conn.SetReadDeadline(time.Now().Add(agentReadTimeout))
		if sent, err := strconv.ParseInt(data, 10, 64); err == nil {
			s.mu.Lock()
			s.state.LatencyMs = int(time.Since(time.Unix(0, sent)) / time.Millisecond)
			s.mu.Unlock()
		}
		return nil
	})
	for {
		var msg agentproto.Message
		if err := s.conn.ReadJSON(&msg); err != nil {
			return
		}
		_ = s.conn.SetReadDeadline(time.Now().Add(agentReadTimeout))
		s.handle(&msg)
	}
}

func (s *agentSession) handle(msg *agentproto.Message) {
	switch msg.Type {
	case agentproto.TypeHello:
		if msg.Hello == nil {
			return
		}
		s.mu.Lock()
		s.state.Hello = *msg.Hello
		s.state.AppliedHash = msg.Hello.ConfigHash
		s.mu.Unlock()
		s.hub.Nudge(s.nodeID)
	case agentproto.TypeStatus:
		if msg.Status == nil {
			return
		}
		s.mu.Lock()
		s.state.Status = *msg.Status
		s.state.StatusAt = time.Now()
		s.mu.Unlock()
		if status := s.hub.currentHandlers().Status; status != nil {
			status(s.nodeID, msg.Status)
		}
	case agentproto.TypeTraffic:
		if msg.Traffic == nil {
			return
		}
		traffic := s.hub.currentHandlers().Traffic
		if traffic == nil {
			return
		}
		if err := traffic(s.nodeID, msg.Traffic); err != nil {
			logger.Warningf("agent node %d: traffic report %d not stored, the agent will resend it: %v", s.nodeID, msg.Traffic.Seq, err)
			return
		}
		ack := agentproto.Ack{Instance: msg.Traffic.Instance, Seq: msg.Traffic.Seq}
		if err := s.send(agentproto.Message{Type: agentproto.TypeAck, Ack: &ack}); err != nil {
			s.close()
		}
	case agentproto.TypeResult:
		if msg.Result == nil {
			return
		}
		s.mu.Lock()
		ch := s.pending[msg.ID]
		s.mu.Unlock()
		if ch != nil {
			ch <- msg.Result
		}
	}
}

var (
	agentHubMu sync.RWMutex
	agentHub   *AgentHub
)

func SetAgentHub(h *AgentHub) {
	agentHubMu.Lock()
	defer agentHubMu.Unlock()
	agentHub = h
}

func GetAgentHub() *AgentHub {
	agentHubMu.RLock()
	defer agentHubMu.RUnlock()
	return agentHub
}

// Probe checks the destination from the connected relay host. Older agents are
// explicitly unverified; a panel-side TCP dial is not an equivalent test.
func (h *AgentHub) Probe(ctx context.Context, nodeID int, p agentproto.Probe) error {
	state, ok := h.Session(nodeID)
	if !ok {
		return ErrAgentNotConnected
	}
	if !slices.Contains(state.Hello.Capabilities, agentproto.ProbeCapability) {
		return errors.New("中转 Agent 需要升级才能检查目标连通性")
	}
	s := h.session(nodeID)
	if s == nil {
		return ErrAgentNotConnected
	}
	res, err := s.request(ctx, agentproto.Message{Type: agentproto.TypeProbe, Probe: &p})
	if err != nil {
		return err
	}
	if !res.OK {
		return errors.New(res.Error)
	}
	return nil
}
