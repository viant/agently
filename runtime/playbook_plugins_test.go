package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/viant/agently-core/app/executor"
	cfg "github.com/viant/agently-core/protocol/mcp/config"
	"github.com/viant/agently-core/protocol/mcp/manager"
	identity "github.com/viant/agently-core/protocol/resource"
	promptsvc "github.com/viant/agently-core/protocol/tool/service/prompt"
	resources "github.com/viant/agently-core/service/resource"
	fsstore "github.com/viant/agently-core/workspace/store/fs"
	"github.com/viant/mcp"
	mcpclient "github.com/viant/mcp/client"
)

type playbookFixturePolicy struct{ actor identity.VerifiedActor }

func (p playbookFixturePolicy) SelectRevision(_ context.Context, _ identity.ResourceRef, candidates []identity.ResourceCandidate) (identity.ResourceDecision, error) {
	if len(candidates) != 1 {
		return identity.ResourceDecision{}, identity.ErrResourceDenied
	}
	return identity.ResourceDecision{Candidate: candidates[0], AuthorityBinding: "synthetic-local", ValidUntil: p.actor.ValidUntil}, nil
}

func TestPromptPluginListsAndReadsNativeMarkdownPlaybooks(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	for file, body := range map[string]string{
		"playbooks/review.md": "---\nid: review\nname: Review\n---\n---\nrole: system\n---\nUse supplied evidence.",
		"intents/legacy.yaml": "id: legacy\nname: Legacy\ninstructions: Keep legacy compatibility.\n",
	} {
		path := filepath.Join(root, file)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
		require.NoError(t, os.WriteFile(path, []byte(body), 0600))
	}
	actor, verify := resources.PublicReadActors()
	provider, err := resources.NewInternalProvider(ctx, root, []resources.ExtensionDirectory{resources.PlaybookDirectory("workspace", "playbooks"), resources.IntentDirectory("workspace", "intents")}, func(_ context.Context, a identity.VerifiedActor, _ identity.ResourceURI, _ string) (identity.ResourceRevisionPolicy, error) {
		return playbookFixturePolicy{a}, nil
	}, resources.LocalConfig{Validators: map[string]resources.LocalResourceValidator{"playbook": resources.ValidateIntentDefinition, "intent": resources.ValidateIntentDefinition}, ProviderIdentity: resources.InternalProviderIdentity, Actor: actor, Verify: verify, Authorize: func(context.Context, identity.VerifiedActor, identity.ResourceURI, string) error { return nil }})
	require.NoError(t, err)
	client, err := resources.NewLocalMCPClient(provider)
	require.NoError(t, err)
	mgr, err := manager.New(nil)
	require.NoError(t, err)
	require.NoError(t, mgr.RegisterLocal(ctx, "fixture", &cfg.MCPClient{ClientOptions: &mcp.ClientOptions{}, PrimitiveProviderIdentity: resources.InternalProviderIdentity}, func(context.Context) (mcpclient.Interface, error) { return client, nil }))
	gateway := resources.NewGateway(mgr, actor, verify, "")
	t.Cleanup(gateway.Close)
	rt := &executor.Runtime{PrimitiveProviders: gateway, Store: fsstore.New(root)}
	service := internalServiceFactory(rt, root, "prompt")
	list, err := service.Method("list")
	require.NoError(t, err)
	output := &promptsvc.ListOutput{}
	require.NoError(t, list(ctx, &promptsvc.ListInput{}, output))
	require.Len(t, output.Profiles, 2)
	get, err := service.Method("get")
	require.NoError(t, err)
	for _, id := range []string{"review", "legacy"} {
		require.NoError(t, get(ctx, &promptsvc.GetInput{ID: id}, &promptsvc.GetOutput{}))
	}
}
