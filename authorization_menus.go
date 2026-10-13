package agently

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/viant/agently-core/app/executor"
	identity "github.com/viant/agently-core/protocol/resource"
	resources "github.com/viant/agently-core/service/resource"
	windowloader "github.com/viant/agently-core/service/ui/window"
	workspaceui "github.com/viant/agently-core/service/workspace"
	"github.com/viant/authz/gating"
)

const hostMenuOperation = "menu.view"

// HostMenuSource is explicit trusted publication/consumption configuration.
// An external source has an owner and no local path. Local paths publish only
// through the explicitly configured internal provider; authoring alone is inert.
type HostMenuSource struct {
	URI              string `json:"uri"`
	LocalLayoutPath  string `json:"localLayoutPath,omitempty"`
	ProviderIdentity string `json:"providerIdentity,omitempty"`
	Revision         string `json:"revision,omitempty"`
}

func validateHostMenuSources(config *hostAuthorizationFile, path string) error {
	seen := map[string]bool{}
	for i := range config.MenuSources {
		source := &config.MenuSources[i]
		uri, err := identity.ParseResourceURI(source.URI)
		if err != nil || uri.Kind != "menu" || uri.Namespace == "users" && source.LocalLayoutPath != "" {
			return fmt.Errorf("menu source requires explicit supported logical menu URI")
		}
		if source.LocalLayoutPath == "" && source.ProviderIdentity == "" {
			return fmt.Errorf("external menu source requires providerIdentity")
		}
		if source.Revision != "" {
			probe := identity.ResourceCandidate{Kind: identity.StampedCandidate, Revision: source.Revision, ContentFingerprint: identity.ContentFingerprint(nil)}
			if source.Revision != identity.WorkingCandidate && !probe.Valid() {
				return fmt.Errorf("invalid menu revision")
			}
		}
		if source.LocalLayoutPath != "" && !filepath.IsAbs(source.LocalLayoutPath) {
			source.LocalLayoutPath = filepath.Join(filepath.Dir(path), source.LocalLayoutPath)
		}
		key := source.ProviderIdentity + "\x00" + source.URI
		if seen[key] {
			return fmt.Errorf("duplicate menu source")
		}
		seen[key] = true
	}
	return nil
}

func configuredMenuBindings(owner string, config *hostAuthorizationFile, prepared *executor.PreparedAuthorization, shared ...*internalWindowSnapshot) ([]resources.LocalResourceBinding, error) {
	var bindings []resources.LocalResourceBinding
	// Each explicit trusted filesystem root joins the existing native snapshot
	// mechanism. Authored bytes are compiled once; actors and menu policy stay
	// request-scoped in the unchanged resolver and visibility callbacks.
	definitions := map[string][]resources.NativeSnapshotDefinition{}
	for _, source := range config.MenuSources {
		if source.LocalLayoutPath == "" {
			continue
		}
		if owner == "" || source.ProviderIdentity != "" && source.ProviderIdentity != owner {
			return nil, fmt.Errorf("local menu requires its configured internal provider")
		}
		if prepared == nil {
			return nil, identity.ErrResourceDenied
		}
		if len(shared) > 0 && shared[0] != nil && shared[0].menuURIs[source.URI] {
			entry := source
			bindings = append(bindings, resources.LocalResourceBinding{URI: entry.URI, Title: "Navigation", FormatVersion: 1, Resolver: func(ctx context.Context, a identity.VerifiedActor, _ string) (*identity.ResourceResolver, error) {
				if err := internalActorVerifier(prepared)(ctx, a); err != nil {
					return nil, err
				}
				resolver, err := prepared.ResourceResolver(hostMenuOperation, shared[0], nil)
				if err != nil {
					return nil, err
				}
				resolver.ProviderIdentity = owner
				return resolver, nil
			}})
			continue
		}
		root, file := filepath.Dir(source.LocalLayoutPath), filepath.Base(source.LocalLayoutPath)
		definition, err := resources.SnapshotLayoutMenuDefinition(file, source.URI)
		if err != nil {
			return nil, err
		}
		definitions[root] = append(definitions[root], definition)

	}
	roots := make([]string, 0, len(definitions))
	for root := range definitions {
		roots = append(roots, root)
	}
	sort.Strings(roots)
	for _, root := range roots {
		snapshot, err := resources.NewNativeAssetSnapshot(context.Background(), root, definitions[root], resources.NativeSnapshotOptions{})
		if err != nil {
			return nil, err
		}
		group, err := snapshot.Bindings(owner, func(ctx context.Context, actor identity.VerifiedActor, _ identity.ResourceURI, _ string) (identity.ResourceRevisionPolicy, error) {
			if err := internalActorVerifier(prepared)(ctx, actor); err != nil {
				return nil, err
			}
			resolver, err := prepared.ResourceResolver(hostMenuOperation, candidateSource{}, nil)
			if err != nil {
				return nil, err
			}
			return resolver.Policy, nil
		})
		if err != nil {
			return nil, err
		}
		bindings = append(bindings, group...)
	}

	return bindings, nil
}

func configuredMenuVisibility(config *hostAuthorizationFile, principals gating.PrincipalResolver, namespace resources.LocalResourceAuthorizer) (resources.MenuActionVisibility, error) {
	visible, err := internalWindowVisibility(config, principals)
	if err != nil {
		return nil, err
	}
	aliases := map[string]resources.LocalResourceBinding{}
	for _, group := range [][]windowloader.ResourceBinding{config.WindowResources, config.FrameworkWindowResources} {
		for _, entry := range group {
			binding := resources.LocalResourceBinding{URI: entry.URI, Title: entry.WindowKey, FormatVersion: 2}
			for _, key := range []string{entry.URI, entry.WindowKey, entry.CatalogID} {
				if key == "" {
					continue
				}
				if existing, ok := aliases[key]; ok && existing.URI != binding.URI {
					return nil, fmt.Errorf("ambiguous menu window binding")
				}
				aliases[key] = binding
			}
		}
	}
	return func(ctx context.Context, actor identity.VerifiedActor, _ identity.ResourceURI, raw json.RawMessage) (bool, error) {
		var action struct {
			Type      string `json:"type"`
			WindowKey string `json:"windowKey"`
			Provider  string `json:"provider"`
		}
		if json.Unmarshal(raw, &action) != nil {
			return false, identity.ErrResourceDenied
		}
		if action.Type != "window" || action.Provider != "" && action.Provider != "workspace" {
			return false, nil
		}
		binding, exists := aliases[action.WindowKey]
		if !exists {
			return false, nil
		}
		uri, err := identity.ParseResourceURI(binding.URI)
		if err != nil {
			return false, err
		}
		if err = namespace(ctx, actor, uri, "resource.list"); err != nil {
			if errors.Is(err, identity.ErrResourceDenied) {
				return false, nil
			}
			return false, err
		}
		return visible(ctx, actor, binding)
	}, nil
}

func configureLayoutMenuReader(sources []HostMenuSource, localOwner string, prepared *executor.PreparedAuthorization) func(*executor.Runtime) (workspaceui.LayoutMenuReader, error) {
	if len(sources) == 0 {
		return nil
	}
	configured := append([]HostMenuSource(nil), sources...)
	return func(rt *executor.Runtime) (workspaceui.LayoutMenuReader, error) {
		if rt == nil || rt.PrimitiveProviders == nil || prepared == nil {
			return nil, identity.ErrResourceDenied
		}
		for i := range configured {
			if configured[i].LocalLayoutPath != "" {
				configured[i].ProviderIdentity = localOwner
			}
			if configured[i].ProviderIdentity == "" {
				return nil, fmt.Errorf("menu consumer requires explicit provider")
			}
		}
		return func(ctx context.Context) (result *workspaceui.LayoutMenuRead, resultErr error) {
			scoped, finish, err := rt.PrimitiveProviders.BeginReadSequence(ctx, nil)
			if err != nil {
				return nil, err
			}
			ctx = scoped
			var finishOnce sync.Once
			var finishErr error
			closeRead := func() error { finishOnce.Do(func() { finishErr = finish() }); return finishErr }
			defer func() {
				if resultErr != nil {
					_ = closeRead()
				}
			}()
			heldActor, err := prepared.ResourceActor(ctx)
			if err != nil {
				return nil, err
			}
			checkActor := func() error { return internalActorVerifier(prepared)(ctx, heldActor) }
			expires := heldActor.ValidUntil
			constrainLease := func(until time.Time) {
				if until.Before(expires) {
					expires = until
				}
			}
			type selectedMenu struct {
				connection resources.Connection
				source     identity.ResolvedResource
				admission  identity.ResourceDecision
				projection string
			}
			selected := make([]selectedMenu, 0, len(configured))
			applications := make([]json.RawMessage, 0)
			admit := func(ctx context.Context, ref identity.ResourceRef, pin identity.ResolvedResource) (identity.ResourceDecision, error) {
				resolver, err := prepared.ResourceResolver(hostMenuOperation, candidateSource{}, nil)
				if err != nil {
					return identity.ResourceDecision{}, err
				}
				decision, err := resolver.Policy.SelectRevision(ctx, ref, []identity.ResourceCandidate{pin.ResourceCandidate})
				if err != nil {
					return identity.ResourceDecision{}, err
				}
				if decision.Candidate != pin.ResourceCandidate || decision.AuthorityBinding == "" || !decision.ValidUntil.After(time.Now()) {
					return identity.ResourceDecision{}, identity.ErrResourceDenied
				}
				return decision, nil
			}
			for _, source := range configured {
				if err := checkActor(); err != nil {
					return nil, err
				}
				connection, err := rt.PrimitiveProviders.ConnectionForProvider(ctx, source.ProviderIdentity)
				if err != nil {
					return nil, err
				}
				ref := identity.ResourceRef{URI: source.URI, Revision: source.Revision}
				got, err := rt.PrimitiveProviders.GetMenu(ctx, connection, ref, nil)
				if err != nil {
					return nil, err
				}
				decision, err := admit(ctx, ref, *got.ResolvedResource)
				if err != nil {
					return nil, err
				}
				constrainLease(got.ResolvedResource.ValidUntil)
				constrainLease(decision.ValidUntil)
				var envelope struct {
					Applications []json.RawMessage `json:"applications"`
				}
				if json.Unmarshal(got.MenuProjection.DefinitionBytes, &envelope) != nil || envelope.Applications == nil {
					return nil, identity.ErrResourceDenied
				}
				applications = append(applications, envelope.Applications...)
				selected = append(selected, selectedMenu{connection: connection, source: *got.ResolvedResource, admission: decision, projection: got.MenuProjection.ContentFingerprint})
			}
			raw, err := json.Marshal(applications)
			if err != nil {
				return nil, err
			}
			revisionParts := make([]string, 0, len(selected))
			for _, held := range selected {
				revisionParts = append(revisionParts, held.source.ProviderIdentity+"\x00"+held.source.URI+"\x00"+held.source.Selector()+"\x00"+held.source.ContentFingerprint+"\x00"+held.projection)
			}
			revisionBytes, err := json.Marshal(revisionParts)
			if err != nil {
				return nil, err
			}
			return &workspaceui.LayoutMenuRead{Applications: raw, Revision: identity.ContentFingerprint(revisionBytes), Close: closeRead, Revalidate: func(releaseCtx context.Context) error {
				if releaseCtx == nil || releaseCtx.Err() != nil {
					return identity.ErrResourceDenied
				}
				if err := internalActorVerifier(prepared)(releaseCtx, heldActor); err != nil {
					return err
				}
				if err := checkActor(); err != nil {
					return err
				}
				for _, held := range selected {
					if !held.source.ValidUntil.After(time.Now()) || !held.admission.ValidUntil.After(time.Now()) {
						return identity.ErrResourceDenied
					}
					got, err := rt.PrimitiveProviders.GetMenu(ctx, held.connection, identity.ResourceRef{URI: held.source.URI, Revision: held.source.Selector()}, &held.source)
					if err != nil {
						return err
					}
					if got.MenuProjection.ContentFingerprint != held.projection {
						return identity.ErrResourceDenied
					}
					decision, err := admit(ctx, identity.ResourceRef{URI: held.source.URI, Revision: held.source.Selector()}, held.source)
					if err != nil {
						return err
					}
					constrainLease(got.ResolvedResource.ValidUntil)
					constrainLease(decision.ValidUntil)
					if decision.AuthorityBinding != held.admission.AuthorityBinding || !held.source.ValidUntil.After(time.Now()) || !held.admission.ValidUntil.After(time.Now()) {
						return identity.ErrResourceDenied
					}
				}
				if err := checkActor(); err != nil {
					return err
				}
				if err := closeRead(); err != nil {
					return err
				}
				if !expires.After(time.Now()) {
					return identity.ErrResourceDenied
				}
				return ctx.Err()
			}}, nil
		}, nil
	}
}

// configuredNativeMenuDefinitions joins only explicitly declared local files
// confined to the already trusted workspace snapshot root. External trusted
// roots are handled separately by the same native snapshot mechanism.
func configuredNativeMenuDefinitions(root string, sources []HostMenuSource) ([]resources.NativeSnapshotDefinition, error) {
	if root == "" {
		return nil, nil
	}
	var defs []resources.NativeSnapshotDefinition
	for _, source := range sources {
		if source.LocalLayoutPath == "" {
			continue
		}
		relative, err := filepath.Rel(root, source.LocalLayoutPath)
		if err != nil {
			return nil, err
		}
		relative = filepath.ToSlash(relative)
		if relative == ".." || strings.HasPrefix(relative, "../") {
			continue
		}
		definition, err := resources.SnapshotLayoutMenuDefinition(relative, source.URI)
		if err != nil {
			return nil, err
		}
		defs = append(defs, definition)

	}
	return defs, nil
}
