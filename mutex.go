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
func New(opts ...Option) *Mutex {
	m := &Mutex{
		logFunc: DefaultLogger,
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

func (m *Mutex) emit(typ, state string, id int) {
	if m.logFunc == nil {
		return
	}
	trace := ""
	if m.traceDepth > 0 {
		trace = getCallerChain(4, m.traceDepth)
	}
	m.logFunc(newEvent(typ, state, m.name, id, trace))
}

// Lock acquires the write lock.
// Uses type "WLOCK" which does not track RELEASED (use LockFunc for that).
func (m *Mutex) Lock() {
	id := rand.IntN(9999999)
	m.emit("WLOCK", "START", id)
	m.mu.Lock()
	m.emit("WLOCK", "ACQUIRED", id)
}

// Unlock releases the write lock.
func (m *Mutex) Unlock() {
	m.mu.Unlock()
}

// LockFunc acquires the write lock and returns an unlock function
// that logs the RELEASED event with a correlated ID.
// Uses type "LOCK" which tracks the full lifecycle.
func (m *Mutex) LockFunc() func() {
	id := rand.IntN(9999999)
	m.emit("LOCK", "START", id)
	m.mu.Lock()
	m.emit("LOCK", "ACQUIRED", id)
	return func() {
		m.emit("LOCK", "RELEASED", id)
		m.mu.Unlock()
	}
}

// RLock acquires the read lock.
// Uses type "RWLOCK" which does not track RELEASED (use RLockFunc for that).
func (m *Mutex) RLock() {
	id := rand.IntN(9999999)
	m.emit("RWLOCK", "START", id)
	m.mu.RLock()
	m.emit("RWLOCK", "ACQUIRED", id)
}

// RUnlock releases the read lock.
func (m *Mutex) RUnlock() {
	m.mu.RUnlock()
}

// RLockFunc acquires the read lock and returns an unlock function
// that logs the RELEASED event with a correlated ID.
// Uses type "RLOCK" which tracks the full lifecycle.
func (m *Mutex) RLockFunc() func() {
	id := rand.IntN(9999999)
	m.emit("RLOCK", "START", id)
	m.mu.RLock()
	m.emit("RLOCK", "ACQUIRED", id)
	return func() {
		m.emit("RLOCK", "RELEASED", id)
		m.mu.RUnlock()
	}
}
