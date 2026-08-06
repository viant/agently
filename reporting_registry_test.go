package agently

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	uiview "github.com/viant/agently-core/protocol/tool/service/ui/view"
	windowloader "github.com/viant/agently-core/service/ui/window"
	"github.com/viant/agently-core/workspace"
	wscfg "github.com/viant/agently-core/workspace/config"
	reportregistry "github.com/viant/forge/backend/reporting/registry"
	metasvc "github.com/viant/forge/backend/service/meta"
	forgeTypes "github.com/viant/forge/backend/types"
)

func TestWorkspaceReportingEnricherResolvesBuilderAndReadOnlyPresets(t *testing.T) {
	workspace := t.TempDir()
	writeReportingTestAsset(t, workspace, "extension/forge/reporting/performance/builder.yaml", `
kind: forge.reporting.builder
id: performance
reportBuilder:
  title: Performance
  filterPresentation: rail-left
`)
	writeReportingTestAsset(t, workspace, "extension/forge/reporting/performance/presets/command-center.yaml", `
kind: forge.reporting.preset
id: command_center
builderRef: performance
label: Command Center
description: Complete delivery dashboard
statePatch:
  selectedMeasures: [spend]
document:
  title: Command Center
  blocks:
    - {id: summary, kind: kpiBlock, datasetRef: primary}
`)
	loader := reportregistry.NewLoader(reportregistry.Options{WorkspaceRoot: workspace})
	if _, err := loader.Reload(context.Background()); err != nil {
		t.Fatalf("reload registry: %v", err)
	}
	window := &forgeTypes.Window{View: forgeTypes.View{Content: &forgeTypes.Container{
		ID:   "performanceWindow",
		Kind: "dashboard.reportBuilder",
		Dashboard: &forgeTypes.Dashboard{
			ReportBuilderRef: "performance",
			ReportBuilder: map[string]any{
				"filterPresentation": "drawer-left",
			},
		},
	}}}
	if err := workspaceReportingEnricher(loader)(context.Background(), window); err != nil {
		t.Fatalf("enrich report window: %v", err)
	}
	config := window.View.Content.Dashboard.ReportBuilder
	if config["title"] != "Performance" || config["filterPresentation"] != "drawer-left" {
		t.Fatalf("expected registry base with window override, got %#v", config)
	}
	templates, _ := config["reportDocumentTemplates"].([]any)
	if len(templates) != 1 {
		t.Fatalf("expected one discovered preset, got %#v", templates)
	}
	template, _ := templates[0].(map[string]any)
	if template["id"] != "command_center" || template["readOnly"] != true || template["sourceKind"] != "preset" {
		t.Fatalf("unexpected discovered preset template %#v", template)
	}
	if _, ok := template["documentPatch"].(map[string]any); !ok {
		t.Fatalf("expected explicit preset document to normalize to documentPatch: %#v", template)
	}
}

func TestWorkspaceReportingEnricherRejectsUnknownReference(t *testing.T) {
	workspace := t.TempDir()
	loader := reportregistry.NewLoader(reportregistry.Options{WorkspaceRoot: workspace})
	if _, err := loader.Reload(context.Background()); err != nil {
		t.Fatalf("reload empty registry: %v", err)
	}
	window := &forgeTypes.Window{View: forgeTypes.View{Content: &forgeTypes.Container{
		Kind:      "dashboard.reportBuilder",
		Dashboard: &forgeTypes.Dashboard{ReportBuilderRef: "missing"},
	}}}
	if err := workspaceReportingEnricher(loader)(context.Background(), window); err == nil {
		t.Fatalf("expected unknown builder reference to fail")
	}
}

func TestWorkspaceReportingEnricherBuildsVariantCatalogFromWorkspaceAssets(t *testing.T) {
	workspace := t.TempDir()
	writeReportingTestAsset(t, workspace, "extension/forge/reporting/alpha/builder.yaml", `
kind: forge.reporting.builder
id: alpha
label: Alpha Report Family
dataSourceRef: alpha_source
reportBuilder:
  filterPresentation: inline
`)
	writeReportingTestAsset(t, workspace, "extension/forge/reporting/beta/builder.yaml", `
kind: forge.reporting.builder
id: beta
label: Beta Report Family
dataSourceRef: beta_source
reportBuilder:
  filterPresentation: drawer-left
`)
	loader := reportregistry.NewLoader(reportregistry.Options{WorkspaceRoot: workspace})
	if _, err := loader.Reload(context.Background()); err != nil {
		t.Fatalf("reload registry: %v", err)
	}
	window := &forgeTypes.Window{View: forgeTypes.View{Content: &forgeTypes.Container{
		ID:   "canonicalReportWindow",
		Kind: "dashboard.reportBuilder",
		Dashboard: &forgeTypes.Dashboard{
			ReportBuilderRef: "alpha",
			ReportBuilders:   map[string]map[string]interface{}{},
		},
	}}}
	if err := workspaceReportingEnricher(loader)(context.Background(), window); err != nil {
		t.Fatalf("enrich report window: %v", err)
	}
	catalog := window.View.Content.Dashboard.ReportBuilders
	if len(catalog) != 2 {
		t.Fatalf("expected every workspace-defined builder variant, got %#v", catalog)
	}
	if got := catalog["alpha"]["dataSourceRef"]; got != "alpha_source" {
		t.Fatalf("expected alpha workspace data source, got %#v", got)
	}
	if got := catalog["beta"]["label"]; got != "Beta Report Family" {
		t.Fatalf("expected beta workspace label, got %#v", got)
	}
	betaConfig, _ := catalog["beta"]["reportBuilder"].(map[string]any)
	if got := betaConfig["filterPresentation"]; got != "drawer-left" {
		t.Fatalf("expected beta workspace config, got %#v", betaConfig)
	}
}

func TestWorkspaceReportingRuntimeEnrichesViewCatalog(t *testing.T) {
	workspace := t.TempDir()
	writeReportingTestAsset(t, workspace, "extension/forge/reporting/performance/builder.yaml", `
kind: forge.reporting.builder
id: performance
reportBuilder: {}
`)
	writeReportingTestAsset(t, workspace, "extension/forge/reporting/performance/preset.yaml", `
kind: forge.reporting.preset
id: command_center
builderRef: performance
label: Command Center
description: Complete dashboard
document: {blocks: []}
`)
	loader := reportregistry.NewLoader(reportregistry.Options{WorkspaceRoot: workspace})
	if _, err := loader.Reload(context.Background()); err != nil {
		t.Fatalf("reload registry: %v", err)
	}
	runtime := &workspaceReportingRuntime{loader: loader}
	item := &uiview.ListItem{ID: "performanceView", ReportBuilderRef: "performance"}
	if err := runtime.EnrichView(context.Background(), item); err != nil {
		t.Fatalf("enrich view: %v", err)
	}
	if len(item.ReportPresets) != 1 || item.ReportPresets[0].ID != "command_center" || item.ReportPresets[0].Description == "" {
		t.Fatalf("unexpected discovered view catalog %#v", item.ReportPresets)
	}
}

func TestConfigureWorkspaceReportingStartsWatcherOnlyInDevelopment(t *testing.T) {
	workspace := t.TempDir()
	writeReportingTestAsset(t, workspace, "extension/forge/reporting/performance/builder.yaml", `
kind: forge.reporting.builder
id: performance
reportBuilder: {}
`)
	runtime, err := configureWorkspaceReporting(context.Background(), workspace, nil, false)
	if err != nil {
		t.Fatalf("configure production registry: %v", err)
	}
	if runtime.watcher != nil {
		t.Fatalf("production registry unexpectedly started a filesystem watcher")
	}
	runtime.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runtime, err = configureWorkspaceReporting(ctx, workspace, nil, true)
	if err != nil {
		t.Fatalf("configure development registry: %v", err)
	}
	defer runtime.Close()
	if runtime.watcher == nil {
		t.Fatalf("development registry did not start its filesystem watcher")
	}
}

func TestConfiguredStewardReportingRootEnrichesCanonicalWindow(t *testing.T) {
	workspaceRoot := strings.TrimSpace(os.Getenv("FORGE_REPORTING_TEST_WORKSPACE"))
	if workspaceRoot == "" {
		t.Skip("FORGE_REPORTING_TEST_WORKSPACE is not set")
	}
	config, err := wscfg.Load(workspaceRoot)
	if err != nil {
		t.Fatalf("load workspace config: %v", err)
	}
	if got := config.ForgeReportingRoot(); got == "" {
		t.Fatalf("workspace config does not declare forge.reporting.root")
	}

	previousRoot := workspace.Root()
	workspace.SetRoot(workspaceRoot)
	t.Cleanup(func() { workspace.SetRoot(previousRoot) })

	runtime, err := configureWorkspaceReporting(context.Background(), workspaceRoot, config, false)
	if err != nil {
		t.Fatalf("configure workspace reporting: %v", err)
	}
	t.Cleanup(runtime.Close)

	window, err := windowloader.LoadWorkspaceWindow(context.Background(), "reportBuilder", &metasvc.TargetContext{
		Platform:   "web",
		FormFactor: "desktop",
		Surface:    "browser",
	})
	if err != nil {
		t.Fatalf("load canonical report window: %v", err)
	}
	if window == nil || window.View.Content == nil || window.View.Content.Dashboard == nil {
		t.Fatalf("canonical report window is incomplete: %#v", window)
	}
	builders := window.View.Content.Dashboard.ReportBuilders
	if len(builders) != 2 {
		t.Fatalf("expected two discovered Steward report builders, got %#v", builders)
	}
	assertDiscoveredPresetCount(t, builders, "metricsCubeBuilder", 4)
	assertDiscoveredPresetCount(t, builders, "forecastingCubeBuilder", 3)

	actionCode := ""
	if window.Actions != nil {
		actionCode = window.Actions.Code
	}
	if !strings.Contains(actionCode, "stewardReportBuilder") || !strings.Contains(actionCode, "stewardForecastingBuilder") {
		t.Fatalf("canonical report window did not merge both workspace hook families")
	}
}

func assertDiscoveredPresetCount(t *testing.T, builders map[string]map[string]interface{}, builderRef string, want int) {
	t.Helper()
	variant := builders[builderRef]
	if variant == nil {
		t.Fatalf("missing discovered builder variant %q", builderRef)
	}
	config, _ := variant["reportBuilder"].(map[string]any)
	templates, _ := config["reportDocumentTemplates"].([]any)
	if len(templates) != want {
		t.Fatalf("builder %q preset count = %d, want %d", builderRef, len(templates), want)
	}
	for _, raw := range templates {
		template, _ := raw.(map[string]any)
		if template["sourceKind"] != "preset" || template["readOnly"] != true {
			t.Fatalf("builder %q contains non-preset or writable built-in template: %#v", builderRef, template)
		}
	}
}

func writeReportingTestAsset(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir reporting asset: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write reporting asset: %v", err)
	}
}
