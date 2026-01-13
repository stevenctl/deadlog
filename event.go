package deadlog

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"time"
)

// Event represents a lock operation for logging.
type Event struct {
	Type  string `json:"type"`            // "LOCK" or "RLOCK"
	State string `json:"state"`           // "START", "ACQUIRED", or "RELEASED"
	Name  string `json:"name"`            // mutex name
	ID    int    `json:"id"`              // correlation ID
	Trace string `json:"trace,omitempty"` // optional stack trace
	Ts    int64  `json:"ts"`              // unix nanoseconds
}

// LogFunc is a function that handles lock events.
type LogFunc func(Event)

// DefaultLogger writes JSON events to stdout.
func DefaultLogger(e Event) {
	_ = json.NewEncoder(os.Stdout).Encode(e)
}

// WriterLogger returns a LogFunc that writes JSON events to the given writer.
func WriterLogger(w io.Writer) LogFunc {
	enc := json.NewEncoder(w)
	return func(e Event) {
		_ = enc.Encode(e)
	}
}

func newEvent(typ, state, name string, id int, trace string) Event {
	return Event{
		Type:  typ,
		State: state,
		Name:  name,
		ID:    id,
		Trace: trace,
		Ts:    time.Now().UnixNano(),
	}
}

func getCallerChain(skip, depth int) string {
	if depth <= 0 {
		return ""
	}
	pcs := make([]uintptr, depth)
	n := runtime.Callers(skip, pcs)
	if n == 0 {
		return ""
	}
	frames := runtime.CallersFrames(pcs[:n])

	var parts []string
	for {
		frame, more := frames.Next()
		name := frame.Function
		if idx := strings.LastIndex(name, "."); idx != -1 {
			name = name[idx+1:]
		}
		parts = append(parts, fmt.Sprintf("%s:%d", name, frame.Line))
		if !more || len(parts) >= depth {
			break
		}
	}
	return strings.Join(parts, " <- ")
}
