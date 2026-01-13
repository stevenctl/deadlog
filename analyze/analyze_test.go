package analyze

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stevenctl/deadlog"
)

func TestAnalyze_NoIssues(t *testing.T) {
	var buf bytes.Buffer
	m := deadlog.New(deadlog.WithLogger(deadlog.WriterLogger(&buf)))

	// Normal lock/unlock cycle
	unlock := m.LockFunc()
	unlock()

	result, err := Analyze(&buf)
	if err != nil {
		t.Fatalf("Analyze error: %v", err)
	}

	if len(result.Stuck) != 0 {
		t.Errorf("expected no stuck locks, got %d", len(result.Stuck))
	}
	if len(result.Held) != 0 {
		t.Errorf("expected no held locks, got %d", len(result.Held))
	}
}

func TestAnalyze_HeldLock(t *testing.T) {
	var buf bytes.Buffer
	m := deadlog.New(
		deadlog.WithName("held-test"),
		deadlog.WithLogger(deadlog.WriterLogger(&buf)),
	)

	// Acquire with LockFunc but don't call unlock
	_ = m.LockFunc() // ignore the unlock function

	result, err := Analyze(&buf)
	if err != nil {
		t.Fatalf("Analyze error: %v", err)
	}

	if len(result.Held) != 1 {
		t.Fatalf("expected 1 held lock, got %d", len(result.Held))
	}
	if result.Held[0].Name != "held-test" {
		t.Errorf("expected name 'held-test', got %q", result.Held[0].Name)
	}
	if result.Held[0].Type != "LOCK" {
		t.Errorf("expected type 'LOCK', got %q", result.Held[0].Type)
	}

	// Clean up the actual mutex
	m.Unlock()
}

func TestAnalyze_UntrackedLockNotHeld(t *testing.T) {
	var buf bytes.Buffer
	m := deadlog.New(
		deadlog.WithName("untracked-test"),
		deadlog.WithLogger(deadlog.WriterLogger(&buf)),
	)

	// Lock() uses WLOCK which is untracked - should NOT show as held
	m.Lock()

	result, err := Analyze(&buf)
	if err != nil {
		t.Fatalf("Analyze error: %v", err)
	}

	// WLOCK is untracked, so no "held" should be reported
	if len(result.Held) != 0 {
		t.Errorf("expected 0 held locks for untracked WLOCK, got %d", len(result.Held))
	}

	m.Unlock()
}

func TestAnalyze_StuckWaiting(t *testing.T) {
	var buf bytes.Buffer
	m := deadlog.New(
		deadlog.WithName("stuck-test"),
		deadlog.WithLogger(deadlog.WriterLogger(&buf)),
	)

	// Hold a write lock
	m.Lock()

	started := make(chan struct{})
	done := make(chan struct{})

	// Start a goroutine that will block trying to acquire
	go func() {
		close(started)
		m.Lock() // This will block and log START but not ACQUIRED
		m.Unlock()
		close(done)
	}()

	<-started
	// Give the goroutine time to emit START event
	time.Sleep(50 * time.Millisecond)

	// Analyze at this point - one stuck (waiting)
	result, err := Analyze(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("Analyze error: %v", err)
	}

	// Lock() uses WLOCK (untracked), so no "held" reported
	if len(result.Held) != 0 {
		t.Errorf("expected 0 held locks (WLOCK is untracked), got %d", len(result.Held))
	}

	// The waiting goroutine is stuck (START without ACQUIRED)
	if len(result.Stuck) != 1 {
		t.Errorf("expected 1 stuck lock, got %d", len(result.Stuck))
	}
	if len(result.Stuck) > 0 && result.Stuck[0].Type != "WLOCK" {
		t.Errorf("expected stuck type WLOCK, got %s", result.Stuck[0].Type)
	}

	// Clean up
	m.Unlock()
	<-done
}

func TestAnalyze_RLockHeld(t *testing.T) {
	var buf bytes.Buffer
	m := deadlog.New(
		deadlog.WithName("rlock-held"),
		deadlog.WithLogger(deadlog.WriterLogger(&buf)),
	)

	// Acquire read lock with RLockFunc but don't call unlock
	_ = m.RLockFunc() // ignore the unlock function

	result, err := Analyze(&buf)
	if err != nil {
		t.Fatalf("Analyze error: %v", err)
	}

	if len(result.Held) != 1 {
		t.Fatalf("expected 1 held lock, got %d", len(result.Held))
	}
	if result.Held[0].Type != "RLOCK" {
		t.Errorf("expected type 'RLOCK', got %q", result.Held[0].Type)
	}

	m.RUnlock()
}

func TestAnalyze_UntrackedRLockNotHeld(t *testing.T) {
	var buf bytes.Buffer
	m := deadlog.New(
		deadlog.WithName("untracked-rlock"),
		deadlog.WithLogger(deadlog.WriterLogger(&buf)),
	)

	// RLock() uses RWLOCK which is untracked - should NOT show as held
	m.RLock()

	result, err := Analyze(&buf)
	if err != nil {
		t.Fatalf("Analyze error: %v", err)
	}

	// RWLOCK is untracked, so no "held" should be reported
	if len(result.Held) != 0 {
		t.Errorf("expected 0 held locks for untracked RWLOCK, got %d", len(result.Held))
	}

	m.RUnlock()
}

func TestAnalyze_WithTrace(t *testing.T) {
	var buf bytes.Buffer
	m := deadlog.New(
		deadlog.WithName("trace-test"),
		deadlog.WithTrace(3),
		deadlog.WithLogger(deadlog.WriterLogger(&buf)),
	)

	// Use LockFunc to get a tracked lock that shows as held
	_ = m.LockFunc() // ignore unlock function

	result, err := Analyze(&buf)
	if err != nil {
		t.Fatalf("Analyze error: %v", err)
	}

	if len(result.Held) != 1 {
		t.Fatalf("expected 1 held lock, got %d", len(result.Held))
	}
	if result.Held[0].Trace == "" {
		t.Error("expected trace to be present")
	}

	m.Unlock()
}

func TestAnalyze_MultipleIssues(t *testing.T) {
	var buf bytes.Buffer
	logger := deadlog.WriterLogger(&buf)

	m1 := deadlog.New(deadlog.WithName("mutex-1"), deadlog.WithLogger(logger))
	m2 := deadlog.New(deadlog.WithName("mutex-2"), deadlog.WithLogger(logger))

	// m1 is held (use LockFunc for tracked type)
	_ = m1.LockFunc()

	// m2 has a waiter (use Lock for untracked, waiter also uses Lock)
	m2.Lock()
	started := make(chan struct{})
	done := make(chan struct{})
	go func() {
		close(started)
		m2.Lock()
		m2.Unlock()
		close(done)
	}()
	<-started
	time.Sleep(50 * time.Millisecond)

	result, err := Analyze(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("Analyze error: %v", err)
	}

	// Only m1 (LOCK type from LockFunc) shows as held
	// m2 uses Lock() which is WLOCK (untracked)
	if len(result.Held) != 1 {
		t.Errorf("expected 1 held lock (tracked LOCK only), got %d", len(result.Held))
	}
	// One waiter stuck on m2
	if len(result.Stuck) != 1 {
		t.Errorf("expected 1 stuck lock, got %d", len(result.Stuck))
	}

	// Clean up
	m1.Unlock()
	m2.Unlock()
	<-done
}

func TestAnalyze_IgnoresNonJSON(t *testing.T) {
	input := `not json
{"type":"LOCK","state":"START","name":"test","id":123,"ts":1234567890}
also not json
{"type":"LOCK","state":"ACQUIRED","name":"test","id":123,"ts":1234567891}
`
	result, err := Analyze(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Analyze error: %v", err)
	}

	// Lock is held (no RELEASED)
	if len(result.Held) != 1 {
		t.Errorf("expected 1 held lock, got %d", len(result.Held))
	}
}

func TestPrintReport(t *testing.T) {
	result := &Result{
		Stuck: []LockInfo{
			{Type: "LOCK", Name: "stuck-mutex", ID: 123},
		},
		Held: []LockInfo{
			{Type: "RLOCK", Name: "held-mutex", ID: 456, Trace: "foo:10 <- bar:20"},
		},
	}

	var buf bytes.Buffer
	PrintReport(&buf, result)

	output := buf.String()

	if !strings.Contains(output, "LOCK CONTENTION ANALYSIS") {
		t.Error("report should contain header")
	}
	if !strings.Contains(output, "stuck-mutex") {
		t.Error("report should contain stuck mutex name")
	}
	if !strings.Contains(output, "held-mutex") {
		t.Error("report should contain held mutex name")
	}
	if !strings.Contains(output, "foo:10 <- bar:20") {
		t.Error("report should contain trace")
	}
	if !strings.Contains(output, "Stuck waiting: 1") {
		t.Error("report should show stuck count")
	}
	if !strings.Contains(output, "Held:          1") {
		t.Error("report should show held count")
	}
}

func TestAnalyze_ConcurrentWrites(t *testing.T) {
	var buf bytes.Buffer
	var bufMu sync.Mutex

	safeLogger := func(e deadlog.Event) {
		bufMu.Lock()
		defer bufMu.Unlock()
		deadlog.WriterLogger(&buf)(e)
	}

	m := deadlog.New(deadlog.WithName("concurrent"), deadlog.WithLogger(safeLogger))

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unlock := m.LockFunc()
			time.Sleep(time.Millisecond)
			unlock()
		}()
	}
	wg.Wait()

	bufMu.Lock()
	result, err := Analyze(bytes.NewReader(buf.Bytes()))
	bufMu.Unlock()

	if err != nil {
		t.Fatalf("Analyze error: %v", err)
	}

	if len(result.Stuck) != 0 {
		t.Errorf("expected no stuck, got %d", len(result.Stuck))
	}
	if len(result.Held) != 0 {
		t.Errorf("expected no held, got %d", len(result.Held))
	}
}

func TestAnalyze_MixedTrackedUntracked(t *testing.T) {
	var buf bytes.Buffer
	var bufMu sync.Mutex

	safeLogger := func(e deadlog.Event) {
		bufMu.Lock()
		defer bufMu.Unlock()
		deadlog.WriterLogger(&buf)(e)
	}

	// Create mutexes with different names to distinguish them
	tracked1 := deadlog.New(deadlog.WithName("tracked-1"), deadlog.WithLogger(safeLogger))
	tracked2 := deadlog.New(deadlog.WithName("tracked-2"), deadlog.WithLogger(safeLogger))
	untracked1 := deadlog.New(deadlog.WithName("untracked-1"), deadlog.WithLogger(safeLogger))
	untracked2 := deadlog.New(deadlog.WithName("untracked-2"), deadlog.WithLogger(safeLogger))

	// Tracked locks (LockFunc) - don't release, should show as held
	_ = tracked1.LockFunc()
	_ = tracked2.RLockFunc()

	// Untracked locks (Lock/RLock) - don't release, should NOT show as held
	untracked1.Lock()
	untracked2.RLock()

	// Create some stuck waiters on untracked locks
	started := make(chan struct{})
	go func() {
		close(started)
		untracked1.Lock() // will block
		untracked1.Unlock()
	}()
	<-started
	time.Sleep(50 * time.Millisecond)

	bufMu.Lock()
	result, err := Analyze(bytes.NewReader(buf.Bytes()))
	bufMu.Unlock()

	if err != nil {
		t.Fatalf("Analyze error: %v", err)
	}

	// Should have exactly 2 held (tracked-1 LOCK, tracked-2 RLOCK)
	if len(result.Held) != 2 {
		t.Errorf("expected 2 held locks (tracked only), got %d", len(result.Held))
		for _, h := range result.Held {
			t.Logf("  held: %s %s", h.Type, h.Name)
		}
	}

	// Should have 1 stuck (waiter on untracked-1)
	if len(result.Stuck) != 1 {
		t.Errorf("expected 1 stuck lock, got %d", len(result.Stuck))
	}

	// Verify the held locks are the right types
	heldTypes := make(map[string]bool)
	for _, h := range result.Held {
		heldTypes[h.Type] = true
	}
	if !heldTypes["LOCK"] {
		t.Error("expected LOCK in held types")
	}
	if !heldTypes["RLOCK"] {
		t.Error("expected RLOCK in held types")
	}

	// Verify stuck is WLOCK (untracked write lock)
	if len(result.Stuck) > 0 && result.Stuck[0].Type != "WLOCK" {
		t.Errorf("expected stuck type WLOCK, got %s", result.Stuck[0].Type)
	}

	// Clean up
	tracked1.Unlock()
	tracked2.RUnlock()
	untracked1.Unlock()
	untracked2.RUnlock()
}

func TestAnalyze_CountsByType(t *testing.T) {
	// Test with raw JSON to precisely control event types
	input := `{"type":"LOCK","state":"START","name":"a","id":1,"ts":1}
{"type":"LOCK","state":"ACQUIRED","name":"a","id":1,"ts":2}
{"type":"LOCK","state":"START","name":"b","id":2,"ts":3}
{"type":"LOCK","state":"ACQUIRED","name":"b","id":2,"ts":4}
{"type":"LOCK","state":"RELEASED","name":"b","id":2,"ts":5}
{"type":"RLOCK","state":"START","name":"c","id":3,"ts":6}
{"type":"RLOCK","state":"ACQUIRED","name":"c","id":3,"ts":7}
{"type":"WLOCK","state":"START","name":"d","id":4,"ts":8}
{"type":"WLOCK","state":"ACQUIRED","name":"d","id":4,"ts":9}
{"type":"RWLOCK","state":"START","name":"e","id":5,"ts":10}
{"type":"RWLOCK","state":"ACQUIRED","name":"e","id":5,"ts":11}
{"type":"WLOCK","state":"START","name":"f","id":6,"ts":12}
`
	result, err := Analyze(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Analyze error: %v", err)
	}

	// Held should only include tracked types (LOCK, RLOCK) without RELEASED
	// - LOCK a (id=1): acquired, no release -> held
	// - LOCK b (id=2): acquired and released -> not held
	// - RLOCK c (id=3): acquired, no release -> held
	// - WLOCK d (id=4): untracked, ignored for held
	// - RWLOCK e (id=5): untracked, ignored for held
	if len(result.Held) != 2 {
		t.Errorf("expected 2 held (LOCK a, RLOCK c), got %d", len(result.Held))
		for _, h := range result.Held {
			t.Logf("  held: type=%s name=%s id=%d", h.Type, h.Name, h.ID)
		}
	}

	// Stuck should include any type with START but no ACQUIRED
	// - WLOCK f (id=6): started, not acquired -> stuck
	if len(result.Stuck) != 1 {
		t.Errorf("expected 1 stuck (WLOCK f), got %d", len(result.Stuck))
	}
	if len(result.Stuck) > 0 {
		if result.Stuck[0].Type != "WLOCK" || result.Stuck[0].Name != "f" {
			t.Errorf("expected stuck WLOCK f, got %s %s", result.Stuck[0].Type, result.Stuck[0].Name)
		}
	}
}
