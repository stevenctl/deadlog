package deadlog

// Option configures a Mutex.
type Option func(*Mutex)

// WithName sets an identifier for this mutex instance.
func WithName(name string) Option {
	return func(m *Mutex) {
		m.name = name
	}
}

// WithLogger sets a custom logging function.
// Default is DefaultLogger which writes JSON to stdout.
func WithLogger(fn LogFunc) Option {
	return func(m *Mutex) {
		m.logFunc = fn
	}
}

// WithTrace enables stack trace logging with the specified depth.
// A depth of 0 disables stack traces (default).
func WithTrace(depth int) Option {
	return func(m *Mutex) {
		m.traceDepth = depth
	}
}
