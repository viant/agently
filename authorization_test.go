package agently

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/viant/agently-core/app/executor"
	"github.com/viant/agently-core/service/policy"
	wscfg "github.com/viant/agently-core/workspace/config"
	"github.com/viant/authz"
	"github.com/viant/authz/gating"
	"github.com/viant/authz/oauth"
)

func TestHostAuthorizationConfigRejectsUnknownAuthorityAndTrailingJSON(t *testing.T) {
	for name, body := range map[string]string{
		"browser identity": `{"schemaVersion":1,"catalogPath":"catalog.yaml","subject":"browser"}`,
		"unknown storage":  `{"schemaVersion":1,"catalogPath":"catalog.yaml","storage":{"driver":"sqlite"}}`,
		"trailing config":  `{"schemaVersion":1,"catalogPath":"catalog.yaml"} {}`,
		"version":          `{"schemaVersion":2,"catalogPath":"catalog.yaml"}`,
		"projection":       `{"schemaVersion":1,"catalogPath":"catalog.yaml","accountProjection":"implicit"}`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "authz.json")
			if err := os.WriteFile(path, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := readHostAuthorization(path); err == nil {
				t.Fatal("untrusted or unsupported authorization config accepted")
			}
		})
	}
}

func TestHostAuthorizationConfigBindsCatalogToConfigDirectory(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "authz.json")
	if err := os.WriteFile(path, []byte(`{"schemaVersion":1,"catalogPath":"catalog.yaml","accountProjection":"opaque"}`), 0600); err != nil {
		t.Fatal(err)
	}
	config, err := readHostAuthorization(path)
	if err != nil {
		t.Fatal(err)
	}
	if config.CatalogPath != filepath.Join(root, "catalog.yaml") {
		t.Fatalf("catalogPath=%s", config.CatalogPath)
	}
}

func TestHostAuthorizationForwardsTrustedEmbeddingBuilderAndRejectsMissingFile(t *testing.T) {
	t.Setenv("AGENTLY_AUTHZ_CONFIG", "")
	marker := errors.New("trusted host callback")
	called := false
	configure, err := configureHostAuthorization(ServeOptions{ConfigureBuilder: func(_ context.Context, builder *executor.Builder) error { called = builder != nil; return marker }}, t.TempDir(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = configure(context.Background(), executor.NewBuilder()); !errors.Is(err, marker) || !called {
		t.Fatalf("called=%v err=%v", called, err)
	}
	_, err = configureHostAuthorization(ServeOptions{AuthzConfigPath: filepath.Join(t.TempDir(), "absent.json")}, t.TempDir(), nil, nil)
	if err == nil {
		t.Fatal("missing explicit authorization config silently disabled authorization")
	}
}

func TestHostAuthorizationBuildsInjectedProvidersAndProtectedCatalog(t *testing.T) {
	var err error
	root := t.TempDir()
	catalog := filepath.Join(root, "catalog.yaml")
	if err = os.WriteFile(catalog, []byte("baseURL: .\nwindows: []\n"), 0600); err != nil {
		t.Fatal(err)
	}
	config := hostAuthorizationFile{SchemaVersion: 1, ProviderRef: "verified", CapabilityMappingRef: "mapped", PolicyVersion: "revision1", CatalogPath: "catalog.yaml"}
	config.Identity = json.RawMessage(`{"binding":"host-owned"}`)
	factory := func(_ context.Context, identity, evaluation json.RawMessage) (AuthorizationProviders, error) {
		if string(identity) != string(config.Identity) || len(evaluation) != 0 {
			t.Fatal("provider settings changed")
		}
		return AuthorizationProviders{Identity: fixtureHostIdentity{}, AllowsTenant: func(string) bool { return true }}, nil
	}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "authz.json")
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	called := false
	workspaceConfig := authorizationWorkspaceConfig("authz", "verified", "mapped")
	settingsJSON, err := json.Marshal(workspaceConfig.Raw)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "config.yaml"), settingsJSON, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := configureHostAuthorization(ServeOptions{AuthzConfigPath: path}, root, workspaceConfig, nil); err == nil {
		t.Fatal("provider-specific settings activated without injected binding")
	}
	configure, err := configureHostAuthorization(ServeOptions{AuthzConfigPath: path, AuthorizationProviderFactory: factory, ConfigureBuilder: func(context.Context, *executor.Builder) error { called = true; return nil }}, root, workspaceConfig, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = configure(context.Background(), executor.NewBuilder()); err != nil || !called {
		t.Fatalf("host registration called=%v err=%v", called, err)
	}
	if err = ValidateHostAuthorizationConfiguration(ServeOptions{WorkspacePath: root, AuthzConfigPath: path, AuthorizationProviderFactory: factory}); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(root, "agently.db")); !os.IsNotExist(err) {
		t.Fatal("authorization validation created an execution store")
	}
}

func TestHostAuthorizationPreflightRequiresExplicitInputs(t *testing.T) {
	t.Setenv("AGENTLY_AUTHZ_CONFIG", "")
	for _, options := range []ServeOptions{{}, {WorkspacePath: t.TempDir()}, {AuthzConfigPath: "unused"}} {
		if err := ValidateHostAuthorizationConfiguration(options); err == nil {
			t.Fatal("implicit authorization validation accepted")
		}
	}
}

func authorizationWorkspaceConfig(mode, provider, mapping string) *wscfg.Root {
	return &wscfg.Root{Raw: map[string]interface{}{"ui": map[string]interface{}{"authorization": map[string]interface{}{"mode": mode, "providerRef": provider, "capabilityMappingRef": mapping}}, "policy": map[string]interface{}{"authorization": map[string]interface{}{"mode": mode, "providerRef": provider}}}}
}

func TestHostAuthorizationConfigCannotSelectLegacyOrUnregisteredWorkspaceMode(t *testing.T) {
	config := &hostAuthorizationFile{ProviderRef: "verified", CapabilityMappingRef: "mapped"}
	for _, workspaceConfig := range []*wscfg.Root{nil, authorizationWorkspaceConfig("", "verified", "mapped"), authorizationWorkspaceConfig("legacy", "verified", "mapped"), authorizationWorkspaceConfig("authz", "other", "mapped"), authorizationWorkspaceConfig("authz", "verified", "other")} {
		if err := validateHostAuthorizationSelection(config, workspaceConfig); err == nil {
			t.Fatal("explicit host authorization silently selected legacy or unmatched enforcement")
		}
	}
	if err := validateHostAuthorizationSelection(config, authorizationWorkspaceConfig("authz", "verified", "mapped")); err != nil {
		t.Fatal(err)
	}
}

func TestHostAuthorizationMappingsRequireACLAndRequirements(t *testing.T) {
	resource := authz.Resource{Kind: "window", ID: "records", Version: "1", Tenant: "fixture"}
	config := &hostAuthorizationFile{WholeResources: []policy.ResourceBinding{{Resource: resource, Action: "describe"}}}
	for _, complete := range []int{0, 1, 2} {
		var documents []authz.Document
		var bindings []gating.Binding
		if complete > 0 {
			documents = []authz.Document{{Resource: resource, Revision: 1, Policies: map[string]authz.Policy{"describe": {Mode: "public"}}}}
		}
		if complete > 1 {
			bindings = []gating.Binding{{Resource: resource, Action: "describe", Document: gating.RequirementsDocument{Revision: "requirements1", Requirements: gating.Requirements{SchemaVersion: 1}}}}
		}
		policies, err := authz.NewStaticStore(documents)
		if err != nil {
			t.Fatal(err)
		}
		requirements, err := gating.NewStaticStore(bindings)
		if err != nil {
			t.Fatal(err)
		}
		err = validateHostAuthorizationMappings(config, &oauth.StaticAuthorization{Policies: policies, Requirements: requirements})
		if (err == nil) != (complete == 2) {
			t.Fatalf("complete=%d err=%v", complete, err)
		}
	}
}

type fixtureHostIdentity struct{}

func (fixtureHostIdentity) Resolve(context.Context) (authz.Facts, error) {
	return authz.Facts{}, authz.ErrDenied
}
func (fixtureHostIdentity) ResolvePrincipal(context.Context) (gating.Principal, error) {
	return gating.Principal{}, authz.ErrDenied
}
func (fixtureHostIdentity) Account(context.Context, authz.Facts) (string, error) {
	return "", authz.ErrDenied
}
func (fixtureHostIdentity) AuthorityRevision(context.Context, authz.Facts, string) (string, time.Time, error) {
	return "", time.Time{}, authz.ErrDenied
}
