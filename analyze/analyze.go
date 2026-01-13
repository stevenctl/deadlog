// Package analyze provides tools for analyzing deadlog output to detect deadlocks.
package analyze

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/stevenctl/deadlog"
)

// LockInfo contains information about a lock event.
type LockInfo struct {
	Type  string // "LOCK" or "RLOCK"
	Name  string // mutex name
	ID    int    // correlation ID
	Trace string // stack trace if available
}

// Result contains the analysis results.
type Result struct {
	// Stuck contains locks that started but never acquired (waiting for lock).
	Stuck []LockInfo
	// Held contains locks that acquired but never released (holding lock).
	Held []LockInfo
}

// Analyze reads deadlog JSON events from r and returns analysis results.
func Analyze(r io.Reader) (*Result, error) {
	starts := make(map[string]*LockInfo)
	acquires := make(map[string]*LockInfo)
	releases := make(map[string]struct{})

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var e deadlog.Event
		if err := json.Unmarshal(line, &e); err != nil {
			// Skip non-JSON lines
			continue
		}

		key := fmt.Sprintf("%s|%s|%d", e.Type, e.Name, e.ID)

		switch e.State {
		case "START":
			starts[key] = &LockInfo{
				Type:  e.Type,
				Name:  e.Name,
				ID:    e.ID,
				Trace: e.Trace,
			}
		case "ACQUIRED":
			acquires[key] = &LockInfo{
				Type:  e.Type,
				Name:  e.Name,
				ID:    e.ID,
				Trace: e.Trace,
			}
		case "RELEASED":
			releases[key] = struct{}{}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	result := &Result{}

	// Find stuck: started but never acquired
	for key, info := range starts {
		if _, acquired := acquires[key]; !acquired {
			result.Stuck = append(result.Stuck, *info)
		}
	}

	// Find held: acquired but never released
	for key, info := range acquires {
		if _, released := releases[key]; !released {
			result.Held = append(result.Held, *info)
		}
	}

	// Sort for deterministic output
	sort.Slice(result.Stuck, func(i, j int) bool {
		return result.Stuck[i].ID < result.Stuck[j].ID
	})
	sort.Slice(result.Held, func(i, j int) bool {
		return result.Held[i].ID < result.Held[j].ID
	})

	return result, nil
}

// AnalyzeFile reads deadlog JSON events from a file and returns analysis results.
func AnalyzeFile(path string) (*Result, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Analyze(f)
}

// PrintReport prints a human-readable report of the analysis results.
func PrintReport(w io.Writer, r *Result) {
	fmt.Fprintln(w, "===============================================")
	fmt.Fprintln(w, "  LOCK CONTENTION ANALYSIS")
	fmt.Fprintln(w, "===============================================")
	fmt.Fprintln(w)

	fmt.Fprintln(w, "=== STUCK: Started but never acquired (waiting for lock) ===")
	if len(r.Stuck) == 0 {
		fmt.Fprintln(w, "  (none)")
	} else {
		for _, info := range r.Stuck {
			name := info.Name
			if name == "" {
				name = "(unnamed)"
			}
			fmt.Fprintf(w, "  %-5s | %-20s | ID: %d\n", info.Type, name, info.ID)
			if info.Trace != "" {
				fmt.Fprintf(w, "         Trace: %s\n", info.Trace)
			}
		}
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "=== HELD: Acquired but never released (holding lock) ===")
	if len(r.Held) == 0 {
		fmt.Fprintln(w, "  (none)")
	} else {
		for _, info := range r.Held {
			name := info.Name
			if name == "" {
				name = "(unnamed)"
			}
			fmt.Fprintf(w, "  %-5s | %-20s | ID: %d\n", info.Type, name, info.ID)
			if info.Trace != "" {
				fmt.Fprintf(w, "         Trace: %s\n", info.Trace)
			}
		}
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "=== SUMMARY ===")
	fmt.Fprintf(w, "  Stuck waiting: %d\n", len(r.Stuck))
	fmt.Fprintf(w, "  Held:          %d\n", len(r.Held))
	fmt.Fprintln(w)
}
