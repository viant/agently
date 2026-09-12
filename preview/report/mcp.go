package reportpreview

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/viant/agently/preview/mcp/mock"
	preview "github.com/viant/agently/preview/report/provider"
)

// MockServer adapts report datasource declarations to the generic MCP replay
// server. Tool names and request routing come from the loaded report contract.
func MockServer(config Config) (*mock.Server, error) {
	p, err := loadPackage(config, config.Variant)
	if err != nil {
		return nil, err
	}
	catalogMode := config.ReportRoot != ""
	var manifest *previewManifest
	if catalogMode {
		manifest, err = loadPreviewManifest(config.Folder, config.GroupID, config.ReportID)
		if err != nil {
			return nil, err
		}
	}
	tools := map[string]mock.Tool{}
	ids := map[string]string{}
	for id, source := range p.Sources {
		if source.Service == nil || source.Service.URI == "" {
			return nil, fmt.Errorf("datasource %s requires service.endpoint and service.uri", id)
		}
		name := source.Service.URI
		d := p.Extension.DataSources[id]
		if catalogMode {
			tool := tools[name]
			if tool.File == "" {
				tool = mock.Tool{File: d.File, Description: "Contract-matched synthetic report datasource", InputSchema: mock.QueryInputSchema()}
			}
			tool.Routes = append(tool.Routes, mock.Route{Match: map[string]any{"query.dataSource": id}, File: d.File})
			tools[name] = tool
			continue
		}
		if _, ok := tools[name]; ok {
			return nil, fmt.Errorf("duplicate mock tool %q", name)
		}
		ids[name] = id
		tools[name] = mock.Tool{File: d.File, Description: "Synthetic report datasource " + id, InputSchema: mock.QueryInputSchema()}
	}
	if len(tools) == 0 {
		return nil, fmt.Errorf("report has no MCP datasources")
	}
	if catalogMode {
		tool := tools[manifest.MCP.Tool]
		if manifest.MCP.DefaultFile != "" {
			tool.File = manifest.MCP.DefaultFile
		}
		for _, configured := range manifest.MCP.Routes {
			match, _ := expandManifestValue(configured.Match, config.GroupID, config.ReportID).(map[string]any)
			tool.Routes = append(tool.Routes, mock.Route{Match: match, File: configured.File})
		}
		tools[manifest.MCP.Tool] = tool
	}
	if catalogMode && len(manifest.MCP.Definition.Match) > 0 {
		name := manifest.MCP.Definition.Tool
		if _, ok := tools[name]; !ok {
			for _, tool := range tools {
				tools[name] = mock.Tool{File: tool.File, Description: "Remote Forge report definition", InputSchema: mock.QueryInputSchema()}
				break
			}
		}
	}
	return mock.New(mock.Config{Root: config.Folder, DataRoot: p.Extension.DataRoot, Variant: p.Variant, Tools: tools, Transform: func(ctx context.Context, call mock.Call) (any, error) {
		current, err := loadPackage(config, call.Variant)
		if err != nil {
			return nil, err
		}
		if catalogMode && call.Tool == manifest.MCP.Definition.Tool {
			match, _ := expandManifestValue(manifest.MCP.Definition.Match, config.GroupID, config.ReportID).(map[string]any)
			if mock.Matches(call.Arguments, match) {
				return current.DescribeEnvelope(config.GroupID, config.ReportID), nil
			}
		}
		if call.Arguments["query"] == nil {
			if id := responseResultName(call.Body); id != "" {
				if _, ok := current.Sources[id]; ok {
					query, queryErr := mappedManifestQuery(manifest, call.Arguments)
					if queryErr != nil {
						return nil, queryErr
					}
					result, queryErr := current.ExecuteFixture(id, query, call.Body)
					if queryErr != nil {
						return nil, queryErr
					}
					return result.Response, nil
				}
			}
			var response any
			if err = json.Unmarshal(call.Body, &response); err != nil {
				return nil, err
			}
			return response, nil
		}
		body, err := json.Marshal(call.Arguments["query"])
		if err != nil {
			return nil, err
		}
		var q preview.Query
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.DisallowUnknownFields()
		if err = decoder.Decode(&q); err != nil {
			return nil, err
		}
		id := q.DataSource
		if id == "" {
			id = ids[call.Tool]
		}
		if _, ok := current.Sources[id]; !ok {
			return nil, fmt.Errorf("unknown report datasource %q", id)
		}
		result, err := current.ExecuteFixture(id, q, call.Body)
		if err != nil {
			return nil, err
		}
		return result.Response, nil
	}})
}

func responseResultName(body json.RawMessage) string {
	var envelope struct {
		Data []struct {
			Name string `json:"name"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &envelope) != nil || len(envelope.Data) != 1 {
		return ""
	}
	return strings.TrimSpace(envelope.Data[0].Name)
}

func mappedManifestQuery(manifest *previewManifest, arguments map[string]any) (preview.Query, error) {
	query := preview.Query{}
	if manifest == nil {
		return query, nil
	}
	addFields := func(path string) []preview.Field {
		value, _ := manifestValue(arguments, path)
		values, _ := value.([]any)
		result := make([]preview.Field, 0, len(values))
		for _, item := range values {
			if name, ok := item.(string); ok && strings.TrimSpace(name) != "" {
				result = append(result, preview.Field{Field: strings.TrimSpace(name)})
			}
		}
		return result
	}
	dimensions := addFields(manifest.MCP.Query.Dimensions)
	measures := addFields(manifest.MCP.Query.Measures)
	if len(dimensions) > 0 || len(measures) > 0 {
		query.Projection = &preview.Projection{Dimensions: dimensions, Measures: measures}
	}
	if value, ok := manifestValue(arguments, manifest.MCP.Query.Filter); ok && value != nil {
		body, err := json.Marshal(value)
		if err != nil {
			return query, err
		}
		filter := &preview.Predicate{}
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.DisallowUnknownFields()
		if err = decoder.Decode(filter); err != nil {
			return query, fmt.Errorf("decode mapped MCP filter: %w", err)
		}
		query.Filter = filter
	}
	if value, ok := manifestValue(arguments, manifest.MCP.Query.OrderBy); ok {
		body, err := json.Marshal(value)
		if err != nil {
			return query, err
		}
		if values, valid := value.([]any); valid && len(values) > 0 {
			if _, stringsOnly := values[0].(string); stringsOnly {
				for _, item := range values {
					parts := strings.Fields(fmt.Sprint(item))
					if len(parts) == 0 {
						continue
					}
					direction := "asc"
					if len(parts) > 1 {
						direction = strings.ToLower(parts[1])
					}
					query.OrderBy = append(query.OrderBy, preview.Order{Field: parts[0], Direction: direction})
				}
			} else if err = json.Unmarshal(body, &query.OrderBy); err != nil {
				return query, fmt.Errorf("decode mapped MCP ordering: %w", err)
			}
		}
	}
	limit, hasLimit := manifestInteger(arguments, manifest.MCP.Query.Limit)
	offset, hasOffset := manifestInteger(arguments, manifest.MCP.Query.Offset)
	if hasLimit || hasOffset {
		if limit <= 0 {
			limit = 1000
		}
		query.Page = &preview.Page{Limit: limit, Offset: offset}
	}
	return query, nil
}

func manifestInteger(arguments map[string]any, path string) (int, bool) {
	value, ok := manifestValue(arguments, path)
	if !ok {
		return 0, false
	}
	switch actual := value.(type) {
	case int:
		return actual, true
	case float64:
		return int(actual), true
	default:
		return 0, false
	}
}
