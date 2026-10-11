package agently

import (
	"context"
	"encoding/json"
	"fmt"
	identity "github.com/viant/agently-core/protocol/resource"
	"github.com/viant/agently-core/sdk/api"
	"github.com/viant/agently-core/service/ui/permittedview"
	"github.com/viant/forge/backend/types"
	"math"
	"reflect"
	"strconv"
	"strings"
)

// WindowBootstrapBinding declares the protected bootstrap of one exact window.
// Integer identities select the first requested object, matching an authored .0
// authorization selector. Other identity types are deliberately unsupported.
type WindowBootstrapBinding struct {
	URI          string `json:"uri"`
	WindowKey    string `json:"windowKey"`
	ResourceType string `json:"resourceType"`
	Parameter    string `json:"parameter"`
	DataSource   string `json:"dataSource"`
	Input        string `json:"input"`
	RowIdentity  string `json:"rowIdentity"`
	IdentityType string `json:"identityType"`
}

// ConfiguredWindowBootstrap consumes operator declarations rather than guessing
// bootstrap semantics from names, titles, fetch flags or read model placement.
// Every fetch still uses the ready protected datasource stack and original pin.
func ConfiguredWindowBootstrap(bindings []WindowBootstrapBinding) WindowOpenBootstrapFactory {
	copied := append([]WindowBootstrapBinding(nil), bindings...)
	return func(_ context.Context, source WindowBootstrapDatasource) (permittedview.OpenBootstrap, permittedview.OpenSelectionCheck, error) {
		if source == nil {
			return nil, nil, identity.ErrResourceDenied
		}
		byURI := map[string]WindowBootstrapBinding{}
		byKey := map[string]WindowBootstrapBinding{}
		if err := validateWindowBootstrapBindings(copied); err != nil {
			return nil, nil, err
		}
		for _, binding := range copied {
			byURI[binding.URI] = binding
			byKey[binding.WindowKey] = binding
		}

		load := func(ctx context.Context, pin identity.ResolvedResource, w *types.Window, parameters map[string]any) (map[string]any, error) {
			binding, ok := byURI[pin.URI]
			if !ok || !matchesDeclaredBootstrap(w, binding) {
				return nil, identity.ErrResourceDenied
			}
			primary, err := declaredBootstrapParameter(w, binding.Parameter, parameters[binding.Parameter])
			if err != nil {
				return nil, identity.ErrResourceDenied
			}
			ids, err := authoredBootstrapIDs(primary)
			if err != nil {
				return nil, identity.ErrResourceDenied
			}
			inputs := map[string]interface{}{binding.Input: ids}
			for _, p := range w.DataSource[binding.DataSource].Parameters {
				from := p.From
				if from == "" {
					from = p.In
				}
				if from != "windowForm" || p.Name == binding.Input {
					continue
				}
				value, present := bootstrapParameterValue(parameters, p.Location)
				if present && value != nil {
					value, err = declaredBootstrapParameter(w, p.Location, value)
					if err != nil {
						return nil, identity.ErrResourceDenied
					}
					inputs[p.Name] = value
				} else if p.Required != nil && *p.Required {
					return nil, identity.ErrResourceDenied
				}
			}
			result, err := source.FetchDatasource(ctx, &api.FetchDatasourceInput{ID: binding.DataSource, Resource: &pin, Target: w.ResourceTarget, Inputs: inputs, Cache: &api.DatasourceCacheHints{BypassCache: true}})
			if err != nil {
				return nil, err
			}
			if result == nil || len(result.Rows) != 1 {
				return nil, identity.ErrResourceDenied
			}
			returned, err := authoredBootstrapID(result.Rows[0][binding.RowIdentity])
			if err != nil || returned != ids[0] {
				return nil, identity.ErrResourceDenied
			}
			return map[string]any{binding.Parameter: []int64{returned}}, nil
		}
		match := func(_ context.Context, w *types.Window, parameters, row map[string]any) error {
			if w == nil {
				return identity.ErrResourceDenied
			}
			binding, ok := byKey[w.WindowKey]
			if !ok || !matchesDeclaredBootstrap(w, binding) {
				return identity.ErrResourceDenied
			}
			requested, err := authoredBootstrapIDs(parameters[binding.Parameter])
			if err != nil {
				return identity.ErrResourceDenied
			}
			returned, err := authoredBootstrapIDs(row[binding.Parameter])
			if err != nil || len(returned) != 1 || returned[0] != requested[0] {
				return identity.ErrResourceDenied
			}
			return nil
		}
		return load, match, nil
	}
}
func matchesDeclaredBootstrap(w *types.Window, b WindowBootstrapBinding) bool {
	if w == nil || w.WindowKey != b.WindowKey || w.Authorization == nil || !strings.EqualFold(w.Authorization.Scope, "resource") || w.Authorization.Resource == nil {
		return false
	}
	resource := w.Authorization.Resource
	if resource.Type != b.ResourceType || resource.ID.Source != "resource" || resource.ID.Selector != b.Parameter+".0" {
		return false
	}
	declared := false
	for _, parameter := range w.Parameters {
		if parameter["name"] != b.Parameter {
			continue
		}
		switch parameter["type"] {
		case "int", "integer", "int[]", "integer[]":
			declared = true
		default:
			return false
		}
	}
	if !declared {
		return false
	}
	ds, ok := w.DataSource[b.DataSource]
	if !ok {
		return false
	}
	if len(ds.UniqueKey) != 1 || ds.UniqueKey[0] == nil || ds.UniqueKey[0].Field != b.RowIdentity {
		return false
	}
	matches := 0
	for _, p := range ds.Parameters {
		from := p.From
		if from == "" {
			from = p.In
		}
		if from == "windowForm" && p.Name == b.Input && p.Location == b.Parameter {
			matches++
		}
	}
	return matches == 1
}
func bootstrapParameterValue(parameters map[string]any, path string) (any, bool) {
	var current any = parameters
	for _, part := range strings.Split(path, ".") {
		if part == "" {
			return nil, false
		}
		if values, ok := current.(map[string]any); ok {
			var exists bool
			current, exists = values[part]
			if !exists {
				return nil, false
			}
			continue
		}
		value := reflect.ValueOf(current)
		index, err := strconv.Atoi(part)
		if err != nil || !value.IsValid() || (value.Kind() != reflect.Slice && value.Kind() != reflect.Array) || index < 0 || index >= value.Len() {
			return nil, false
		}
		current = value.Index(index).Interface()
	}
	return current, true
}
func authoredBootstrapIDs(value any) ([]int64, error) {
	if value == nil {
		return nil, identity.ErrResourceDenied
	}
	reflected := reflect.ValueOf(value)
	values := []any{value}
	if reflected.Kind() == reflect.Slice || reflected.Kind() == reflect.Array {
		if reflected.Len() < 1 || reflected.Len() > 100 {
			return nil, identity.ErrResourceDenied
		}
		values = make([]any, reflected.Len())
		for i := range values {
			values[i] = reflected.Index(i).Interface()
		}
	}
	result := make([]int64, len(values))
	for i, item := range values {
		id, e := authoredBootstrapID(item)
		if e != nil {
			return nil, e
		}
		result[i] = id
	}
	return result, nil
}
func authoredBootstrapID(value any) (int64, error) {
	var text string
	switch v := value.(type) {
	case json.Number:
		text = v.String()
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) || v != math.Trunc(v) || v > 9007199254740991 {
			return 0, identity.ErrResourceDenied
		}
		text = strconv.FormatFloat(v, 'f', 0, 64)
	case int:
		text = strconv.Itoa(v)
	case int64:
		text = strconv.FormatInt(v, 10)
	case int32:
		text = strconv.FormatInt(int64(v), 10)
	case uint64:
		if v > math.MaxInt64 {
			return 0, identity.ErrResourceDenied
		}
		text = strconv.FormatUint(v, 10)
	default:
		return 0, identity.ErrResourceDenied
	}
	id, e := strconv.ParseInt(text, 10, 64)
	if e != nil || id <= 0 {
		return 0, fmt.Errorf("invalid selected resource identity")
	}
	return id, nil
}

func validateWindowBootstrapBindings(bindings []WindowBootstrapBinding) error {
	byURI := map[string]WindowBootstrapBinding{}
	byKey := map[string]WindowBootstrapBinding{}
	for _, binding := range bindings {
		uri, err := identity.ParseResourceURI(binding.URI)
		if err != nil || uri.Kind != "window" || binding.WindowKey == "" || binding.ResourceType == "" || binding.Parameter == "" || binding.DataSource == "" || binding.Input == "" || binding.RowIdentity == "" || binding.IdentityType != "integer" || strings.ContainsAny(binding.Parameter, ".[]/") {
			return fmt.Errorf("invalid window bootstrap declaration")
		}
		if _, ok := byURI[binding.URI]; ok {
			return fmt.Errorf("duplicate window bootstrap URI")
		}
		if _, ok := byKey[binding.WindowKey]; ok {
			return fmt.Errorf("ambiguous window bootstrap key")
		}
		byURI[binding.URI] = binding
		byKey[binding.WindowKey] = binding
	}
	return nil
}

// Only an existing authored integer declaration triggers integer conversion.
// Other values remain input hints for the protected datasource's own binder.
func declaredBootstrapParameter(w *types.Window, name string, value any) (any, error) {
	declared := ""
	for _, parameter := range w.Parameters {
		if parameter["name"] != name {
			continue
		}
		kind, ok := parameter["type"].(string)
		if !ok || kind == "" {
			return nil, identity.ErrResourceDenied
		}
		if declared != "" && declared != kind {
			return nil, identity.ErrResourceDenied
		}
		declared = kind
	}
	switch declared {
	case "int[]", "integer[]":
		return authoredBootstrapIDs(value)
	case "int", "integer":
		return authoredBootstrapID(value)
	default:
		return value, nil
	}
}
