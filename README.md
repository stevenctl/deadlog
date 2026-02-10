# deadlog

![Build Badge](https://github.com/stevenctl/deadlog/actions/workflows/go.yml/badge.svg)
![Go Version](https://img.shields.io/badge/go-1.25-blue)
![License](https://img.shields.io/github/license/stevenctl/deadlog)
![Go Report Card](https://goreportcard.com/badge/github.com/stevenctl/deadlog)

A Go library for debugging mutex deadlocks with logged wrappers and analysis tools.

## Installation

```bash
go get github.com/stevenctl/deadlog
```

## Usage

Replace `sync.Mutex` or `sync.RWMutex` with `deadlog.Mutex`:

```go
import "github.com/stevenctl/deadlog"

// Before
var mu sync.RWMutex

// After
var mu = deadlog.New(deadlog.WithName("my-service"))
```

The API is compatible with both `sync.Mutex` and `sync.RWMutex`:

```go
// Write lock (sync.Mutex compatible)
mu.Lock()
defer mu.Unlock()

// Read lock (sync.RWMutex compatible)
mu.RLock()
defer mu.RUnlock()
```

### Tracking unreleased locks

Use `LockFunc()` or `RLockFunc()` to get correlated RELEASED events:

```go
unlock := mu.LockFunc()
defer unlock()
```

This logs START, ACQUIRED, and RELEASED events with the same correlation ID, making it easy to identify which lock was never released.

### Named callsites

Use `WithLockName()` to label individual lock operations on the same mutex:

```go
mu := deadlog.New(deadlog.WithName("player-state"), deadlog.WithTrace(1))

// Each callsite gets its own name in the logs
unlock := mu.LockFunc(deadlog.WithLockName("update-health"))
defer unlock()
```

Combined with `WithTrace(1)`, the JSON events pinpoint exactly what's happening:

```json
{"type":"LOCK","state":"START","name":"update-health","id":4480578,"trace":"updateHealth:25","ts":1770746273707970140}
{"type":"LOCK","state":"ACQUIRED","name":"update-health","id":4480578,"trace":"updateHealth:25","ts":1770746273707993939}
{"type":"LOCK","state":"START","name":"add-item","id":9375956,"trace":"addItem:29","ts":1770746273707996887}
{"type":"LOCK","state":"ACQUIRED","name":"add-item","id":9375956,"trace":"addItem:29","ts":1770746273707998734}
{"type":"LOCK","state":"START","name":"apply-damage","id":6439038,"trace":"applyDamage:33","ts":1770746273708002604}
```

The analyzer turns this into a clear report — `apply-damage` is stuck waiting, while `update-health` and `add-item` are holding their locks:

```
===============================================
  LOCK CONTENTION ANALYSIS
===============================================

=== STUCK: Started but never acquired (waiting for lock) ===
  LOCK  | apply-damage         | ID: 6439038
         Trace: applyDamage:33

=== HELD: Acquired but never released (holding lock) ===
  LOCK  | update-health        | ID: 4480578
         Trace: updateHealth:25
  LOCK  | add-item             | ID: 9375956
         Trace: addItem:29

=== SUMMARY ===
  Stuck waiting: 1
  Held:          2
```

### Stack traces

Enable stack traces to see where locks are being acquired:

```go
mu := deadlog.New(
    deadlog.WithName("my-mutex"),
    deadlog.WithTrace(5), // 5 frames deep
)
```

### Custom logging

By default, events are written as JSON to stdout. Use a custom logger:

```go
mu := deadlog.New(
    deadlog.WithLogger(func(e deadlog.Event) {
        log.Printf("[DEADLOG] %s %s %s id=%d", e.Type, e.State, e.Name, e.ID)
    }),
)
```

Or write to a specific writer:

```go
f, _ := os.Create("locks.jsonl")
mu := deadlog.New(deadlog.WithLogger(deadlog.WriterLogger(f)))
```

## Analysis

### CLI

Install the CLI:

```bash
go install github.com/stevenctl/deadlog/cmd/deadlog@latest
```

Analyze a log file:

```bash
deadlog analyze app.log
```

Or pipe from your application:

```bash
go run ./myapp 2>&1 | deadlog analyze -
```

See [Named callsites](#named-callsites) above for example output.

### Library

Use the analysis library programmatically:

```go
import "github.com/stevenctl/deadlog/analyze"

result, err := analyze.AnalyzeFile("app.log")
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Stuck: %d, Held: %d\n", len(result.Stuck), len(result.Held))

// Print formatted report
analyze.PrintReport(os.Stdout, result)
```

## Log Format

Events are logged as JSON:

```json
{"type":"LOCK","state":"START","name":"my-mutex","id":1234567,"ts":1704067200000000000}
{"type":"LOCK","state":"ACQUIRED","name":"my-mutex","id":1234567,"ts":1704067200000001000}
{"type":"LOCK","state":"RELEASED","name":"my-mutex","id":1234567,"ts":1704067200000002000}
```

Fields:
- `type`: lock type (see below)
- `state`: `START`, `ACQUIRED`, or `RELEASED`
- `name`: mutex name from `WithName()`
- `id`: correlation ID (random, same for START/ACQUIRED/RELEASED of one lock operation)
- `ts`: unix nanoseconds
- `trace`: stack trace (if enabled with `WithTrace()`)

### Lock Types

| Method | Type | Tracked | Description |
|--------|------|---------|-------------|
| `LockFunc()` | `LOCK` | Yes | Write lock with RELEASED tracking |
| `RLockFunc()` | `RLOCK` | Yes | Read lock with RELEASED tracking |
| `Lock()` | `WLOCK` | No | Write lock, no RELEASED event |
| `RLock()` | `RWLOCK` | No | Read lock, no RELEASED event |

**Tracked** types (`LOCK`, `RLOCK`) emit RELEASED events via the unlock function, so the analyzer can detect held locks. **Untracked** types (`WLOCK`, `RWLOCK`) are drop-in compatible with `sync.Mutex`/`sync.RWMutex` but won't be reported as "held" since there's no RELEASED event to correlate.

Use untracked methods (`Lock()`/`RLock()`) initially to detect contention, then switch to tracked methods (`LockFunc()`/`RLockFunc()`) where you need to identify which locks are being held.

## How It Works

1. **START**: Logged before attempting to acquire the lock
2. **ACQUIRED**: Logged after the lock is acquired
3. **RELEASED**: Logged when the unlock function is called (only with `LockFunc()`/`RLockFunc()`)

The analyzer detects:
- **Stuck**: START without ACQUIRED (goroutine waiting for a lock) - all types
- **Held**: ACQUIRED without RELEASED (lock not released) - tracked types only (`LOCK`, `RLOCK`)

## License

MIT
