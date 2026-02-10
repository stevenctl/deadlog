package deadlog

import (
	"math/rand/v2"
	"sync"
)

// Mutex is a logged wrapper around sync.RWMutex.
// It can be used as a drop-in replacement for both sync.Mutex and sync.RWMutex.
type Mutex struct {
	mu         sync.RWMutex
	name       string
	logFunc    LogFunc
	traceDepth int
}

// New creates a new logged Mutex with the given options.
func New(opts ...Option) Mutex {
	m := Mutex{
		logFunc: DefaultLogger,
	}
	for _, opt := range opts {
		opt(&m)
	}
	return m
}

func (m *Mutex) emit(typ, state string, id int, name string) {
	if m.logFunc == nil {
		return
	}
	trace := ""
	if m.traceDepth > 0 {
		trace = getCallerChain(4, m.traceDepth)
	}
	m.logFunc(newEvent(typ, state, name, id, trace))
}

// Lock acquires the write lock.
// Uses type "WLOCK" which does not track RELEASED (use LockFunc for that).
func (m *Mutex) Lock() {
	id := rand.IntN(9999999)
	m.emit("WLOCK", "START", id, m.name)
	m.mu.Lock()
	m.emit("WLOCK", "ACQUIRED", id, m.name)
}

// Unlock releases the write lock.
func (m *Mutex) Unlock() {
	m.mu.Unlock()
}

// LockFunc acquires the write lock and returns an unlock function
// that logs the RELEASED event with a correlated ID.
// Uses type "LOCK" which tracks the full lifecycle.
// Optional LockOpt arguments override per-call settings (e.g. WithLockName).
func (m *Mutex) LockFunc(opts ...LockOpt) func() {
	lo := lockOpts{name: m.name}
	for _, opt := range opts {
		opt(&lo)
	}
	id := rand.IntN(9999999)
	m.emit("LOCK", "START", id, lo.name)
	m.mu.Lock()
	m.emit("LOCK", "ACQUIRED", id, lo.name)
	return func() {
		m.emit("LOCK", "RELEASED", id, lo.name)
		m.mu.Unlock()
	}
}

// RLock acquires the read lock.
// Uses type "RWLOCK" which does not track RELEASED (use RLockFunc for that).
func (m *Mutex) RLock() {
	id := rand.IntN(9999999)
	m.emit("RWLOCK", "START", id, m.name)
	m.mu.RLock()
	m.emit("RWLOCK", "ACQUIRED", id, m.name)
}

// RUnlock releases the read lock.
func (m *Mutex) RUnlock() {
	m.mu.RUnlock()
}

// RLockFunc acquires the read lock and returns an unlock function
// that logs the RELEASED event with a correlated ID.
// Uses type "RLOCK" which tracks the full lifecycle.
// Optional LockOpt arguments override per-call settings (e.g. WithLockName).
func (m *Mutex) RLockFunc(opts ...LockOpt) func() {
	lo := lockOpts{name: m.name}
	for _, opt := range opts {
		opt(&lo)
	}
	id := rand.IntN(9999999)
	m.emit("RLOCK", "START", id, lo.name)
	m.mu.RLock()
	m.emit("RLOCK", "ACQUIRED", id, lo.name)
	return func() {
		m.emit("RLOCK", "RELEASED", id, lo.name)
		m.mu.RUnlock()
	}
}
