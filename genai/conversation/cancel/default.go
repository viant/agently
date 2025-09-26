package cancel

// defaultReg is a process-wide singleton used when no explicit registry is injected.
var defaultReg Registry = NewMemory()

// Default returns the process-wide cancel registry.
func Default() Registry { return defaultReg }

// SetDefault overrides the process-wide registry. Intended for tests or custom wiring.
func SetDefault(r Registry) {
	if r != nil {
		defaultReg = r
	}
}
