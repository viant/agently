package resolver

import (
    "context"
    "errors"
    "fmt"
    "sync"
    "time"

    "golang.org/x/sync/singleflight"

    "github.com/viant/agently/internal/auth/tokens"
    "github.com/viant/agently/internal/obs"
    scyauth "github.com/viant/scy/auth"
)

// Broker provides access/ID tokens via exchange or refresh using BFF-backed flows.
type Broker interface {
    // Exchange obtains a downscoped access (and optional id) token for the given audience with at least minTTL.
    Exchange(ctx context.Context, k tokens.Key, minTTL time.Duration) (access scyauth.Token, id string, err error)
    // Refresh refreshes tokens to satisfy minTTL without changing audience.
    Refresh(ctx context.Context, k tokens.Key, minTTL time.Duration) (access scyauth.Token, id string, err error)
}

// MinTTL constraints for tokens.
type MinTTL struct {
    Access time.Duration
    ID     time.Duration
}

// Options control resolver behavior.
type Options struct {
    MinTTL MinTTL
    Tracer  obs.Tracer
    Metrics obs.Metrics
}

// Resolver ensures valid tokens are available with required TTLs using a token Store and Broker.
type Resolver struct {
    store   *tokens.Store
    broker  Broker
    opts    Options
    group   singleflight.Group
    mu      sync.RWMutex
}

func New(store *tokens.Store, broker Broker, opts Options) (*Resolver, error) {
    if store == nil || broker == nil {
        return nil, errors.New("resolver requires non-nil store and broker")
    }
    // fill noops
    if opts.Tracer == nil { opts.Tracer = obs.NoopTracer{} }
    if opts.Metrics == nil { opts.Metrics = obs.NoopMetrics{} }
    return &Resolver{store: store, broker: broker, opts: opts}, nil
}

// EnsureAccess returns an access token with at least MinTTL.Access remaining.
// If missing or expiring, it uses the broker to exchange or refresh and updates the store.
func (r *Resolver) EnsureAccess(ctx context.Context, key tokens.Key) (scyauth.Token, error) {
    // Fast-path: check store
    if v, ok := r.store.GetAccessID(key); ok {
        if v.Access.AccessToken != "" && ttlOK(v.AccessExpiry, r.opts.MinTTL.Access) {
            r.opts.Tracer.Debug("token_store_hit", map[string]any{"audience": key.Audience, "has_id": v.IDToken != ""})
            r.opts.Metrics.Inc("auth_token_store_hit", map[string]string{"audience": key.Audience}, 1)
            tok := v.Access
            tok.Expiry = v.AccessExpiry
            return tok, nil
        }
    }
    // Singleflight by key+audience
    res, err, _ := r.group.Do(sfKey("access", key), func() (any, error) {
        // Recheck inside singleflight to avoid duplicate broker calls
        if v, ok := r.store.GetAccessID(key); ok {
            if v.Access.AccessToken != "" && ttlOK(v.AccessExpiry, r.opts.MinTTL.Access) {
                tok := v.Access
                tok.Expiry = v.AccessExpiry
                return tok, nil
            }
        }
        acc, id, src, err := r.refreshOrExchange(ctx, key, r.opts.MinTTL.Access)
        if err != nil {
            return scyauth.Token{}, err
        }
        if src == "refresh" {
            r.opts.Tracer.Debug("token_broker_refresh", map[string]any{"audience": key.Audience})
            r.opts.Metrics.Inc("auth_token_broker_refresh", map[string]string{"audience": key.Audience}, 1)
        } else if src == "exchange" {
            r.opts.Tracer.Debug("token_broker_exchange", map[string]any{"audience": key.Audience})
            r.opts.Metrics.Inc("auth_token_broker_exchange", map[string]string{"audience": key.Audience}, 1)
        }
        // Update store
        av := tokens.AccessID{
            Access:       acc,
            AccessExpiry: acc.Expiry,
            Issuer:       key.Authority.Normalize().Issuer,
        }
        if id != "" { av.IDToken = id }
        r.store.SetAccessID(key, av)
        return acc, nil
    })
    if err != nil {
        return scyauth.Token{}, err
    }
    return res.(scyauth.Token), nil
}

// EnsureAccessAndID ensures both access and ID tokens meet MinTTL thresholds.
func (r *Resolver) EnsureAccessAndID(ctx context.Context, key tokens.Key) (access scyauth.Token, id string, err error) {
    // Fast path
    if v, ok := r.store.GetAccessID(key); ok {
        if v.Access.AccessToken != "" && ttlOK(v.AccessExpiry, r.opts.MinTTL.Access) && v.IDToken != "" {
            tok := v.Access
            tok.Expiry = v.AccessExpiry
            return tok, v.IDToken, nil
        }
    }
    // Singleflight by composite
    res, err, _ := r.group.Do(sfKey("access_id", key), func() (any, error) {
        if v, ok := r.store.GetAccessID(key); ok {
            if v.Access.AccessToken != "" && ttlOK(v.AccessExpiry, r.opts.MinTTL.Access) && v.IDToken != "" {
                tok := v.Access; tok.Expiry = v.AccessExpiry
                return [2]any{tok, v.IDToken}, nil
            }
        }
        acc, id, src, err := r.refreshOrExchange(ctx, key, min(r.opts.MinTTL.Access, r.opts.MinTTL.ID))
        if err != nil {
            return nil, err
        }
        if src == "refresh" {
            r.opts.Tracer.Debug("token_broker_refresh", map[string]any{"audience": key.Audience})
            r.opts.Metrics.Inc("auth_token_broker_refresh", map[string]string{"audience": key.Audience}, 1)
        } else if src == "exchange" {
            r.opts.Tracer.Debug("token_broker_exchange", map[string]any{"audience": key.Audience})
            r.opts.Metrics.Inc("auth_token_broker_exchange", map[string]string{"audience": key.Audience}, 1)
        }
        av := tokens.AccessID{Access: acc, AccessExpiry: acc.Expiry, Issuer: key.Authority.Normalize().Issuer}
        if id != "" { av.IDToken = id }
        r.store.SetAccessID(key, av)
        return [2]any{acc, id}, nil
    })
    if err != nil {
        return scyauth.Token{}, "", err
    }
    out := res.([2]any)
    return out[0].(scyauth.Token), out[1].(string), nil
}

// InvalidateAccess forces the next EnsureAccess() to obtain a fresh token.
func (r *Resolver) InvalidateAccess(key tokens.Key) {
    r.store.InvalidateAccess(key)
}

func (r *Resolver) refreshOrExchange(ctx context.Context, key tokens.Key, minTTL time.Duration) (scyauth.Token, string, string, error) {
    // Attempt refresh first if we have a refresh token
    if _, ok, _ := r.store.GetRefresh(key); ok {
        acc, id, err := r.broker.Refresh(ctx, key, minTTL)
        if err == nil {
            return acc, id, "refresh", nil
        }
        // fallthrough to exchange on refresh failure
    }
    acc, id, err := r.broker.Exchange(ctx, key, minTTL)
    if err != nil {
        return scyauth.Token{}, "", "", err
    }
    return acc, id, "exchange", nil
}

func ttlOK(expiry time.Time, minTTL time.Duration) bool {
    if expiry.IsZero() {
        return false
    }
    return time.Until(expiry) >= minTTL
}

func sfKey(kind string, k tokens.Key) string {
    n := k.Authority.Normalize()
    return fmt.Sprintf("%s|%s|%s|%s", kind, n.Issuer+"@"+n.Origin, k.Subject, k.Audience)
}

func min(a, b time.Duration) time.Duration {
    if a <= b { return a }
    return b
}
