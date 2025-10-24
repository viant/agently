package runtime

import (
    "net/http"

    "github.com/viant/agently/internal/auth/authority"
    "github.com/viant/agently/internal/auth/cookiejar"
    "github.com/viant/agently/internal/auth/resolver"
    "github.com/viant/agently/internal/auth/tokens"
    "github.com/viant/agently/internal/obs"
)

// Runtime bundles auth-related dependencies for dependency injection.
// It avoids global singletons and lets services share a single resolver and
// cookie jar manager for the application lifetime.
type Runtime struct {
    CookieJars *cookiejar.Manager
    Store      *tokens.Store
    Broker     resolver.Broker
    MinTTL     resolver.MinTTL
    Tracer     obs.Tracer
    Metrics    obs.Metrics
}

// Option configures Runtime.
type Option func(*Runtime)

// WithMinTTL sets proactive refresh thresholds.
func WithMinTTL(ttl resolver.MinTTL) Option { return func(r *Runtime) { r.MinTTL = ttl } }

// WithTracer sets a tracer for debug events.
func WithTracer(t obs.Tracer) Option { return func(r *Runtime) { r.Tracer = t } }

// WithMetrics sets a metrics recorder.
func WithMetrics(m obs.Metrics) Option { return func(r *Runtime) { r.Metrics = m } }

// New constructs a Runtime. Callers should provide a shared cookie manager,
// token store and a broker implementation.
func New(cj *cookiejar.Manager, st *tokens.Store, br resolver.Broker, opts ...Option) *Runtime {
    rt := &Runtime{CookieJars: cj, Store: st, Broker: br}
    for _, o := range opts { if o != nil { o(rt) } }
    if rt.Tracer == nil { rt.Tracer = obs.NoopTracer{} }
    if rt.Metrics == nil { rt.Metrics = obs.NoopMetrics{} }
    return rt
}

// NewResolver creates a resolver bound to this runtime's store and broker.
func (r *Runtime) NewResolver() (*resolver.Resolver, error) {
    return resolver.New(r.Store, r.Broker, resolver.Options{MinTTL: r.MinTTL, Tracer: r.Tracer, Metrics: r.Metrics})
}

// CookieJarForAuthority returns a per-authority cookie jar and its origin key.
func (r *Runtime) CookieJarForAuthority(a authority.AuthAuthority) (http.CookieJar, string, error) {
    return r.CookieJars.Get(a)
}

// ClearCookieJar removes a cookie jar identified by its origin key.
func (r *Runtime) ClearCookieJar(originKey string) { r.CookieJars.Clear(originKey) }

// OriginKey returns a normalized origin key for the provided authority.
func (r *Runtime) OriginKey(a authority.AuthAuthority) string { return cookiejar.OriginKeyFromAuthority(a) }

