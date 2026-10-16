package agently

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"sync"
	"time"

	identity "github.com/viant/agently-core/protocol/resource"
	"github.com/viant/agently-core/runtime/requestctx"
	auth "github.com/viant/agently-core/service/auth"
	datasources "github.com/viant/agently-core/service/datasource"
	"github.com/viant/agently-core/service/policy"
	"github.com/viant/forge/backend/types"
)

// Delivery authenticates selection provenance, never business read/write rights.
// Runtime directory binding completes before either public transport is served.
type nativeWindowDelivery struct {
	authority *nativeWindowAuthority
	signer    *identity.SelectionHMAC
	target    types.WindowTargetProof
	resolve   func(context.Context, string) (*identity.ResourceResolver, identity.ResourceRef, error)
	mu        sync.RWMutex
	users     auth.UserService
	canonical func(context.Context) (string, string)
}

func (d *nativeWindowDelivery) bindUsers(users auth.UserService) error {
	if d == nil || users == nil {
		return identity.ErrResourceDenied
	}
	if _, ok := users.(auth.UserByIDLookup); !ok {
		return identity.ErrResourceDenied
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.users != nil {
		return identity.ErrResourceDenied
	}
	d.users = users
	d.canonical = func(ctx context.Context) (string, string) {
		return auth.CanonicalUserID(ctx), auth.VerifiedProvider(ctx)
	}
	return nil
}

func (d *nativeWindowDelivery) actor(ctx context.Context) (identity.VerifiedActor, string, error) {
	if d == nil || d.authority == nil || ctx == nil || ctx.Err() != nil {
		return identity.VerifiedActor{}, "", identity.ErrResourceDenied
	}
	actor, err := d.authority.actor(ctx)
	if err != nil {
		return identity.VerifiedActor{}, "", nativeWindowAuthorityError(err)
	}
	d.mu.RLock()
	users, canonical := d.users, d.canonical
	d.mu.RUnlock()
	if users == nil || canonical == nil {
		return identity.VerifiedActor{}, "", identity.ErrResourceDenied
	}
	id, ingressProvider := canonical(ctx)
	if id == "" || ingressProvider == "" {
		return identity.VerifiedActor{}, "", identity.ErrResourceDenied
	}
	lookup, ok := users.(auth.UserByIDLookup)
	if !ok {
		return identity.VerifiedActor{}, "", identity.ErrResourceDenied
	}
	user, err := lookup.GetByID(ctx, id)
	if err != nil {
		return identity.VerifiedActor{}, "", nativeWindowAuthorityError(err)
	}
	// Ingress may say jwt while the actual canonical record uses the configured
	// OAuth provider. Verify the stored mapping, never an inferred provider alias.
	if user == nil || user.Disabled || user.ID != id || user.Subject != actor.Subject || user.Provider == "" {
		return identity.VerifiedActor{}, "", identity.ErrResourceDenied
	}
	mapped, err := users.GetBySubjectAndProvider(ctx, actor.Subject, user.Provider)
	if err != nil {
		return identity.VerifiedActor{}, "", nativeWindowAuthorityError(err)
	}
	if mapped == nil || mapped.Disabled || mapped.ID != id || mapped.Subject != actor.Subject || mapped.Provider != user.Provider || !actor.Valid(time.Now()) || ctx.Err() != nil {
		return identity.VerifiedActor{}, "", identity.ErrResourceDenied
	}
	return actor, id, nil
}

func nativeCanonicalDigest(id string) string {
	raw, _ := json.Marshal([]string{"native.window.owner.v1", id})
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:])
}

func (d *nativeWindowDelivery) emit(ctx context.Context, window *types.Window, parameters map[string]any) (*identity.SelectionBinding, error) {
	if window == nil || window.Resource == nil || window.ResourceTarget == nil || d == nil || d.signer == nil || d.resolve == nil {
		return nil, identity.ErrResourceDenied
	}
	actor, canonical, err := d.actor(ctx)
	if err != nil {
		return nil, err
	}
	resolver, ref, err := d.resolve(ctx, window.Resource.URI)
	if err != nil || ref.URI != window.Resource.URI {
		return nil, identity.ErrResourceDenied
	}
	raw, _, err := resolver.ReadResolved(ctx, *window.Resource)
	if err != nil {
		return nil, nativeWindowAuthorityError(err)
	}
	variant, err := types.SelectWindowResource(raw, window.ResourceTarget)
	if err != nil || d.target.Verify(ctx, *window.Resource, *window.ResourceTarget, variant.Fingerprint, window.ResourceTarget.SelectionToken) != nil {
		return nil, identity.ErrResourceDenied
	}
	if window.ResourceTarget.ExecutionProof != nil || len(window.ResourceTarget.DependencyPins) != 0 {
		// This native source emits confined format-2 local closures, not remote
		// datasource resource/proof artifacts. Never manufacture their authority.
		return nil, identity.ErrResourceDenied
	}
	instance := make([]byte, 16)
	if _, err := rand.Read(instance); err != nil {
		return nil, err
	}
	target, err := json.Marshal(window.ResourceTarget)
	if err != nil {
		return nil, identity.ErrResourceDenied
	}
	return d.signer.Bind(ctx, *window.Resource, target, actor, hex.EncodeToString(instance)+"."+nativeCanonicalDigest(canonical), parameters)
}

type nativeDeliveryKey struct{}
type nativeDeliveryGrant struct {
	owner      *nativeWindowDelivery
	actor      identity.VerifiedActor
	canonical  string
	original   identity.ResolvedResource
	parameters string
	prepared   *datasources.PreparedDelivery
}

func (d *nativeWindowDelivery) prepare(ctx context.Context, in datasources.OriginalDelivery) (context.Context, *datasources.PreparedDelivery, error) {
	if d == nil || d.signer == nil || d.target == nil || d.resolve == nil || ctx == nil || ctx.Err() != nil || in.Resource == nil || in.Target == nil || in.SelectionBinding == nil || requestctx.HasResourceReadDispatch(ctx) || requestctx.ResourceReadAssociation(ctx) != ([32]byte{}) || requestctx.WindowReadDecisionActive(ctx) || ctx.Value(nativeDeliveryKey{}) != nil || in.ResourceProof != nil || in.Target.ExecutionProof != nil || len(in.Target.DependencyPins) != 0 {
		return nil, nil, identity.ErrResourceDenied
	}
	actor, canonical, err := d.actor(ctx)
	if err != nil {
		return nil, nil, err
	}
	parts := strings.Split(in.SelectionBinding.InstanceID, ".")
	if len(parts) != 2 || len(parts[0]) != 32 || parts[1] != nativeCanonicalDigest(canonical) {
		return nil, nil, identity.ErrResourceDenied
	}
	target, err := json.Marshal(in.Target)
	if err != nil || d.signer.VerifyOriginalForNewDelivery(ctx, *in.Resource, target, *in.SelectionBinding, in.WindowParameters, actor) != nil {
		return nil, nil, identity.ErrResourceDenied
	}
	grant := &nativeDeliveryGrant{owner: d, actor: actor, canonical: canonical, original: *in.Resource, parameters: in.SelectionBinding.ParametersDigest}
	ctx = context.WithValue(ctx, nativeDeliveryKey{}, grant)
	resolver, ref, err := d.resolve(ctx, in.Resource.URI)
	if err != nil || ref.URI != in.Resource.URI {
		return nil, nil, identity.ErrResourceDenied
	}
	// Preserve a still-current delivery exactly. A fresh credential revision may
	// require separate NEW-operation authority despite the same stable actor.
	candidates, err := resolver.Source.Candidates(ctx, mustNativeURI(in.Resource.URI))
	if err != nil {
		return nil, nil, nativeWindowAuthorityError(err)
	}
	decision, err := resolver.Policy.SelectRevision(ctx, identity.ResourceRef{URI: in.Resource.URI, Revision: in.Resource.Selector()}, candidates)
	if err != nil {
		return nil, nil, nativeWindowAuthorityError(err)
	}
	var raw json.RawMessage
	var fresh *identity.ResolvedResource
	if in.Resource.ValidUntil.After(time.Now()) && decision.AuthorityBinding == in.Resource.AuthorityBinding {
		raw, fresh, err = resolver.ReadResolved(ctx, *in.Resource)
	} else {
		raw, fresh, err = resolver.RenewResolved(ctx, *in.Resource)
	}
	if err != nil {
		return nil, nil, nativeWindowAuthorityError(err)
	}
	variant, err := types.SelectWindowResource(raw, in.Target)
	if err != nil || fresh == nil {
		return nil, nil, identity.ErrResourceDenied
	}
	freshTarget := *in.Target
	if !nativeJSONEqual(fresh, in.Resource) {
		freshTarget.SelectionToken, err = d.target.Sign(ctx, *fresh, freshTarget, variant.Fingerprint)
	} else {
		err = d.target.Verify(ctx, *fresh, freshTarget, variant.Fingerprint, freshTarget.SelectionToken)
	}
	if err != nil {
		return nil, nil, identity.ErrResourceDenied
	}
	grant.prepared = &datasources.PreparedDelivery{Resource: fresh, Target: &freshTarget}
	return ctx, grant.prepared, nil
}

func mustNativeURI(raw string) identity.ResourceURI {
	uri, _ := identity.ParseResourceURI(raw)
	return uri
}

// Signed NEW delivery renews namespace/artifact authority, never window business
// capabilities. Ordinary OPEN metadata keeps the existing native policy.
type nativeDeliveryPolicy struct {
	base     *nativeWindowPolicy
	delivery *nativeWindowDelivery
}

func (p *nativeDeliveryPolicy) grant(ctx context.Context) (*nativeDeliveryGrant, error) {
	if p == nil || p.delivery == nil || ctx == nil || ctx.Err() != nil {
		return nil, identity.ErrResourceDenied
	}
	grant, _ := ctx.Value(nativeDeliveryKey{}).(*nativeDeliveryGrant)
	if grant == nil || grant.owner != p.delivery {
		return nil, identity.ErrResourceDenied
	}
	actor, canonical, err := p.delivery.actor(ctx)
	if err != nil {
		return nil, err
	}
	if canonical != grant.canonical || actor.Subject != grant.actor.Subject || actor.Issuer != grant.actor.Issuer || actor.TenantID != grant.actor.TenantID || actor.AccountID != grant.actor.AccountID || actor.IdentityRevision != grant.actor.IdentityRevision || !grant.actor.Valid(time.Now()) {
		return nil, identity.ErrResourceDenied
	}
	return grant, nil
}

func (p *nativeDeliveryPolicy) AuthorizeRenewal(ctx context.Context, original identity.ResolvedResource) (time.Time, error) {
	grant, err := p.grant(ctx)
	if err != nil {
		return time.Time{}, err
	}
	if original != grant.original {
		return time.Time{}, identity.ErrResourceDenied
	}
	return grant.actor.ValidUntil, nil
}

func (p *nativeDeliveryPolicy) SelectRevision(ctx context.Context, ref identity.ResourceRef, candidates []identity.ResourceCandidate) (identity.ResourceDecision, error) {
	if ctx == nil || ctx.Value(nativeDeliveryKey{}) == nil {
		return p.base.SelectRevision(ctx, ref, candidates)
	}
	grant, err := p.grant(ctx)
	if err != nil {
		return identity.ResourceDecision{}, err
	}
	if ref.URI != grant.original.URI || ref.Revision != "" && ref.Revision != grant.original.Selector() {
		return identity.ResourceDecision{}, identity.ErrResourceDenied
	}
	if _, ok := p.base.authority.allowed[ref.URI]; !ok {
		return identity.ResourceDecision{}, identity.ErrResourceDenied
	}
	bound, lease, err := policy.ResolveAuthorityBinding(ctx, p.base.authority.principal)
	if err != nil {
		return identity.ResourceDecision{}, nativeWindowAuthorityError(err)
	}
	if grant.actor.ValidUntil.Before(lease) {
		lease = grant.actor.ValidUntil
	}
	for _, candidate := range candidates {
		if candidate == grant.original.ResourceCandidate && candidate.Valid() {
			return identity.ResourceDecision{Candidate: candidate, AuthorityBinding: bound, ValidUntil: lease}, nil
		}
	}
	return identity.ResourceDecision{}, identity.ErrResourceStale
}

func nativeJSONEqual(left, right any) bool {
	a, ea := json.Marshal(left)
	b, eb := json.Marshal(right)
	return ea == nil && eb == nil && string(a) == string(b)
}

func nativeSelectionContains(selection, value any) bool {
	raw, err := json.Marshal(selection)
	if err != nil {
		return false
	}
	var values []json.RawMessage
	if json.Unmarshal(raw, &values) != nil {
		return nativeJSONEqual(selection, value)
	}
	for _, item := range values {
		var candidate any
		decoder := json.NewDecoder(strings.NewReader(string(item)))
		decoder.UseNumber()
		if decoder.Decode(&candidate) == nil && nativeJSONEqual(candidate, value) {
			return true
		}
	}
	return false
}

var _ identity.RenewalAuthorizer = (*nativeDeliveryPolicy)(nil)
