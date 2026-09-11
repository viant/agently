package reportpreview

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/viant/agently/preview/mcp/mock"
	preview "github.com/viant/agently/preview/report/provider"
)

// MockServer adapts the shared filesystem MCP server to the report's declared
// tool identities and typed query capabilities. Windows can use mock.New directly.
func MockServer(config Config) (*mock.Server, error) {
	p, err := preview.Load(config.Folder, config.Variant)
	if err != nil {
		return nil, err
	}
	tools := map[string]mock.Tool{}
	ids := map[string]string{}
	for id, source := range p.Sources {
		if source.Service == nil || source.Service.URI == "" {
			return nil, fmt.Errorf("datasource %s requires service.endpoint and service.uri", id)
		}
		name := source.Service.URI
		if _, ok := tools[name]; ok {
			return nil, fmt.Errorf("duplicate mock tool %q", name)
		}
		ids[name] = id
		d := p.Extension.DataSources[id]
		tools[name] = mock.Tool{File: d.File, Description: "Synthetic report datasource " + id, InputSchema: mock.QueryInputSchema()}
	}
	return mock.New(mock.Config{Root: config.Folder, DataRoot: p.Extension.DataRoot, Variant: p.Variant, Tools: tools, Transform: func(ctx context.Context, call mock.Call) (any, error) {
		for key := range call.Arguments {
			if key != "query" && key != "variant" {
				return nil, fmt.Errorf("unknown tool argument %q", key)
			}
		}
		body, err := json.Marshal(call.Arguments["query"])
		if err != nil {
			return nil, err
		}
		var q preview.Query
		if call.Arguments["query"] != nil {
			decoder := json.NewDecoder(bytes.NewReader(body))
			decoder.DisallowUnknownFields()
			if err = decoder.Decode(&q); err != nil {
				return nil, err
			}
		}
		current, err := preview.Load(config.Folder, call.Variant)
		if err != nil {
			return nil, err
		}
		result, err := current.ExecuteFixture(ids[call.Tool], q, call.Body)
		if err != nil {
			return nil, err
		}
		return result.Response, nil
	}})
}
