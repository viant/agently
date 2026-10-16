package agently

import (
	"context"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"github.com/viant/agently-core/app/executor"
	execconfig "github.com/viant/agently-core/app/executor/config"
	appserver "github.com/viant/agently-core/app/server"
	"github.com/viant/agently-core/sdk"
	svcauth "github.com/viant/agently-core/service/auth"
	"github.com/viant/agently-core/service/policy"
	resources "github.com/viant/agently-core/service/resource"
	"github.com/viant/authz"
	"github.com/viant/authz/gating"
	scyauth "github.com/viant/scy/auth"
	"os"
	"path/filepath"
	"testing"
)

func TestConfiguredHostRegistersPrivateAuthoredSourcesOnOneGateway(t *testing.T) {
	for _, k := range []string{"AGENTLY_DB_DRIVER", "AGENTLY_DB_DSN", "AGENTLY_DB_PATH", "AGENTLY_DB_SECRETS"} {
		t.Setenv(k, "")
	}
	root, config, _, principal := toolfeedHostFixture(t)
	ctx := svcauth.InjectTokens(context.Background(), &scyauth.Token{IDToken: "trusted-fixture"})
	for _, dir := range []string{"templates/bundles", "intents"} {
		require.NoError(t, os.MkdirAll(filepath.Join(root, dir), 0700))
	}
	templateBytes := []byte("id: original-template\nname: Original Template\ninstructions: Original template body\n")
	profileBytes := []byte("id: original-profile\nmodel: OriginalModel\ntemplate: original-template\nmessages: [{role: system, text: Original profile body}]\n")
	require.NoError(t, os.WriteFile(filepath.Join(root, "templates/output.yaml"), templateBytes, 0600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "templates/bundles/output.yaml"), []byte("id: original-output-bundle\ntemplates: [original-template]\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "intents/run.yaml"), profileBytes, 0600))
	config.TemplateSources = []HostAuthoredSource{{Namespace: "team", Directory: "templates"}}
	config.IntentSources = []HostAuthoredSource{{Namespace: "team", Directory: "intents"}}
	for _, kind := range []string{"template", "intent"} {
		name := "output"
		if kind == "intent" {
			name = "run"
		}
		uri := kind + "://team/" + name
		family := authz.ResourceFamily{Kind: kind, ID: uri, Tenant: "tenant"}
		resource := authz.Resource{Kind: kind, ID: uri, Tenant: "tenant", Version: "working"}
		config.Policies = append(config.Policies, authz.Document{Resource: resource, Revision: 1, Policies: map[string]authz.Policy{"read": {Mode: "protected", Rule: &authz.Rule{Kind: "role", Value: "reader"}}}})
		config.Requirements = append(config.Requirements, gating.Binding{Resource: resource, Action: "read", Document: gating.RequirementsDocument{Revision: "one", Requirements: gating.Requirements{SchemaVersion: 1}}})
		config.ResourceSelections = append(config.ResourceSelections, authz.SelectionDocument{Resource: family, Revision: 1, DefaultVersion: "working"})
		for _, op := range []string{"resource.list", "resource.get"} {
			config.ResourceRevisionBindings = append(config.ResourceRevisionBindings, policy.ResourceRevisionBinding{Operation: op, URI: uri, Resource: family, Action: "read"})
		}
	}
	config.AccountProjection = "opaque"
	config.SchemaVersion = 1
	config.ProviderRef = "identity"
	config.CapabilityMappingRef = "mapping"
	config.PolicyVersion = "one"
	bytes, err := json.Marshal(config)
	require.NoError(t, err)
	configPath := filepath.Join(root, "authorization.json")
	require.NoError(t, os.WriteFile(configPath, bytes, 0600))
	settings := authorizationWorkspaceConfig("authz", "identity", "mapping")
	raw, err := json.Marshal(settings.Raw)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, "config.yaml"), raw, 0600))
	options := ServeOptions{AuthzConfigPath: configPath, InternalResourceProviderIdentity: "host-resources", AuthorizationProviderFactory: func(context.Context, json.RawMessage, json.RawMessage) (AuthorizationProviders, error) {
		return AuthorizationProviders{Identity: principal, AllowsTenant: func(v string) bool { return v == "tenant" }}, nil
	}}
	registration := &internalHostRegistration{}
	configure, err := configureHostAuthorization(options, root, settings, nil, registration)
	require.NoError(t, err)
	rt, backend, _, err := appserver.BuildWorkspaceRuntime(ctx, appserver.RuntimeOptions{WorkspaceRoot: root, Defaults: &execconfig.Defaults{}, SkipRegistryInitialize: true, ConfigureBuilder: configure})
	require.NoError(t, err)
	defer rt.Close(ctx)
	require.NoError(t, registration.Bind(ctx, rt))
	require.NotNil(t, rt.PrimitiveProviders)
	items, listErr := resources.NewTemplateReader(rt.PrimitiveProviders).LoadAll(ctx)
	require.NoError(t, listErr)
	require.Len(t, items, 1)
	selected, loadErr := resources.NewTemplateReader(rt.PrimitiveProviders).Load(ctx, "template://team/output")
	require.NoError(t, loadErr)
	require.Equal(t, "Original template body", selected.Instructions)
	got, err := backend.(sdk.Client).GetTemplate(ctx, &sdk.GetTemplateInput{Name: "Original Template"})
	require.NoError(t, err)
	require.Equal(t, "Original template body", got.Instructions)
	nativePrompt, err := rt.Registry.Execute(ctx, "prompt:get", map[string]interface{}{"id": "original-profile"})
	require.NoError(t, err)
	require.Contains(t, nativePrompt, "Original profile body")
	prof, err := resources.NewIntentReader(rt.PrimitiveProviders).Load(ctx, "original-profile")
	require.NoError(t, err)
	require.Equal(t, "OriginalModel", prof.Model)
	require.Equal(t, "Original profile body", prof.Messages[0].Text)
	require.Equal(t, "intent://team/run", prof.ResourceURI)
	parent, _ := json.Marshal(prof)
	require.NotContains(t, string(parent), "ResourceProviderIdentity")
	principal.principal.Facts.Roles = nil
	_, err = rt.MCPManager.Reconnect(ctx, "", "internal")
	require.NoError(t, err)
	got, err = backend.(sdk.Client).GetTemplate(ctx, &sdk.GetTemplateInput{Name: "Original Template"})
	require.Error(t, err)
	require.Nil(t, got)
	prof, err = resources.NewIntentReader(rt.PrimitiveProviders).Load(ctx, "original-profile")
	require.Error(t, err)
	require.Nil(t, prof)
	actual, err := os.ReadFile(filepath.Join(root, "templates/output.yaml"))
	require.NoError(t, err)
	require.Equal(t, templateBytes, actual)
	actual, err = os.ReadFile(filepath.Join(root, "intents/run.yaml"))
	require.NoError(t, err)
	require.Equal(t, profileBytes, actual)
	_ = executor.Runtime{}
}
