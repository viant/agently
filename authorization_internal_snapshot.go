package agently

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	workspaceconfig "github.com/viant/agently-core/workspace/config"
	"io"
	"io/fs"
	"sort"
	"strings"
	"sync"

	"github.com/viant/agently-core/app/host"
	"github.com/viant/agently-core/protocol/primitive"
	identity "github.com/viant/agently-core/protocol/resource"
	resources "github.com/viant/agently-core/service/resource"
	"github.com/viant/forge/backend/reporting/registry"
	"github.com/viant/forge/backend/types"
)

// internalWindowSnapshot contains authored content only. Caller identity and
// revision policy are checked independently on every operation.
type internalWindowSnapshot struct {
	layoutSource   *resources.CapturedLayoutSource
	menuURIs       map[string]bool
	initialize     func(context.Context) error
	initOnce       sync.Once
	initErr        error
	workspace      *resources.NativeAssetSnapshot
	framework      map[string]json.RawMessage
	frameworkIndex []primitive.ResourceState
}

func newInternalWindowSnapshot(ctx context.Context, root, reportingRoot string, config *hostAuthorizationFile, deferred ...bool) (*internalWindowSnapshot, error) {
	if len(deferred) > 0 && deferred[0] {
		if len(config.WindowResources) > 0 {
			if _, err := resources.SnapshotWindowDefinitions(ctx, root, config.WindowResources, nil); err != nil {
				return nil, err
			}
		}
		if _, err := newEmbeddedWindowSource(config.FrameworkWindowResources); err != nil {
			return nil, err
		}
		declaredMenus, err := configuredNativeMenuDefinitions(root, config.MenuSources)
		if err != nil {
			return nil, err
		}
		result := &internalWindowSnapshot{menuURIs: map[string]bool{}}
		for _, definition := range declaredMenus {
			result.menuURIs[definition.URI] = true
		}

		result.initialize = func(ctx context.Context) error {
			loaded, err := newInternalWindowSnapshot(ctx, root, reportingRoot, config)
			if err != nil {
				return err
			}
			result.workspace = loaded.workspace
			result.framework = loaded.framework
			result.frameworkIndex = loaded.frameworkIndex
			result.menuURIs = loaded.menuURIs
			result.layoutSource = loaded.layoutSource
			return nil
		}
		return result, nil
	}
	menuDefinitions, err := configuredNativeMenuDefinitions(root, config.MenuSources)
	if err != nil {
		return nil, err
	}
	result := &internalWindowSnapshot{framework: map[string]json.RawMessage{}, menuURIs: map[string]bool{}}
	for _, definition := range menuDefinitions {
		result.menuURIs[definition.URI] = true
	}
	if len(menuDefinitions) > 0 || len(config.WindowResources) > 0 || (root != "" && (len(config.FrameworkWindowResources) > 0 || len(config.ToolfeedSources)+len(config.TemplateSources)+len(config.IntentSources) > 0)) {
		// This registry belongs to one serialized snapshot compilation. It is
		// refreshed through the same confined reader before authored enrichment.
		var current *registry.Registry
		enrich := workspaceReportingRegistryEnricher(func() *registry.Registry { return current })
		definitions := append([]resources.NativeSnapshotDefinition(nil), menuDefinitions...)
		var err error
		if len(config.WindowResources) > 0 {
			windowDefinitions, loadErr := resources.SnapshotWindowDefinitions(ctx, root, config.WindowResources, resources.StaticWorkspaceWindowEnricher(enrich))
			err = loadErr
			definitions = append(definitions, windowDefinitions...)
		}
		if err != nil {
			return nil, err
		}
		linked, err := newEmbeddedWindowSource(config.FrameworkWindowResources)
		if err != nil {
			return nil, err
		}
		for _, binding := range config.FrameworkWindowResources {
			binding := binding
			uri, _ := identity.ParseResourceURI(binding.URI)
			base := serializedWindowBase(func(ctx context.Context, target *types.WindowTarget) (json.RawMessage, error) {
				return linked.loadTarget(ctx, uri, target)
			})
			// The directory is the real workspace source anchor; linked bytes
			// are process-owned while dependency imports use this same reader.
			definitions = append(definitions, resources.NativeSnapshotDefinition{URI: binding.URI, Title: binding.WindowKey, FormatVersion: 2, File: "extension/forge", Load: resources.NativeComposedWindowLoader(binding, base, resources.StaticWorkspaceWindowEnricher(enrich))})
		}
		var feedDirectories []resources.ExtensionDirectory
		for _, source := range config.ToolfeedSources {
			feedDirectories = append(feedDirectories, resources.ToolfeedDirectory(source.Namespace, source.Directory))

		}
		authoredDirs, compilers, e := host.AuthoredDirectories(config.TemplateSources, config.IntentSources)
		if e != nil {
			return nil, e
		}
		feedDirectories = append(feedDirectories, authoredDirs...)
		defer func() {
			for _, c := range compilers {
				c.Release()
			}
		}()
		if len(feedDirectories) > 0 {
			feeds, e := resources.SnapshotExtensionDefinitions(ctx, root, feedDirectories)
			if e != nil {
				return nil, e
			}
			definitions = append(definitions, feeds...)
		}
		result.workspace, err = resources.NewNativeAssetSnapshot(ctx, root, definitions, resources.NativeSnapshotOptions{BeforeCompile: func(ctx context.Context, reader *resources.ExtensionReader) error {
			for _, c := range compilers {
				if err := c.Prepare(ctx, reader); err != nil {
					return err
				}
			}
			if len(config.MenuSources) > 0 {
				layoutFile := "ui/layout.yaml"
				workspaceConfig, loadErr := workspaceconfig.Load(root)
				if loadErr != nil {
					return loadErr
				}
				if workspaceConfig != nil {
					if ui, ok := workspaceConfig.Raw["ui"].(map[string]any); ok {
						if layout, ok := ui["layout"].(map[string]any); ok {
							if ref, ok := layout["ref"].(string); ok && strings.TrimSpace(ref) != "" {
								layoutFile = ref
							}
						}
					}
				}
				layoutRaw, loadErr := reader.ReadFile(ctx, layoutFile)
				if loadErr != nil && !errors.Is(loadErr, fs.ErrNotExist) {
					return loadErr
				}
				if loadErr == nil {
					checkedURI, _ := identity.ParseResourceURI(definitions[0].URI)
					result.layoutSource = &resources.CapturedLayoutSource{Root: root, Source: layoutFile, Raw: append([]byte(nil), layoutRaw...), Check: func(ctx context.Context) error { _, err := result.workspace.Candidates(ctx, checkedURI); return err }}
				}
			}

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
		if result.workspace != nil {
			continue
		}
		uri, _ := identity.ParseResourceURI(binding.URI)
		raw, err := linked.load(ctx, uri)
		if err != nil {
			return nil, fmt.Errorf("framework snapshot unavailable: %s: %w", binding.URI, err)
		}
		result.framework[binding.URI] = append(json.RawMessage(nil), raw...)
		title := binding.WindowKey
		result.frameworkIndex = append(result.frameworkIndex, primitive.ResourceState{URI: binding.URI, Kind: "window", Namespace: uri.Namespace, Name: uri.Name, Title: title, Lifecycle: identity.WorkingCandidate, Revision: identity.WorkingCandidate, FormatVersion: 2, ContentFingerprint: identity.ContentFingerprint(raw)})
	}
	return result, nil
}

// Initialize captures the original source generation after runtime construction
// has finished preparing its filesystem, before any request can be served.
func (s *internalWindowSnapshot) Initialize(ctx context.Context) error {
	if s == nil {
		return identity.ErrResourceDenied
	}
	if s.initialize == nil {
		return nil
	}
	s.initOnce.Do(func() { s.initErr = s.initialize(ctx) })
	return s.initErr
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

// CheckCandidate verifies original authored content after the authority phase ends.
// Framework bytes are fixed in the linked bundle; workspace metadata remains
// sticky-invalid after any source/dependency change until restart.
func (s *internalWindowSnapshot) CheckCandidate(ctx context.Context, uri identity.ResourceURI, candidate identity.ResourceCandidate) error {
	if s == nil || ctx == nil || ctx.Err() != nil || !uri.Valid() || !candidate.Valid() {
		return identity.ErrResourceDenied
	}
	if raw, ok := s.framework[uri.String()]; ok {
		if candidate.Kind != identity.WorkingCandidate || candidate.ContentFingerprint != identity.ContentFingerprint(raw) {
			return identity.ErrResourceStale
		}
		return nil
	}
	if s.workspace == nil {
		return identity.ErrResourceDenied
	}
	return s.workspace.CheckCandidate(ctx, uri, candidate)
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

// serializedWindowBase is the trusted linked-byte boundary shared by composed hosts.
func serializedWindowBase(load func(context.Context, *types.WindowTarget) (json.RawMessage, error)) func(context.Context, *types.WindowTarget) (*types.Window, error) {
	return func(ctx context.Context, target *types.WindowTarget) (*types.Window, error) {
		raw, err := load(ctx, target)
		if err != nil {
			return nil, err
		}
		return decodeTrustedWindow(raw)
	}
}
func decodeTrustedWindow(raw json.RawMessage) (*types.Window, error) {
	var window types.Window
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&window); err != nil {
		return nil, err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, fmt.Errorf("linked window must contain one complete JSON value")
	}
	return &window, nil
}
