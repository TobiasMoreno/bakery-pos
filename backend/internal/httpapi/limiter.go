package httpapi

import (
	"sync"
	"time"
)

type attemptWindow struct {
	Count     int
	StartedAt time.Time
}

type LoginLimiter struct {
	mu       sync.Mutex
	attempts map[string]attemptWindow
	max      int
	window   time.Duration
}

func NewLoginLimiter(max int, window time.Duration) *LoginLimiter {
	return &LoginLimiter{attempts: make(map[string]attemptWindow), max: max, window: window}
}

func (l *LoginLimiter) Blocked(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry, ok := l.attempts[key]
	if !ok {
		return false
	}
	if now.Sub(entry.StartedAt) >= l.window {
		delete(l.attempts, key)
		return false
	}
	return entry.Count >= l.max
}

func (l *LoginLimiter) RecordFailure(key string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry, ok := l.attempts[key]
	if !ok || now.Sub(entry.StartedAt) >= l.window {
		l.attempts[key] = attemptWindow{Count: 1, StartedAt: now}
		return
	}
	entry.Count++
	l.attempts[key] = entry
}

func (l *LoginLimiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, key)
}
