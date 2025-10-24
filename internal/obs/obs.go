package obs

// Tracer provides structured debug event recording without secrets.
type Tracer interface {
    Debug(event string, fields map[string]any)
}

// Metrics provides simple counter increments with string labels.
type Metrics interface {
    Inc(name string, labels map[string]string, delta int64)
}

// NoopTracer is a Tracer that does nothing.
type NoopTracer struct{}

func (NoopTracer) Debug(event string, fields map[string]any) {}

// NoopMetrics is a Metrics that does nothing.
type NoopMetrics struct{}

func (NoopMetrics) Inc(name string, labels map[string]string, delta int64) {}

