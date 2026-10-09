package agently

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/viant/afs"
	"github.com/viant/agently-core/app/executor"
	dsproto "github.com/viant/agently-core/protocol/datasource"
	identity "github.com/viant/agently-core/protocol/resource"
	"github.com/viant/agently-core/service/policy"
	forgeservice "github.com/viant/agently-core/service/primitiveprovider"
	windowloader "github.com/viant/agently-core/service/ui/window"
	"github.com/viant/agently/metadata"
	"github.com/viant/forge/backend/handlers"
	"github.com/viant/forge/backend/service/meta"
	"github.com/viant/forge/backend/types"
)

// embeddedWindowSource serves only operator-declared framework definitions from
// the linked metadata filesystem. Workspace files cannot shadow this source.
type embeddedWindowSource struct {
	keys    map[string]string
	bundle  bool
	targets map[string][]types.WindowTarget
}

// NewFrameworkWindowResourceSource exposes declared linked framework metadata
// for explicit resource imports. It grants no visibility or execution rights.
func NewFrameworkWindowResourceSource(bindings []windowloader.ResourceBinding) (identity.ResourceSource, error) {
	return newEmbeddedWindowSource(bindings)
}

// NewFrameworkWindowBundleSource captures every declared linked target as one
// immutable parent definition. Existing singleton stamps retain their bytes.
func NewFrameworkWindowBundleSource(bindings []windowloader.ResourceBinding) (identity.ResourceSource, error) {
	s, err := newEmbeddedWindowSource(bindings)
	if err != nil {
		return nil, err
	}
	s.bundle = true
	return s, nil
}

func newEmbeddedWindowSource(bindings []windowloader.ResourceBinding) (*embeddedWindowSource, error) {
	s := &embeddedWindowSource{keys: map[string]string{}, targets: map[string][]types.WindowTarget{}}
	for _, binding := range bindings {
		uri, err := identity.ParseResourceURI(binding.URI)
		key := binding.WindowKey
		if err != nil || uri.Kind != "window" || key == "" || path.Clean(key) != key || strings.HasPrefix(key, "/") || strings.HasPrefix(key, "../") || strings.ContainsAny(key, "\\:\x00?#") || key == "." || key == ".." || s.keys[binding.URI] != "" {
			return nil, fmt.Errorf("invalid embedded window resource binding")
		}
		s.keys[binding.URI] = key
		s.targets[binding.URI] = append([]types.WindowTarget(nil), binding.Targets...)
	}
	return s, nil
}
func (s *embeddedWindowSource) load(ctx context.Context, uri identity.ResourceURI) (json.RawMessage, error) {
	if s.bundle {
		return s.loadBundle(ctx, uri)
	}
	return s.loadTarget(ctx, uri, nil)
}
func (s *embeddedWindowSource) loadBundle(ctx context.Context, uri identity.ResourceURI) (json.RawMessage, error) {
	e := types.WindowResourceEnvelope{SchemaVersion: 2, Format: types.WindowBundleFormat, Variants: map[string]types.WindowResourceVariant{}}
	seen := map[string]bool{}
	for _, target := range append(windowloader.StandardWindowTargets(), s.targets[uri.String()]...) {
		normalized, err := target.Normalize()
		if err != nil || normalized.SelectionToken != "" || len(normalized.Capabilities) > 0 || seen[normalized.ProfileKey()] {
			return nil, fmt.Errorf("invalid/duplicate linked target")
		}
		seen[normalized.ProfileKey()] = true
		var profile *types.WindowTarget
		if normalized.ProfileKey() != (types.WindowTarget{}).ProfileKey() {
			profile = &normalized
		}
		raw, err := s.loadTarget(ctx, uri, profile)
		if err != nil {
			return nil, err
		}
		var definition types.Window
		if json.Unmarshal(raw, &definition) != nil {
			return nil, identity.ErrResourceDenied
		}
		definition.ResourceDependencies = map[string]string{}
		variant := types.WindowResourceVariant{Window: &definition, DataSources: map[string]json.RawMessage{}}
		for id, inline := range definition.DataSource {
			descriptor, err := json.Marshal(dsproto.DataSource{DataSource: inline, ID: id})
			if err != nil {
				return nil, err
			}
			descriptor, err = types.CanonicalWindowDescriptor(descriptor)
			if err != nil {
				return nil, err
			}
			variant.DataSources[id] = descriptor
			definition.ResourceDependencies[id] = identity.ContentFingerprint(descriptor)
		}
		key, err := types.WindowVariantFingerprint(variant)
		if err != nil {
			return nil, err
		}
		e.Variants[key] = variant
		e.Targets = append(e.Targets, types.WindowTargetBinding{Target: normalized, Variant: key})
	}
	if err := e.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(e)
}
func (s *embeddedWindowSource) loadTarget(ctx context.Context, uri identity.ResourceURI, target *types.WindowTarget) (json.RawMessage, error) {
	key := s.keys[uri.String()]
	if key == "" {
		return nil, identity.ErrResourceDenied
	}
	parts := strings.SplitN(key, "/", 2)
	sub := ""
	if len(parts) == 2 {
		sub = parts[1]
	}
	const base = "embed://localhost/window"
	loader := meta.New(afs.New(), base, &metadata.FS)
	var requested *meta.TargetContext
	if target != nil {
		requested = &meta.TargetContext{Platform: target.Platform, FormFactor: target.FormFactor, Surface: target.Surface, Capabilities: target.Capabilities}
	}
	definition, err := handlers.LoadWindow(ctx, loader, base, parts[0], sub, requested)
	if err != nil {
		return nil, err
	}
	definition.Resource = nil
	definition.ResourceTarget = nil
	return json.Marshal(definition)
}

// NewFrameworkLegacyTargetSupport establishes byte equivalence from the linked
// framework artifact, only for explicit host declarations. A match applies to
// that exact URI+content fingerprint, never another historical/default body.
// No runtime request reads a source directory or substitutes current metadata.
func NewFrameworkLegacyTargetSupport(ctx context.Context, bindings []windowloader.ResourceBinding, targets []types.WindowTarget) (forgeservice.WindowLegacyTargetSupport, error) {
	source, err := newEmbeddedWindowSource(bindings)
	if err != nil {
		return nil, err
	}
	proven := map[string]map[string]bool{}
	for _, binding := range bindings {
		uri, _ := identity.ParseResourceURI(binding.URI)
		base, err := source.load(ctx, uri)
		if err != nil {
			return nil, err
		}
		key := binding.URI + "|" + identity.ContentFingerprint(base)
		for _, target := range targets {
			normalized, err := target.Normalize()
			if err != nil || len(normalized.Capabilities) > 0 || normalized.SelectionToken != "" {
				return nil, fmt.Errorf("invalid framework target declaration")
			}
			candidate, err := source.loadTarget(ctx, uri, &normalized)
			if err != nil {
				return nil, err
			}
			if bytes.Equal(base, candidate) {
				if proven[key] == nil {
					proven[key] = map[string]bool{}
				}
				proven[key][normalized.ProfileKey()] = true
			}
		}
	}
	return func(ctx context.Context, pin identity.ResolvedResource, target types.WindowTarget) bool {
		if ctx == nil || ctx.Err() != nil {
			return false
		}
		return proven[pin.URI+"|"+pin.ContentFingerprint][target.ProfileKey()]
	}, nil
}
func (s *embeddedWindowSource) Candidates(ctx context.Context, uri identity.ResourceURI) ([]identity.ResourceCandidate, error) {
	raw, err := s.load(ctx, uri)
	if err != nil {
		return nil, err
	}
	return []identity.ResourceCandidate{{Kind: identity.WorkingCandidate, ContentFingerprint: identity.ContentFingerprint(raw)}}, nil
}
func (s *embeddedWindowSource) ReadCandidate(ctx context.Context, uri identity.ResourceURI, candidate identity.ResourceCandidate) (json.RawMessage, error) {
	if candidate.Kind != identity.WorkingCandidate || !candidate.Valid() {
		return nil, identity.ErrResourceDenied
	}
	return s.load(ctx, uri)
}

func configuredWindowResourceResolver(config *hostAuthorizationFile, root string, prepared *executor.PreparedAuthorization, enrich windowloader.WorkspaceWindowEnricher, overrides ...func(context.Context, identity.VerifiedActor) (identity.ResourceSource, error)) (func(context.Context, string) (*identity.ResourceResolver, identity.ResourceRef, error), error) {
	workspace, err := windowloader.NewWorkspaceResourceSource(root, config.WindowResources, enrich)
	if err != nil {
		return nil, err
	}
	embedded, err := NewFrameworkWindowBundleSource(config.FrameworkWindowResources)
	if err != nil {
		return nil, err
	}
	type entry struct {
		resolver *identity.ResourceResolver
		ref      identity.ResourceRef
	}
	entries := map[string]entry{}
	for _, group := range []struct {
		source   identity.ResourceSource
		bindings []windowloader.ResourceBinding
	}{{workspace, config.WindowResources}, {embedded, config.FrameworkWindowResources}} {
		resolver, err := prepared.ResourceResolver(policy.OperationWindowView, group.source, nil)
		if err != nil {
			return nil, err
		}
		for _, binding := range group.bindings {
			keys := map[string]bool{binding.WindowKey: true, binding.URI: true}
			if binding.CatalogID != "" {
				keys[binding.CatalogID] = true
			}
			for key := range keys {
				if _, exists := entries[key]; exists {
					return nil, fmt.Errorf("ambiguous configured window resource")
				}
				entries[key] = entry{resolver: resolver, ref: identity.ResourceRef{URI: binding.URI}}
			}
		}
	}
	return func(ctx context.Context, key string) (*identity.ResourceResolver, identity.ResourceRef, error) {
		value, exists := entries[key]
		if !exists {
			return nil, identity.ResourceRef{}, identity.ErrResourceDenied
		}
		if len(overrides) > 0 && overrides[0] != nil {
			actor, err := prepared.ResourceActor(ctx)
			if err != nil {
				return nil, identity.ResourceRef{}, err
			}
			source, err := overrides[0](ctx, actor)
			if err != nil {
				return nil, identity.ResourceRef{}, err
			}
			resolver, err := prepared.ResourceResolver(policy.OperationWindowView, source, nil)
			return resolver, value.ref, err
		}
		return value.resolver, value.ref, nil
	}, nil
}
