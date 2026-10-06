package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/viant/agently-core/app/executor"
	documents "github.com/viant/agently-core/app/store/reportingevidence"
	"github.com/viant/agently-core/genai/llm"
	"github.com/viant/agently-core/protocol/tool"
	agentsvc "github.com/viant/agently-core/service/agent"
	fb "github.com/viant/agently-core/service/reporting/forecastbinding"
	wsconfig "github.com/viant/agently-core/workspace/config"
)

type ForecastEvidenceConfig struct {
	Enabled       bool   `json:"enabled"`
	Profile       string `json:"profile"`
	TimeZone      string `json:"timeZone"`
	RolloutCutoff string `json:"rolloutCutoff"`
	cutoff        time.Time
}

// LoadForecastEvidenceConfig reads only explicit workspace opt-in. No environment,
// model hint, prompt, or current boot clock can enable or date a rollout.
func LoadForecastEvidenceConfig(root string) (ForecastEvidenceConfig, error) {
	return loadForecastEvidenceConfigAt(root, time.Now())
}
func loadForecastEvidenceConfigAt(root string, activationTime time.Time) (ForecastEvidenceConfig, error) {
	var result ForecastEvidenceConfig
	config, err := wsconfig.Load(root)
	if err != nil {
		return result, err
	}
	if config == nil {
		return result, nil
	}
	features, ok := config.Raw["features"].(map[string]interface{})
	if !ok {
		return result, nil
	}
	raw, exists := features["forecastEvidence"]
	if !exists {
		return result, nil
	}
	body, err := json.Marshal(raw)
	if err != nil {
		return result, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&result); err != nil {
		return result, fmt.Errorf("forecast evidence configuration: %w", err)
	}
	if !result.Enabled {
		return result, nil
	}
	if result.Profile != fb.Profile {
		return result, fmt.Errorf("forecast evidence unsupported profile %q", result.Profile)
	}
	if result.TimeZone == "" {
		return result, fmt.Errorf("forecast evidence timeZone required")
	}
	if _, err = time.LoadLocation(result.TimeZone); err != nil {
		return result, fmt.Errorf("forecast evidence timeZone: %w", err)
	}
	result.cutoff, err = time.Parse(time.RFC3339Nano, result.RolloutCutoff)
	if err != nil || result.cutoff.IsZero() {
		return result, fmt.Errorf("forecast evidence fixed RFC3339 rolloutCutoff required")
	}
	if activationTime.IsZero() || result.cutoff.After(activationTime) {
		return result, fmt.Errorf("forecast evidence rolloutCutoff must not be after activation time")
	}
	return result, nil
}

// ConfigureForecastEvidence runs before serving. It uses the same native runtime
// and immutable documents for agent publication, linked compilation and completion.
// Disabled workspaces install restore-only authority, without warmup or new
// admissions. Historical saved report bodies are not modified here.
func ConfigureForecastEvidence(ctx context.Context, rt *executor.Runtime, root string) error {
	config, err := LoadForecastEvidenceConfig(root)
	if err != nil {
		return err
	}
	if !config.Enabled {
		if rt == nil || rt.Native == nil || rt.Conversation == nil || rt.Data == nil || rt.Agent == nil {
			return nil
		}
		_, err = installForecastEvidence(rt, config)
		return err
	}
	if rt == nil || rt.Native == nil || rt.Conversation == nil || rt.Data == nil || rt.Agent == nil || rt.Reporting == nil || rt.ReportRuns == nil || rt.Registry == nil {
		return fmt.Errorf("forecast evidence requires native, agent and durable report services")
	}
	if err = waitForecastRegistryWarmup(ctx, rt, 15*time.Second); err != nil {
		return err
	}
	if err = validateForecastEvidenceCapability(ctx, rt.Registry); err != nil {
		return err
	}
	_, err = installForecastEvidence(rt, config)
	return err
}

// Reuse the runtime-owned initializer and its lifetime; a short preflight wait
// context must not become the registry refresh lifetime.
func waitForecastRegistryWarmup(ctx context.Context, rt *executor.Runtime, budget time.Duration) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("forecast evidence registry warmup: %w", err)
	}
	done := rt.InitializeRegistryAsync(ctx, 15*time.Second)
	waiting, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	select {
	case <-done:
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("forecast evidence registry warmup: %w", err)
		}
		return nil
	case <-waiting.Done():
		return fmt.Errorf("forecast evidence registry warmup: %w", waiting.Err())
	}
}

func installForecastEvidence(rt *executor.Runtime, config ForecastEvidenceConfig) (*fb.Factory, error) {
	store := fb.NewNativeSourceStore(rt.Conversation, rt.Data, documents.New(rt.Native))
	zone := config.TimeZone
	options := []fb.FactoryOption{}
	if config.Enabled {
		options = append(options, fb.WithRolloutCutoff(config.cutoff))
	} else {
		zone = "UTC"
		options = append(options, fb.WithRestoreOnly())
	}
	factory, err := fb.NewFactory(fb.ProjectionPolicyProducer{}, store, zone, options...)
	if err != nil {
		return nil, err
	}
	runtime, err := fb.NewRuntime(fb.ProjectionPolicyProducer{}, store)
	if err != nil {
		return nil, err
	}
	backend, err := fb.NewReportCommandBackend(runtime, store)
	if err != nil {
		return nil, err
	}
	if rt.Reporting != nil {
		if err = rt.Reporting.ConfigureReportCompilation(backend); err != nil {
			return nil, err
		}
	}
	if rt.ReportRuns != nil {
		if err = rt.ReportRuns.ConfigureReportAdmissions(backend); err != nil {
			return nil, err
		}
	}
	agentsvc.WithEvidenceFactory(factory)(rt.Agent)
	return factory, nil
}

func validateForecastEvidenceCapability(ctx context.Context, registry tool.Registry) error {
	const name = "steward/ForecastingTargetingConvert"
	var definition *llm.ToolDefinition
	var found bool
	if getter, ok := registry.(tool.ContextDefinitionGetter); ok {
		definition, found = getter.GetDefinitionWithContext(ctx, name)
	} else {
		definition, found = registry.GetDefinition(name)
	}
	if !found || definition == nil {
		return fmt.Errorf("forecast evidence converter capability unavailable")
	}
	for key, kind := range map[string]string{"evidenceProfile": "object", "evidenceAudienceId": "integer", "evidenceSourceOpId": "string"} {
		schema := schemaProperty(definition.Parameters, "Request", key)
		if !schemaHasType(definition.Parameters, schema, kind) {
			return fmt.Errorf("forecast evidence converter does not declare Request.%s (%s)", key, kind)
		}
	}
	if !schemaHasType(definition.OutputSchema, schemaProperty(definition.OutputSchema, "forecastEvidence"), "object") {
		return fmt.Errorf("forecast evidence converter does not declare forecastEvidence output")
	}
	return nil
}

// Resolve local schema references only; external refs cannot establish startup capability.
func resolveSchema(root, node map[string]interface{}) map[string]interface{} {
	seen := map[string]bool{}
	for node != nil {
		ref, _ := node["$ref"].(string)
		if ref == "" {
			return node
		}
		if !strings.HasPrefix(ref, "#/") || seen[ref] {
			return nil
		}
		seen[ref] = true
		value := interface{}(root)
		for _, part := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
			object, ok := value.(map[string]interface{})
			if !ok {
				return nil
			}
			part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
			value = object[part]
		}
		node, _ = value.(map[string]interface{})
	}
	return nil
}
func normalizedSchema(root map[string]interface{}) map[string]interface{} {
	body, err := json.Marshal(root)
	if err != nil {
		return nil
	}
	var normalized map[string]interface{}
	if json.Unmarshal(body, &normalized) != nil {
		return nil
	}
	return normalized
}

func schemaProperty(root map[string]interface{}, path ...string) map[string]interface{} {
	root = normalizedSchema(root)
	node := root
	for _, key := range path {
		node = resolveSchema(root, node)
		if node == nil {
			return nil
		}
		switch props := node["properties"].(type) {
		case map[string]interface{}:
			node, _ = props[key].(map[string]interface{})
		case map[string]map[string]interface{}:
			node = props[key]
		default:
			return nil
		}
	}
	return resolveSchema(root, node)
}
func schemaHasType(root, node map[string]interface{}, kind string) bool {
	return schemaHasTypeDepth(normalizedSchema(root), node, kind, 0)
}
func schemaHasTypeDepth(root, node map[string]interface{}, kind string, depth int) bool {
	if depth > 16 {
		return false
	}
	node = resolveSchema(root, node)
	if node == nil {
		return false
	}
	if node["type"] == kind {
		return true
	}
	if types, ok := node["type"].([]interface{}); ok {
		for _, item := range types {
			if item == kind {
				return true
			}
		}
	}
	// Nullable schema alternatives retain the explicitly declared concrete type.
	for _, union := range []string{"anyOf", "oneOf"} {
		if branches, ok := node[union].([]interface{}); ok {
			for _, raw := range branches {
				branch, _ := raw.(map[string]interface{})
				if schemaHasTypeDepth(root, branch, kind, depth+1) {
					return true
				}
			}
		}
	}
	return false
}
