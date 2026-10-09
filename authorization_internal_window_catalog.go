package agently

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	identity "github.com/viant/agently-core/protocol/resource"
	primitiveprovider "github.com/viant/agently-core/service/primitiveprovider"
	resources "github.com/viant/agently-core/service/resource"
	windowloader "github.com/viant/agently-core/service/ui/window"
	"github.com/viant/forge/backend/types"
)

// internalWindowCatalog preserves the host's configured window names while
// routing every read to the canonical primitive Gateway. The alias table is
// fixed at construction; a provider cannot add aliases by advertising them.
type internalWindowCatalog struct {
	remote  *resources.WindowCatalog
	byAlias map[string]internalWindowBinding
	byURI   map[string]internalWindowBinding
}

type internalWindowBinding struct {
	uri   string
	alias string
	key   string
}

func newInternalWindowCatalog(remote *resources.WindowCatalog, declared []windowloader.ResourceBinding) (*internalWindowCatalog, error) {
	if remote == nil || len(declared) == 0 {
		return nil, fmt.Errorf("internal window catalog requires a remote catalog and explicit bindings")
	}
	catalog := &internalWindowCatalog{
		remote:  remote,
		byAlias: make(map[string]internalWindowBinding, len(declared)*3),
		byURI:   make(map[string]internalWindowBinding, len(declared)),
	}
	for _, entry := range declared {
		parsed, err := identity.ParseResourceURI(entry.URI)
		if err != nil || parsed.Kind != "window" || entry.WindowKey == "" || strings.TrimSpace(entry.WindowKey) != entry.WindowKey {
			return nil, fmt.Errorf("invalid internal window binding")
		}
		if _, duplicate := catalog.byURI[entry.URI]; duplicate {
			return nil, fmt.Errorf("duplicate internal window URI binding")
		}
		alias := entry.CatalogID
		if alias == "" {
			alias = entry.WindowKey
		}
		binding := internalWindowBinding{uri: entry.URI, alias: alias, key: entry.WindowKey}
		if err := catalog.addAlias(alias, binding); err != nil {
			return nil, err
		}
		if entry.WindowKey != alias {
			if err := catalog.addAlias(entry.WindowKey, binding); err != nil {
				return nil, err
			}
		}
		if err := catalog.addAlias(entry.URI, binding); err != nil {
			return nil, err
		}
		catalog.byURI[entry.URI] = binding
	}
	return catalog, nil
}

func (c *internalWindowCatalog) addAlias(alias string, binding internalWindowBinding) error {
	if alias == "" || strings.TrimSpace(alias) != alias {
		return fmt.Errorf("invalid internal window alias")
	}
	if previous, exists := c.byAlias[alias]; exists && previous.uri != binding.uri {
		return fmt.Errorf("ambiguous internal window alias")
	}
	c.byAlias[alias] = binding
	return nil
}

func (c *internalWindowCatalog) binding(key string) (internalWindowBinding, error) {
	if c == nil || key == "" || strings.TrimSpace(key) != key {
		return internalWindowBinding{}, identity.ErrResourceDenied
	}
	if binding, ok := c.byAlias[key]; ok {
		return binding, nil
	}
	return internalWindowBinding{}, identity.ErrResourceDenied
}

func (c *internalWindowCatalog) AuthzReady() bool {
	return c != nil && c.remote != nil && c.remote.AuthzReady() && len(c.byURI) > 0
}

func (c *internalWindowCatalog) UsesResourceResolution() bool { return true }
func (c *internalWindowCatalog) MetadataOnlyWindowList() bool { return true }

func (c *internalWindowCatalog) ResourceReference(_ context.Context, key string) (identity.ResourceRef, error) {
	binding, err := c.binding(key)
	if err != nil {
		return identity.ResourceRef{}, err
	}
	return identity.ResourceRef{URI: binding.uri}, nil
}

func (c *internalWindowCatalog) ConfiguredWindowIDs() []string {
	result := make([]string, 0, len(c.byAlias))
	for key := range c.byAlias {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

func (c *internalWindowCatalog) List(ctx context.Context, input *primitiveprovider.WindowDefinitionListInput) (*primitiveprovider.WindowDefinitionListOutput, error) {
	if !c.AuthzReady() {
		return nil, identity.ErrResourceDenied
	}
	if input == nil {
		input = &primitiveprovider.WindowDefinitionListInput{}
	}
	if input.Offset < 0 || input.Limit < 0 || input.Limit > 100 {
		return nil, identity.ErrResource
	}

	// Read only the delegated static index. Paging is done before applying the
	// host's allowlist so provider pagination cannot hide a later bound window.
	indexed := make(map[string]primitiveprovider.WindowDefinitionSummary, len(c.byURI))
	duplicates := make(map[string]bool)
	const pageSize = 100
	const maxPages = 100
	offset := 0
	for page := 0; page < maxPages; page++ {
		listed, err := c.remote.List(ctx, &primitiveprovider.WindowDefinitionListInput{Limit: pageSize, Offset: offset})
		if err != nil || listed == nil {
			if err == nil {
				err = identity.ErrResourceDenied
			}
			return nil, err
		}
		for _, row := range listed.Windows {
			if row.ResourceURI == "" {
				continue
			}
			if _, exists := indexed[row.ResourceURI]; exists {
				duplicates[row.ResourceURI] = true
				continue
			}
			indexed[row.ResourceURI] = row
		}
		offset += len(listed.Windows)
		if !listed.HasMore {
			break
		}
		if len(listed.Windows) == 0 || page == maxPages-1 {
			return nil, identity.ErrResourceDenied
		}
	}

	rows := make([]primitiveprovider.WindowDefinitionSummary, 0, len(c.byURI))
	for uri, binding := range c.byURI {
		row, exists := indexed[uri]
		if !exists || duplicates[uri] {
			continue
		}
		if input.Query != "" && !strings.Contains(strings.ToLower(row.Title+" "+row.Name+" "+uri+" "+binding.alias), strings.ToLower(input.Query)) {
			continue
		}
		title := row.Title
		if title == "" {
			title = binding.alias
		}
		rows = append(rows, primitiveprovider.WindowDefinitionSummary{
			ProviderIdentity: row.ProviderIdentity,
			ResourceURI:      uri,
			Name:             row.Name,
			WindowID:         binding.alias,
			Title:            title,
			Namespace:        row.Namespace,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].WindowID == rows[j].WindowID {
			return rows[i].ResourceURI < rows[j].ResourceURI
		}
		return rows[i].WindowID < rows[j].WindowID
	})
	start := min(input.Offset, len(rows))
	limit := input.Limit
	if limit == 0 {
		limit = 100
	}
	end := min(start+limit, len(rows))
	return &primitiveprovider.WindowDefinitionListOutput{Windows: rows[start:end], HasMore: end < len(rows)}, nil
}

func (c *internalWindowCatalog) Get(ctx context.Context, input *primitiveprovider.WindowDefinitionGetInput) (*primitiveprovider.WindowDefinitionGetOutput, error) {
	return c.get(ctx, input)
}

// GetForOpen deliberately shares the exact pinned remote path. Opening has a
// separate action authorization in the caller; this adapter never resolves a
// new/current resource revision on its own.
func (c *internalWindowCatalog) GetForOpen(ctx context.Context, input *primitiveprovider.WindowDefinitionGetInput) (*primitiveprovider.WindowDefinitionGetOutput, error) {
	return c.get(ctx, input)
}

func (c *internalWindowCatalog) get(ctx context.Context, input *primitiveprovider.WindowDefinitionGetInput) (*primitiveprovider.WindowDefinitionGetOutput, error) {
	if !c.AuthzReady() || input == nil {
		return nil, identity.ErrResourceDenied
	}
	binding, err := c.binding(input.WindowID)
	if err != nil {
		return nil, err
	}
	if input.Resource != nil && input.Resource.URI != binding.uri {
		return nil, identity.ErrResourceDenied
	}
	if input.ResolvedResource != nil && input.ResolvedResource.URI != binding.uri {
		return nil, identity.ErrResourceDenied
	}
	ref := identity.ResourceRef{URI: binding.uri}
	if input.Resource != nil {
		ref = *input.Resource
	}
	var pin *identity.ResolvedResource
	if input.ResolvedResource != nil {
		copy := *input.ResolvedResource
		pin = &copy
	}
	var target *types.WindowTarget
	if input.Target != nil {
		copy := *input.Target
		if copy.DependencyPins != nil {
			pins := make(map[string]identity.ResolvedResource, len(copy.DependencyPins))
			for id, child := range copy.DependencyPins {
				pins[id] = child
			}
			copy.DependencyPins = pins
		}
		target = &copy
	}
	delegated, err := c.remote.Get(ctx, &primitiveprovider.WindowDefinitionGetInput{
		WindowID:         binding.uri,
		Resource:         &ref,
		ResolvedResource: pin,
		Target:           target,
	})
	if err != nil || delegated == nil || delegated.Definition == nil || delegated.Definition.Resource == nil {
		if err == nil {
			err = identity.ErrResourceDenied
		}
		return nil, err
	}
	resolved := delegated.Definition.Resource
	if resolved.URI != binding.uri || input.ResolvedResource != nil && (!sameInternalResource(*input.ResolvedResource, *resolved) || resolved.ValidUntil.After(input.ResolvedResource.ValidUntil)) {
		return nil, identity.ErrResourceDenied
	}
	if !resolved.ValidUntil.After(time.Now()) {
		return nil, identity.ErrResourceDenied
	}
	delegated.WindowID = binding.alias
	return delegated, nil
}

func sameInternalResource(expected, actual identity.ResolvedResource) bool {
	return expected.ProviderIdentity == actual.ProviderIdentity &&
		expected.URI == actual.URI &&
		expected.ResourceCandidate == actual.ResourceCandidate &&
		expected.AuthorityBinding == actual.AuthorityBinding
}

func (c *internalWindowCatalog) CheckWindowAdmission(ctx context.Context, key string) (bool, error) {
	if !c.AuthzReady() {
		return false, identity.ErrResourceDenied
	}
	binding, err := c.binding(key)
	if err != nil {
		return false, err
	}
	return c.remote.CheckWindowAdmission(ctx, binding.uri)
}

func (c *internalWindowCatalog) RevalidateResource(ctx context.Context, key string, pin identity.ResolvedResource) (*identity.ResolvedResource, error) {
	if !c.AuthzReady() {
		return nil, identity.ErrResourceDenied
	}
	binding, err := c.binding(key)
	if err != nil || pin.URI != binding.uri || !pin.ValidUntil.After(time.Now()) {
		return nil, identity.ErrResourceDenied
	}
	connection, err := c.remote.Gateway.ConnectionForProvider(ctx, pin.ProviderIdentity)
	if err != nil {
		return nil, identity.ErrResourceDenied
	}
	// Gateway.Get rechecks the exact supplied candidate and current authority;
	// it cannot select or substitute the provider's current working version.
	result, err := c.remote.Gateway.Get(ctx, connection, identity.ResourceRef{URI: binding.uri, Revision: pin.Selector()}, &pin)
	if err != nil || result == nil || result.ResolvedResource == nil || !sameInternalResource(pin, *result.ResolvedResource) || result.ResolvedResource.ValidUntil.After(pin.ValidUntil) {
		return nil, identity.ErrResourceDenied
	}
	return result.ResolvedResource, nil
}

func (c *internalWindowCatalog) VerifyWindowTarget(ctx context.Context, pin identity.ResolvedResource, target types.WindowTarget, variant string) error {
	if !c.AuthzReady() {
		return identity.ErrResourceDenied
	}
	if _, ok := c.byURI[pin.URI]; !ok {
		return identity.ErrResourceDenied
	}
	return c.remote.VerifyWindowTarget(ctx, pin, target, variant)
}

var _ primitiveprovider.WindowDefinitionCatalog = (*internalWindowCatalog)(nil)
