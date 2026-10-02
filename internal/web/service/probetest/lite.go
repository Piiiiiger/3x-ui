// Package probetest plays a Lite monitor in tests: a real HTTP server on
// loopback that answers the JSON-RPC batch the panel's probe sends.
package probetest

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
)

// Lite is a fake Lite. It dispatches on the method name and answers by id, so
// a wrong method name fails the way it would against the real program.
type Lite struct {
	URL string

	srv      *httptest.Server
	mu       sync.Mutex
	results  map[string]json.RawMessage
	calls    map[string]int
	requests int
	backward bool
	override http.HandlerFunc
}

// NewLite starts a Lite with no servers and no status reports.
func NewLite(t testing.TB) *Lite {
	t.Helper()
	l := &Lite{}
	l.Answer(`{}`, `{}`)
	l.srv = httptest.NewServer(http.HandlerFunc(l.handle))
	t.Cleanup(l.srv.Close)
	l.URL = l.srv.URL
	return l
}

// Stop shuts the server down, so its address refuses connections.
func (l *Lite) Stop() {
	l.srv.Close()
}

// Answer sets the results of common:getNodes and common:getNodesLatestStatus.
func (l *Lite) Answer(nodes, statuses string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.results = map[string]json.RawMessage{
		"common:getNodes":             json.RawMessage(nodes),
		"common:getNodesLatestStatus": json.RawMessage(statuses),
	}
}

// AnswerMetrics sets the result of public:queryMetrics. A Lite that was never
// given one answers that method with JSON-RPC error -32601, as an old Lite does.
func (l *Lite) AnswerMetrics(result string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.results["public:queryMetrics"] = json.RawMessage(result)
}

// Calls is how many times method was called, answered or not.
func (l *Lite) Calls(method string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.calls[method]
}

// Forget removes a method, so calls to it get JSON-RPC error -32601.
func (l *Lite) Forget(method string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.results, method)
}

// AnswerBackward makes batch replies come back in reverse request order.
func (l *Lite) AnswerBackward() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.backward = true
}

// Override replaces the JSON-RPC answer with handler; nil restores it.
func (l *Lite) Override(handler http.HandlerFunc) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.override = handler
}

// Requests is how many HTTP requests reached this Lite.
func (l *Lite) Requests() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.requests
}

type rpcCall struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcReply struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

func (l *Lite) handle(w http.ResponseWriter, r *http.Request) {
	l.mu.Lock()
	l.requests++
	override := l.override
	l.mu.Unlock()
	// Reading the body first lets net/http notice a client that hangs up, which
	// is what ends an override that waits on the request context.
	body, _ := io.ReadAll(r.Body)
	if override != nil {
		override(w, r)
		return
	}
	if r.Method != http.MethodPost || r.URL.Path != "/api/rpc2" {
		http.NotFound(w, r)
		return
	}
	if !json.Valid(body) {
		writeJSON(w, http.StatusBadRequest, rpcReply{JSONRPC: "2.0", Error: &rpcError{Code: -32700, Message: "invalid json"}})
		return
	}
	// Lite answers a single call with a single object and only a batch with an array.
	if !bytes.HasPrefix(bytes.TrimSpace(body), []byte("[")) {
		var call rpcCall
		_ = json.Unmarshal(body, &call)
		writeJSON(w, http.StatusOK, l.answer(call))
		return
	}
	var calls []rpcCall
	_ = json.Unmarshal(body, &calls)
	replies := make([]rpcReply, 0, len(calls))
	for _, call := range calls {
		replies = append(replies, l.answer(call))
	}
	l.mu.Lock()
	backward := l.backward
	l.mu.Unlock()
	if backward {
		slices.Reverse(replies)
	}
	writeJSON(w, http.StatusOK, replies)
}

func (l *Lite) answer(call rpcCall) rpcReply {
	l.mu.Lock()
	if l.calls == nil {
		l.calls = map[string]int{}
	}
	l.calls[call.Method]++
	result, known := l.results[call.Method]
	l.mu.Unlock()
	if !known {
		return rpcReply{JSONRPC: "2.0", ID: call.ID, Error: &rpcError{Code: -32601, Message: "method not found"}}
	}
	return rpcReply{JSONRPC: "2.0", ID: call.ID, Result: result}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
