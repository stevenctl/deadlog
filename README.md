# deadlog

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

Example output:

```
===============================================
  LOCK CONTENTION ANALYSIS
===============================================

=== STUCK: Started but never acquired (waiting for lock) ===
  LOCK  | worker-pool          | ID: 7339384
  RLOCK | cache                | ID: 6593621

=== HELD: Acquired but never released (holding lock) ===
  RLOCK | database             | ID: 5377378

=== SUMMARY ===
  Stuck waiting: 2
  Held:          1
```

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
- `type`: `LOCK` (write) or `RLOCK` (read)
- `state`: `START`, `ACQUIRED`, or `RELEASED`
- `name`: mutex name from `WithName()`
- `id`: correlation ID (random, same for START/ACQUIRED/RELEASED of one lock operation)
- `ts`: unix nanoseconds
- `trace`: stack trace (if enabled with `WithTrace()`)

## How It Works

1. **START**: Logged before attempting to acquire the lock
2. **ACQUIRED**: Logged after the lock is acquired
3. **RELEASED**: Logged when the unlock function is called (only with `LockFunc()`/`RLockFunc()`)

The analyzer detects:
- **Stuck**: START without ACQUIRED (goroutine waiting for a lock)
- **Held**: ACQUIRED without RELEASED (lock not released, possible deadlock cause)

## License

MIT
