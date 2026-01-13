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

	// Acquire but don't release
	m.Lock()
	// Not calling Unlock()

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

	// Analyze at this point - one held, one stuck
	result, err := Analyze(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("Analyze error: %v", err)
	}

	// The original lock is held (no RELEASED because we used Lock() not LockFunc())
	if len(result.Held) != 1 {
		t.Errorf("expected 1 held lock, got %d", len(result.Held))
	}

	// The waiting goroutine is stuck
	if len(result.Stuck) != 1 {
		t.Errorf("expected 1 stuck lock, got %d", len(result.Stuck))
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

	// Acquire read lock but don't release
	m.RLock()

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

func TestAnalyze_WithTrace(t *testing.T) {
	var buf bytes.Buffer
	m := deadlog.New(
		deadlog.WithName("trace-test"),
		deadlog.WithTrace(3),
		deadlog.WithLogger(deadlog.WriterLogger(&buf)),
	)

	m.Lock()
	// Don't release to show up as held

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

	// m1 is held
	m1.Lock()

	// m2 has a waiter
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

	if len(result.Held) < 2 {
		t.Errorf("expected at least 2 held locks, got %d", len(result.Held))
	}
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
