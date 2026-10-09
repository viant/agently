package agently

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/viant/agently-core/protocol/primitive"
	identity "github.com/viant/agently-core/protocol/resource"
	resources "github.com/viant/agently-core/service/resource"
	"github.com/viant/forge/backend/reporting/registry"
)

// internalWindowSnapshot contains authored content only. Caller identity and
// revision policy are checked independently on every operation.
type internalWindowSnapshot struct {
	workspace      *resources.NativeAssetSnapshot
	framework      map[string]json.RawMessage
	frameworkIndex []primitive.ResourceState
}

func newInternalWindowSnapshot(ctx context.Context, root, reportingRoot string, config *hostAuthorizationFile) (*internalWindowSnapshot, error) {
	result := &internalWindowSnapshot{framework: map[string]json.RawMessage{}}
	if len(config.WindowResources) > 0 {
		// This registry belongs to one serialized snapshot compilation. It is
		// refreshed through the same confined reader before authored enrichment.
		var current *registry.Registry
		enrich := workspaceReportingRegistryEnricher(func() *registry.Registry { return current })
		definitions, err := resources.SnapshotWindowDefinitions(ctx, root, config.WindowResources, resources.StaticWorkspaceWindowEnricher(enrich))
		if err != nil {
			return nil, err
		}
		result.workspace, err = resources.NewNativeAssetSnapshot(ctx, root, definitions, resources.NativeSnapshotOptions{BeforeCompile: func(ctx context.Context, reader *resources.ExtensionReader) error {
			options, check, err := resources.ConfinedReportingOptions(ctx, reader, registry.Options{WorkspaceRoot: root, ReportingRoot: reportingRoot})
			if err != nil {
				return err
			}
			current, err = registry.Discover(ctx, options)
			if err != nil {
				return err
			}
			return check()
		}})
		if err != nil {
			return nil, err
		}
	}
	linked, err := newEmbeddedWindowSource(config.FrameworkWindowResources)
	if err != nil {
		return nil, err
	}
	linked.bundle = true
	for _, binding := range config.FrameworkWindowResources {
		uri, _ := identity.ParseResourceURI(binding.URI)
		raw, err := linked.load(ctx, uri)
		if err != nil {
			return nil, fmt.Errorf("framework snapshot unavailable: %s: %w", binding.URI, err)
		}
		result.framework[binding.URI] = append(json.RawMessage(nil), raw...)
		title := binding.WindowKey
		result.frameworkIndex = append(result.frameworkIndex, primitive.ResourceState{URI: binding.URI, Kind: "window", Namespace: uri.Namespace, Name: uri.Name, Title: title, FormatVersion: 2, ContentFingerprint: identity.ContentFingerprint(raw)})
	}
	return result, nil
}

func (s *internalWindowSnapshot) Candidates(ctx context.Context, uri identity.ResourceURI) ([]identity.ResourceCandidate, error) {
	if s == nil || ctx == nil || ctx.Err() != nil {
		return nil, identity.ErrResourceDenied
	}
	if raw, ok := s.framework[uri.String()]; ok {
		return []identity.ResourceCandidate{{Kind: identity.WorkingCandidate, ContentFingerprint: identity.ContentFingerprint(raw)}}, nil
	}
	if s.workspace == nil {
		return nil, identity.ErrResourceDenied
	}
	return s.workspace.Candidates(ctx, uri)
}
func (s *internalWindowSnapshot) ReadCandidate(ctx context.Context, uri identity.ResourceURI, candidate identity.ResourceCandidate) (json.RawMessage, error) {
	if s == nil || ctx == nil || ctx.Err() != nil {
		return nil, identity.ErrResourceDenied
	}
	if raw, ok := s.framework[uri.String()]; ok {
		if !candidate.Valid() || candidate.Kind != identity.WorkingCandidate || candidate.ContentFingerprint != identity.ContentFingerprint(raw) {
			return nil, identity.ErrResourceStale
		}
		return append(json.RawMessage(nil), raw...), nil
	}
	if s.workspace == nil {
		return nil, identity.ErrResourceDenied
	}
	return s.workspace.ReadCandidate(ctx, uri, candidate)
}
func (s *internalWindowSnapshot) WindowIndex(ctx context.Context) ([]primitive.ResourceState, error) {
	if s == nil || ctx == nil || ctx.Err() != nil {
		return nil, identity.ErrResourceDenied
	}
	result := append([]primitive.ResourceState(nil), s.frameworkIndex...)
	if s.workspace != nil {
		rows, err := s.workspace.WindowIndex(ctx)
		if err != nil {
			return nil, err
		}
		result = append(result, rows...)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].URI < result[j].URI })
	return result, nil
}

// Compile-count is test evidence only, never authorization state.
func (s *internalWindowSnapshot) CompileCount() uint64 {
	if s == nil || s.workspace == nil {
		return 0
	}
	return s.workspace.CompileCount()
}

var _ identity.ResourceSource = (*internalWindowSnapshot)(nil)
