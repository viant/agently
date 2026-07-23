package agently

import (
	"context"
	"fmt"
	"log"
	"strings"

	uiview "github.com/viant/agently-core/protocol/tool/service/ui/view"
	viewproto "github.com/viant/agently-core/protocol/ui/view"
	windowloader "github.com/viant/agently-core/service/ui/window"
	wscfg "github.com/viant/agently-core/workspace/config"
	reportregistry "github.com/viant/forge/backend/reporting/registry"
	forgeTypes "github.com/viant/forge/backend/types"
)

type workspaceReportingRuntime struct {
	loader        *reportregistry.Loader
	watcher       *reportregistry.Watcher
	windowCleanup func()
}

func configureWorkspaceReporting(ctx context.Context, workspaceRoot string, config *wscfg.Root, development bool) (*workspaceReportingRuntime, error) {
	reportingRoot := ""
	if config != nil {
		reportingRoot = config.ForgeReportingRoot()
	}
	loader := reportregistry.NewLoader(reportregistry.Options{
		WorkspaceRoot: workspaceRoot,
		ReportingRoot: reportingRoot,
	})
	discovered, err := loader.Reload(ctx)
	if err != nil {
		return nil, fmt.Errorf("load workspace reporting registry: %w", err)
	}
	log.Printf(
		"agently-app: workspace reporting registry loaded: root=%s builders=%d presets=%d fragments=%d",
		discovered.Root,
		len(discovered.Builders),
		len(discovered.Presets),
		len(discovered.Fragments),
	)
	runtime := &workspaceReportingRuntime{
		loader:        loader,
		windowCleanup: windowloader.SetWorkspaceWindowEnricher(workspaceReportingEnricher(loader)),
	}
	if development {
		runtime.watcher = reportregistry.NewWatcher(loader)
		if err = runtime.watcher.Start(ctx, func(current *reportregistry.Registry, reloadErr error) {
			if reloadErr != nil {
				log.Printf("agently-app: workspace reporting registry reload rejected; retaining last valid registry: %v", reloadErr)
				return
			}
			log.Printf(
				"agently-app: workspace reporting registry reloaded: root=%s builders=%d presets=%d fragments=%d",
				current.Root,
				len(current.Builders),
				len(current.Presets),
				len(current.Fragments),
			)
		}); err != nil {
			runtime.Close()
			return nil, fmt.Errorf("watch workspace reporting registry: %w", err)
		}
		log.Printf("agently-app: watching workspace reporting assets under %s", discovered.Root)
	}
	return runtime, nil
}

func (r *workspaceReportingRuntime) Close() {
	if r == nil {
		return
	}
	if r.watcher != nil {
		_ = r.watcher.Close()
	}
	if r.windowCleanup != nil {
		r.windowCleanup()
	}
}

func (r *workspaceReportingRuntime) EnrichView(_ context.Context, item *uiview.ListItem) error {
	if r == nil || r.loader == nil || item == nil {
		return nil
	}
	builderRef := strings.TrimSpace(item.ReportBuilderRef)
	if builderRef == "" {
		return nil
	}
	discovered := r.loader.Current()
	if discovered == nil || discovered.Builder(builderRef) == nil {
		return fmt.Errorf("report builder %q is not available for UI view %q", builderRef, item.ID)
	}
	presets := discovered.PresetsForBuilder(builderRef)
	item.ReportPresets = make([]viewproto.ReportPreset, 0, len(presets))
	for _, preset := range presets {
		if preset == nil {
			continue
		}
		item.ReportPresets = append(item.ReportPresets, viewproto.ReportPreset{
			ID:          preset.ID,
			Label:       preset.Label,
			Description: preset.Description,
		})
	}
	return nil
}

func workspaceReportingEnricher(loader *reportregistry.Loader) windowloader.WorkspaceWindowEnricher {
	return func(_ context.Context, window *forgeTypes.Window) error {
		if window == nil || window.View.Content == nil || window.View.Content.Kind != "dashboard.reportBuilder" {
			return nil
		}
		content := window.View.Content
		if content.Dashboard == nil {
			return nil
		}
		registry := loader.Current()
		if registry == nil {
			return fmt.Errorf("workspace reporting registry is not initialized")
		}
		if content.Dashboard.ReportBuilders != nil {
			for _, builder := range registry.Builders {
				if builder == nil {
					continue
				}
				content.Dashboard.ReportBuilders[builder.ID] = reportBuilderVariant(builder, registry.PresetsForBuilder(builder.ID))
			}
		}
		builderRef := strings.TrimSpace(content.Dashboard.ReportBuilderRef)
		if builderRef == "" {
			return nil
		}
		builder := registry.Builder(builderRef)
		if builder == nil {
			return fmt.Errorf("report builder %q is not available in workspace reporting root %s", builderRef, registry.Root)
		}
		base := reportBuilderConfig(builder.Raw)
		merged := mergeReportingMaps(base, content.Dashboard.ReportBuilder)
		merged["reportDocumentTemplates"] = mergeDiscoveredPresetTemplates(
			listOfMaps(merged["reportDocumentTemplates"]),
			registry.PresetsForBuilder(builder.ID),
		)
		content.Dashboard.ReportBuilder = merged
		return nil
	}
}

func reportBuilderVariant(builder *reportregistry.Asset, presets []*reportregistry.Asset) map[string]any {
	config := reportBuilderConfig(builder.Raw)
	config["reportDocumentTemplates"] = mergeDiscoveredPresetTemplates(
		listOfMaps(config["reportDocumentTemplates"]),
		presets,
	)
	result := map[string]any{
		"id":            builder.ID,
		"dataSourceRef": strings.TrimSpace(fmt.Sprint(builder.Raw["dataSourceRef"])),
		"reportBuilder": config,
	}
	if builder.Label != "" {
		result["label"] = builder.Label
	} else if title := strings.TrimSpace(fmt.Sprint(builder.Raw["title"])); title != "" {
		result["label"] = title
	}
	return result
}

func reportBuilderConfig(raw map[string]any) map[string]any {
	if nested, ok := raw["reportBuilder"].(map[string]any); ok {
		return cloneReportingMap(nested)
	}
	result := cloneReportingMap(raw)
	for _, key := range []string{"kind", "id", "label", "title", "description"} {
		delete(result, key)
	}
	return result
}

func mergeDiscoveredPresetTemplates(existing []map[string]any, presets []*reportregistry.Asset) []any {
	result := make([]any, 0, len(existing)+len(presets))
	byID := map[string]int{}
	for _, template := range existing {
		next := cloneReportingMap(template)
		next["sourceKind"] = "preset"
		next["readOnly"] = true
		id := normalizeReportingID(next["id"])
		if id != "" {
			byID[id] = len(result)
		}
		result = append(result, next)
	}
	for _, preset := range presets {
		if preset == nil {
			continue
		}
		template := presetTemplate(preset)
		id := normalizeReportingID(template["id"])
		if id == "" {
			continue
		}
		if index, ok := byID[id]; ok {
			result[index] = mergeReportingMaps(result[index].(map[string]any), template)
			continue
		}
		byID[id] = len(result)
		result = append(result, template)
	}
	return result
}

func presetTemplate(preset *reportregistry.Asset) map[string]any {
	result := cloneReportingMap(preset.Raw)
	result["id"] = preset.ID
	result["builderRef"] = preset.BuilderRef
	result["sourceKind"] = "preset"
	result["readOnly"] = true
	if preset.Label != "" {
		result["label"] = preset.Label
	}
	if preset.Description != "" {
		result["description"] = preset.Description
	}
	if document, ok := result["document"].(map[string]any); ok {
		result["documentPatch"] = document
		delete(result, "document")
	}
	delete(result, "kind")
	return result
}

func mergeReportingMaps(base, override map[string]any) map[string]any {
	result := cloneReportingMap(base)
	for key, value := range override {
		if overrideMap, ok := value.(map[string]any); ok {
			if baseMap, baseOK := result[key].(map[string]any); baseOK {
				result[key] = mergeReportingMaps(baseMap, overrideMap)
				continue
			}
		}
		result[key] = cloneReportingValue(value)
	}
	return result
}

func cloneReportingMap(source map[string]any) map[string]any {
	result := make(map[string]any, len(source))
	for key, value := range source {
		result[key] = cloneReportingValue(value)
	}
	return result
}

func cloneReportingValue(value any) any {
	switch actual := value.(type) {
	case map[string]any:
		return cloneReportingMap(actual)
	case []any:
		result := make([]any, len(actual))
		for index, item := range actual {
			result[index] = cloneReportingValue(item)
		}
		return result
	default:
		return actual
	}
}

func listOfMaps(value any) []map[string]any {
	items, _ := value.([]any)
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if mapped, ok := item.(map[string]any); ok {
			result = append(result, mapped)
		}
	}
	return result
}

func normalizeReportingID(value any) string {
	return strings.ToLower(strings.TrimSpace(fmt.Sprint(value)))
}
