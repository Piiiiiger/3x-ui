// Package loginlimit slows password guessing: too many failures from one IP for
// one username block that pair for a cooldown.
package loginlimit

import (
	"strings"
	"sync"
	"time"
)

const (
	MaxFailures = 5
	Window      = 5 * time.Minute
	Cooldown    = 15 * time.Minute
	// Hard ceiling on tracked (ip, username) records. The key includes the
	// caller-supplied username, so an unauthenticated attacker rotating
	// usernames would otherwise grow the map without bound.
	maxRecords = 10000
)

// Limiter tracks failed logins per (IP, username) pair.
type Limiter struct {
	mu          sync.Mutex
	now         func() time.Time
	maxFailures int
	window      time.Duration
	cooldown    time.Duration
	attempts    map[string]*attemptRecord
}

type attemptRecord struct {
	failures     []time.Time
	blockedUntil time.Time
}

// New blocks a pair for cooldown once it fails maxFailures times within window.
func New(maxFailures int, window, cooldown time.Duration) *Limiter {
	return &Limiter{
		now:         time.Now,
		maxFailures: maxFailures,
		window:      window,
		cooldown:    cooldown,
		attempts:    make(map[string]*attemptRecord),
	}
}

// Allow reports whether the pair may try now and, if not, until when it is blocked.
func (l *Limiter) Allow(ip, username string) (time.Time, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	key := attemptKey(ip, username)
	record := l.attempts[key]
	if record == nil {
		return time.Time{}, true
	}
	now := l.now()
	if now.Before(record.blockedUntil) {
		return record.blockedUntil, false
	}
	record.blockedUntil = time.Time{}
	record.failures = pruneFailures(record.failures, now.Add(-l.window))
	if len(record.failures) == 0 {
		delete(l.attempts, key)
	}
	return time.Time{}, true
}

// RegisterFailure counts a failed login and reports whether it started a block.
func (l *Limiter) RegisterFailure(ip, username string) (time.Time, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	key := attemptKey(ip, username)
	record := l.attempts[key]
	if record == nil {
		l.evictForRoom(now)
		record = &attemptRecord{}
		l.attempts[key] = record
	}
	record.failures = pruneFailures(record.failures, now.Add(-l.window))
	record.failures = append(record.failures, now)
	if len(record.failures) >= l.maxFailures {
		record.failures = nil
		record.blockedUntil = now.Add(l.cooldown)
		return record.blockedUntil, true
	}
	return time.Time{}, false
}

// RegisterSuccess forgets the pair's earlier failures.
func (l *Limiter) RegisterSuccess(ip, username string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, attemptKey(ip, username))
}

// evictForRoom keeps the attempts map bounded before inserting a new record.
// It first reclaims records that are no longer blocked and whose failures have
// aged out of the window; if the map is still at the ceiling (a genuine
// broad flood), it drops one arbitrary record so memory can never grow past the
// cap. Callers hold l.mu.
func (l *Limiter) evictForRoom(now time.Time) {
	if len(l.attempts) < maxRecords {
		return
	}
	cutoff := now.Add(-l.window)
	for key, record := range l.attempts {
		if now.Before(record.blockedUntil) {
			continue
		}
		record.failures = pruneFailures(record.failures, cutoff)
		if len(record.failures) == 0 {
			delete(l.attempts, key)
		}
	}
	if len(l.attempts) < maxRecords {
		return
	}
	for key, record := range l.attempts {
		if now.Before(record.blockedUntil) {
			continue
		}
		delete(l.attempts, key)
		return
	}
	for key := range l.attempts {
		delete(l.attempts, key)
		return
	}
}

func attemptKey(ip, username string) string {
	return strings.TrimSpace(ip) + "\x00" + strings.ToLower(strings.TrimSpace(username))
}

func pruneFailures(failures []time.Time, cutoff time.Time) []time.Time {
	keepFrom := 0
	for keepFrom < len(failures) && failures[keepFrom].Before(cutoff) {
		keepFrom++
	}
	return failures[keepFrom:]
}
