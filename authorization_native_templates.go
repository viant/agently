package agently

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	identity "github.com/viant/agently-core/protocol/resource"
	"github.com/viant/agently-core/service/policy"
	resources "github.com/viant/agently-core/service/resource"
	windowloader "github.com/viant/agently-core/service/ui/window"
	"github.com/viant/authz/gating"
)

func nativeWindowViewBinding(config *hostAuthorizationFile, binding policy.ResourceRevisionBinding) bool {
	if !config.nativeWindows || binding.Operation != policy.OperationWindowView {
		return false
	}
	for _, group := range [][]windowloader.ResourceBinding{config.WindowResources, config.FrameworkWindowResources} {
		for _, b := range group {
			if binding.URI == b.URI {
				return true
			}
		}
	}
	return false
}
func configuredNativeWindowResolver(config *hostAuthorizationFile, source identity.ResourceSource, authority *nativeWindowAuthority) (func(context.Context, string) (*identity.ResourceResolver, identity.ResourceRef, error), error) {
	aliases := map[string]string{}
	for _, group := range [][]windowloader.ResourceBinding{config.WindowResources, config.FrameworkWindowResources} {
		for _, b := range group {
			keys := map[string]bool{b.URI: true, b.WindowKey: true}
			if b.CatalogID != "" {
				keys[b.CatalogID] = true
			}
			for key := range keys {
				if _, ok := aliases[key]; ok {
					return nil, fmt.Errorf("ambiguous configured native window")
				}
				aliases[key] = b.URI
			}
		}
	}
	return func(ctx context.Context, key string) (*identity.ResourceResolver, identity.ResourceRef, error) {
		uri, ok := aliases[key]
		if !ok {
			return nil, identity.ResourceRef{}, identity.ErrResourceDenied
		}
		return &identity.ResourceResolver{Source: source, Policy: &nativeWindowPolicy{authority: authority}}, identity.ResourceRef{URI: uri}, nil
	}, nil
}

// A native read scope is owned by the configured catalog. Request arguments
// cannot create it, and its lifetime ends before its caller resumes execution.
type nativeWindowReadKey struct{}
type nativeWindowRead struct {
	owner  *nativeWindowAuthority
	active atomic.Bool
}
type nativeWindowAuthority struct {
	principal       func(context.Context) (gating.Principal, error)
	allowed         map[string]resources.LocalResourceBinding
	visibilityLease internalWindowVisibilityLease
}

func (n *nativeWindowAuthority) begin(ctx context.Context) (context.Context, func()) {
	state := &nativeWindowRead{owner: n}
	state.active.Store(true)
	return context.WithValue(ctx, nativeWindowReadKey{}, state), func() { state.active.Store(false) }
}
func (n *nativeWindowAuthority) active(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	state, _ := ctx.Value(nativeWindowReadKey{}).(*nativeWindowRead)
	return state != nil && state.owner == n && state.active.Load()
}
func (n *nativeWindowAuthority) actor(ctx context.Context) (identity.VerifiedActor, error) {
	principal, err := n.principal(ctx)
	if err != nil {
		return identity.VerifiedActor{}, err
	}
	a := identity.VerifiedActor{Subject: principal.Facts.Subject, Issuer: principal.Facts.Issuer, TenantID: principal.Facts.Tenant, AccountID: principal.AccountID, IdentityRevision: principal.IdentityRevision, ValidUntil: principal.Facts.ValidUntil}
	if !a.Valid(time.Now()) {
		return identity.VerifiedActor{}, policy.ErrIdentityRejected
	}
	return a, nil
}
func verifyActor(resolve resources.ActorResolver) resources.ActorVerifier {
	return func(ctx context.Context, held identity.VerifiedActor) error {
		current, err := resolve(ctx)
		if err != nil {
			return err
		}
		if !held.Valid(time.Now()) || !current.Valid(time.Now()) || held.Subject != current.Subject || held.Issuer != current.Issuer || held.TenantID != current.TenantID || held.AccountID != current.AccountID || held.IdentityRevision != current.IdentityRevision {
			return identity.ErrResourceDenied
		}
		return nil
	}
}
func (n *nativeWindowAuthority) resolveActor(fallback resources.ActorResolver) resources.ActorResolver {
	return func(ctx context.Context) (identity.VerifiedActor, error) {
		if n.active(ctx) {
			return n.actor(ctx)
		}
		return fallback(ctx)
	}
}
func (n *nativeWindowAuthority) admit(ctx context.Context, pin identity.ResolvedResource) error {
	d, err := (&nativeWindowPolicy{authority: n}).SelectRevision(ctx, identity.ResourceRef{URI: pin.URI, Revision: pin.Selector()}, []identity.ResourceCandidate{pin.ResourceCandidate})
	if err != nil {
		return err
	}
	if d.Candidate != pin.ResourceCandidate || d.AuthorityBinding != pin.AuthorityBinding || !pin.ValidUntil.After(time.Now()) {
		return identity.ErrResourceDenied
	}
	return nil
}

// Native YAML is an operator-installed static template, with one current
// authored candidate. Its URI adds identity, not a new mandatory ACL document.
// Authored entity/capability checks still execute in the existing open/data path.
type nativeWindowPolicy struct{ authority *nativeWindowAuthority }

func (p *nativeWindowPolicy) SelectRevision(ctx context.Context, ref identity.ResourceRef, candidates []identity.ResourceCandidate) (identity.ResourceDecision, error) {
	n := p.authority
	binding, ok := n.allowed[ref.URI]
	if !ok || ctx == nil || ctx.Err() != nil {
		return identity.ResourceDecision{}, identity.ErrResourceDenied
	}
	a, err := n.actor(ctx)
	if err != nil {
		return identity.ResourceDecision{}, err
	}
	roleLease := a.ValidUntil
	if n.visibilityLease != nil {
		allowed, lease, err := n.visibilityLease(ctx, a, binding)
		if err != nil {
			return identity.ResourceDecision{}, err
		}
		if !allowed {
			return identity.ResourceDecision{}, identity.ErrResourceDenied
		}
		if lease.Before(roleLease) {
			roleLease = lease
		}
	}
	bound, lease, err := policy.ResolveAuthorityBinding(ctx, n.principal)
	if err != nil {
		return identity.ResourceDecision{}, err
	}
	if roleLease.Before(lease) {
		lease = roleLease
	}
	if !lease.After(time.Now()) {
		return identity.ResourceDecision{}, identity.ErrResourceDenied
	}
	for _, c := range candidates {
		if c.Valid() && c.Kind == identity.WorkingCandidate && (ref.Revision == "" || ref.Revision == c.Selector()) {
			return identity.ResourceDecision{Candidate: c, AuthorityBinding: bound, ValidUntil: lease}, nil
		}
	}
	return identity.ResourceDecision{}, identity.ErrResourceDenied
}

// The catalog itself establishes native read scopes after resolving trusted
// bindings. Avoid opening a user-info profile before that classification.
type nativeWindowMetadataScope struct{}

func (nativeWindowMetadataScope) BeginMetadataRead(ctx context.Context) (context.Context, func() error, error) {
	if ctx == nil || ctx.Err() != nil {
		return nil, nil, identity.ErrResourceDenied
	}
	return ctx, func() error { return ctx.Err() }, nil
}
func (nativeWindowMetadataScope) WithoutMetadataRead(ctx context.Context) context.Context {
	return context.WithValue(ctx, nativeWindowReadKey{}, (*nativeWindowRead)(nil))
}
