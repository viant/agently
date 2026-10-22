package agently

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/viant/agently-core/genai/llm"
	mcpcfg "github.com/viant/agently-core/protocol/mcp/config"
	"github.com/viant/agently-core/protocol/mcp/manager"
	"github.com/viant/agently-core/protocol/mcpname"
	"github.com/viant/agently-core/protocol/tool"
	toolbundle "github.com/viant/agently-core/protocol/tool/bundle"
	bridge "github.com/viant/agently-core/service/primitiveprovider"
	forgewindowrepo "github.com/viant/agently-core/workspace/repository/forgewindow"
	fsstore "github.com/viant/agently-core/workspace/store/fs"
)

// A missing internal registration must not silently become a remote definition
// lookup. This is the same strict boundary used by the authored MCP reader.
type startupUIDefinitionReader struct {
	mu    sync.Mutex
	loads []string
}

func (*startupUIDefinitionReader) List(context.Context) ([]string, error) { return nil, nil }
func (r *startupUIDefinitionReader) Load(_ context.Context, name string) (*mcpcfg.MCPClient, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.loads = append(r.loads, name)
	return nil, errors.New("MCP definition not found")
}
func (r *startupUIDefinitionReader) reset() { r.mu.Lock(); defer r.mu.Unlock(); r.loads = nil }
func (r *startupUIDefinitionReader) loaded() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.loads...)
}

func TestHostStartupRegistersMetadataAlongsideWindowServices(t *testing.T) {
	ctx := context.Background()
	mgr, err := manager.New(nil)
	require.NoError(t, err)
	definitions := &startupUIDefinitionReader{}
	_, err = mgr.BindDefinitionReader(definitions)
	require.NoError(t, err)
	registry, err := tool.NewDefaultRegistry(mgr)
	require.NoError(t, err)
	uiBridge := bridge.NewService(&bridge.Config{})
	repository := forgewindowrepo.NewWithStore(fsstore.New(t.TempDir()))
	// Exercise exactly the helper invoked by Serve, without separately adding the
	// metadata service in this test. Omitting it in host startup must fail here.
	registerHostUIWindowServices(registry, repository, uiBridge, nil)
	definitions.reset()
	matcher, ok := registry.(tool.ContextMatcherWithError)
	require.True(t, ok)
	rules := &toolbundle.Bundle{ID: "window-workspace", Match: []llm.Tool{
		{Name: "ui/view:list"}, {Name: "ui/view:get"}, {Name: "ui/view:open"},
		{Name: "ui/metadata:list"}, {Name: "ui/metadata:get"},
		{Name: "ui/window:list"}, {Name: "ui/control:list"},
	}}
	var matchErrors []error
	resolved := toolbundle.ResolveDefinitionsWithOptions(rules, func(pattern string) []*llm.ToolDefinition {
		matches, err := matcher.MatchDefinitionWithContextResult(ctx, pattern)
		if err != nil {
			matchErrors = append(matchErrors, err)
		}
		return matches
	})
	require.Empty(t, matchErrors, "host UI rules must not fall through to remote MCP definitions")
	names := map[string]bool{}
	for _, definition := range resolved.Definitions {
		names[mcpname.Canonical(definition.Name)] = true
	}
	for _, name := range []string{"ui/metadata:list", "ui/metadata:get", "ui/view:open"} {
		require.True(t, names[mcpname.Canonical(name)], "missing startup tool %s", name)
	}
	for _, name := range definitions.loaded() {
		require.NotEqual(t, "ui/metadata", name, "metadata is a registered host service, not a configured remote")
	}
	require.Empty(t, definitions.loaded(), "all bundle members belong to this host registration cohort")

	// Registration does not grant metadata read authority. An unconfigured
	// authorized catalog still denies, and the normal host guard remains active.
	_, err = registry.Execute(ctx, "ui/metadata:get", map[string]interface{}{"id": "neutral-record"})
	require.ErrorContains(t, err, "authorized window metadata catalog is not configured")
	require.True(t, tool.SetAuthorizationGuard(registry, func(context.Context, string, map[string]interface{}) error { return errors.New("host action denied") }))
	_, err = registry.Execute(ctx, "ui/metadata:list", map[string]interface{}{})
	require.ErrorContains(t, err, "host action denied")
}
