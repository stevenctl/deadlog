package deadlog

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

func collectEvents(buf *bytes.Buffer) []Event {
	var events []Event
	dec := json.NewDecoder(buf)
	for dec.More() {
		var e Event
		if err := dec.Decode(&e); err != nil {
			break
		}
		events = append(events, e)
	}
	return events
}

func TestMutex_BasicLockUnlock(t *testing.T) {
	var buf bytes.Buffer
	m := New(WithLogger(WriterLogger(&buf)))

	m.Lock()
	m.Unlock()

	events := collectEvents(&buf)
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	// Lock() uses WLOCK (untracked) type
	if events[0].State != "START" || events[0].Type != "WLOCK" {
		t.Errorf("first event should be WLOCK START, got %s %s", events[0].Type, events[0].State)
	}
	if events[1].State != "ACQUIRED" || events[1].Type != "WLOCK" {
		t.Errorf("second event should be WLOCK ACQUIRED, got %s %s", events[1].Type, events[1].State)
	}
	if events[0].ID != events[1].ID {
		t.Errorf("correlation IDs should match: %d vs %d", events[0].ID, events[1].ID)
	}
}

func TestMutex_LockFunc(t *testing.T) {
	var buf bytes.Buffer
	m := New(WithLogger(WriterLogger(&buf)))

	unlock := m.LockFunc()
	unlock()

	events := collectEvents(&buf)
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	if events[2].State != "RELEASED" {
		t.Errorf("third event should be RELEASED, got %s", events[2].State)
	}
	// All three should have same correlation ID
	if events[0].ID != events[1].ID || events[1].ID != events[2].ID {
		t.Errorf("all events should have same ID: %d, %d, %d", events[0].ID, events[1].ID, events[2].ID)
	}
}

func TestMutex_RLock(t *testing.T) {
	var buf bytes.Buffer
	m := New(WithLogger(WriterLogger(&buf)))

	m.RLock()
	m.RUnlock()

	events := collectEvents(&buf)
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	// RLock() uses RWLOCK (untracked) type
	if events[0].Type != "RWLOCK" || events[0].State != "START" {
		t.Errorf("first event should be RWLOCK START, got %s %s", events[0].Type, events[0].State)
	}
	if events[1].Type != "RWLOCK" || events[1].State != "ACQUIRED" {
		t.Errorf("second event should be RWLOCK ACQUIRED, got %s %s", events[1].Type, events[1].State)
	}
}

func TestMutex_RLockFunc(t *testing.T) {
	var buf bytes.Buffer
	m := New(WithLogger(WriterLogger(&buf)))

	unlock := m.RLockFunc()
	unlock()

	events := collectEvents(&buf)
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	if events[2].Type != "RLOCK" || events[2].State != "RELEASED" {
		t.Errorf("third event should be RLOCK RELEASED")
	}
}

func TestMutex_WithName(t *testing.T) {
	var buf bytes.Buffer
	m := New(WithName("test-mutex"), WithLogger(WriterLogger(&buf)))

	m.Lock()
	m.Unlock()

	events := collectEvents(&buf)
	for _, e := range events {
		if e.Name != "test-mutex" {
			t.Errorf("expected name 'test-mutex', got %q", e.Name)
		}
	}
}

func TestMutex_WithTrace(t *testing.T) {
	var buf bytes.Buffer
	m := New(WithTrace(3), WithLogger(WriterLogger(&buf)))

	m.Lock()
	m.Unlock()

	events := collectEvents(&buf)
	for _, e := range events {
		if e.Trace == "" {
			t.Error("expected trace to be non-empty")
		}
		// Should contain function names and line numbers
		if !strings.Contains(e.Trace, ":") {
			t.Errorf("trace should contain line numbers: %s", e.Trace)
		}
	}
}

func TestMutex_LockFunc_WithLockName(t *testing.T) {
	var buf bytes.Buffer
	m := New(WithName("base"), WithLogger(WriterLogger(&buf)))

	unlock := m.LockFunc(WithLockName("custom-write"))
	unlock()

	events := collectEvents(&buf)
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	for _, e := range events {
		if e.Name != "custom-write" {
			t.Errorf("expected name 'custom-write', got %q", e.Name)
		}
	}
}

func TestMutex_RLockFunc_WithLockName(t *testing.T) {
	var buf bytes.Buffer
	m := New(WithName("base"), WithLogger(WriterLogger(&buf)))

	unlock := m.RLockFunc(WithLockName("custom-read"))
	unlock()

	events := collectEvents(&buf)
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	for _, e := range events {
		if e.Name != "custom-read" {
			t.Errorf("expected name 'custom-read', got %q", e.Name)
		}
	}
}

func TestMutex_LockFunc_NoOpts_UsesMutexName(t *testing.T) {
	var buf bytes.Buffer
	m := New(WithName("mutex-level"), WithLogger(WriterLogger(&buf)))

	unlock := m.LockFunc()
	unlock()

	events := collectEvents(&buf)
	for _, e := range events {
		if e.Name != "mutex-level" {
			t.Errorf("expected name 'mutex-level', got %q", e.Name)
		}
	}
}

func TestMutex_ConcurrentReaders(t *testing.T) {
	m := New(WithLogger(nil)) // disable logging for this test

	var wg sync.WaitGroup
	readers := 10
	readCount := 0
	var countMu sync.Mutex

	// Start multiple readers
	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.RLock()
			countMu.Lock()
			readCount++
			countMu.Unlock()
			time.Sleep(10 * time.Millisecond)
			m.RUnlock()
		}()
	}

	wg.Wait()
	if readCount != readers {
		t.Errorf("expected %d readers, got %d", readers, readCount)
	}
}

func TestMutex_WriterExcludesReaders(t *testing.T) {
	m := New(WithLogger(nil))

	m.Lock() // acquire write lock

	readerStarted := make(chan struct{})
	readerDone := make(chan struct{})

	go func() {
		close(readerStarted)
		m.RLock() // should block
		m.RUnlock()
		close(readerDone)
	}()

	<-readerStarted
	// Give reader time to attempt lock
	time.Sleep(50 * time.Millisecond)

	select {
	case <-readerDone:
		t.Error("reader should be blocked while writer holds lock")
	default:
		// expected
	}

	m.Unlock()

	select {
	case <-readerDone:
		// expected
	case <-time.After(time.Second):
		t.Error("reader should complete after writer releases")
	}
}

func TestMutex_MutualExclusion(t *testing.T) {
	m := New(WithLogger(nil))

	var wg sync.WaitGroup
	counter := 0
	iterations := 100

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				m.Lock()
				counter++
				m.Unlock()
			}
		}()
	}

	wg.Wait()
	if counter != 10*iterations {
		t.Errorf("expected %d, got %d (race condition)", 10*iterations, counter)
	}
}

func TestMutex_JSONFormat(t *testing.T) {
	var buf bytes.Buffer
	m := New(WithName("json-test"), WithLogger(WriterLogger(&buf)))

	m.Lock()
	m.Unlock()

	// Verify we can parse each line as JSON
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	for i, line := range lines {
		var e Event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Errorf("line %d is not valid JSON: %v", i, err)
		}
		if e.Ts == 0 {
			t.Errorf("timestamp should be set")
		}
	}
}

func TestMutex_Timestamp(t *testing.T) {
	var buf bytes.Buffer
	m := New(WithLogger(WriterLogger(&buf)))

	before := time.Now().UnixNano()
	m.Lock()
	m.Unlock()
	after := time.Now().UnixNano()

	events := collectEvents(&buf)
	for _, e := range events {
		if e.Ts < before || e.Ts > after {
			t.Errorf("timestamp %d out of range [%d, %d]", e.Ts, before, after)
		}
	}
}
