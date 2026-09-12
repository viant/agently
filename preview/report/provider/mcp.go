package preview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/viant/agently/preview/datasource"
)

// UseMCP makes every subsequent Execute (including compilation and export page
// draining) call the declared MCP service. There is no filesystem fallback.
func (p *Package) UseMCP(ctx context.Context, hostURL string) error {
	for id, source := range p.Sources {
		if _, _, err := datasource.Resolve(source.Service, p.Endpoints, hostURL); err != nil {
			return fmt.Errorf("datasource %s: %w", id, err)
		}
	}
	p.executeRemote = func(id string, q Query) (*Result, error) {
		source, ok := p.Sources[id]
		if !ok {
			return nil, fail("MissingDataSource", id, "", "unknown datasource")
		}
		endpoint, tool, err := datasource.Resolve(source.Service, p.Endpoints, hostURL)
		if err != nil {
			return nil, err
		}
		client, err := datasource.Dial(ctx, endpoint)
		if err != nil {
			return nil, fmt.Errorf("MCP datasource %s connect: %w", id, err)
		}
		defer client.Close()
		// A report contract may use one public tool for all result sets. Preserve
		// the internal datasource identity in the query envelope so the replay
		// adapter can route the Forge request without exposing datasource tool
		// names as part of the public contract.
		q.DataSource = id
		arguments := map[string]any{"query": q, "variant": p.Variant}
		if source.MCPRequest != nil {
			arguments, err = mappedMCPRequest(source.MCPRequest, q)
			if err != nil {
				return nil, fmt.Errorf("MCP datasource %s request: %w", id, err)
			}
		}
		body, err := client.Call(ctx, tool, arguments)
		if err != nil {
			var remote *datasource.RemoteError
			if errors.As(err, &remote) {
				var d Diagnostic
				if json.Unmarshal(remote.Body, &d) == nil && d.Code != "" {
					return nil, &d
				}
			}
			return nil, fmt.Errorf("MCP datasource %s: %w", id, err)
		}
		return p.decodeMCPResponse(id, q, body)
	}
	return nil
}

func mappedMCPRequest(contract *MCPRequest, q Query) (map[string]any, error) {
	arguments := clone(contract.Arguments)
	if arguments == nil {
		arguments = Object{}
	}
	path := strings.TrimSpace(contract.RequestPath)
	if path == "" {
		path = "request"
	}
	request, err := ensureObjectPath(arguments, path)
	if err != nil {
		return nil, err
	}
	if q.Projection != nil {
		request["dimensions"] = fieldNames(q.Projection.Dimensions)
		request["measures"] = fieldNames(q.Projection.Measures)
		if len(q.Projection.Fields) > 0 {
			request["dimensions"] = fieldNames(q.Projection.Fields)
		}
	}
	if len(q.OrderBy) > 0 {
		values := make([]string, 0, len(q.OrderBy))
		for _, orderlz := range q.OrderBy {
			values = append(values, strings.TrimSpace(orderlz.Field+" "+orderlz.Direction))
		}
		request["orderBy"] = values
	}
	if q.Page != nil {
		request["limit"], request["offset"] = q.Page.Limit, q.Page.Offset
	}
	if q.Filter != nil {
		if err = applyMCPFilter(request, *q.Filter); err != nil {
			return nil, err
		}
	}
	return arguments, nil
}

func fieldNames(fields []Field) []string {
	result := make([]string, 0, len(fields))
	for _, field := range fields {
		result = append(result, field.Field)
	}
	return result
}

func ensureObjectPath(root Object, path string) (Object, error) {
	current := root
	for _, part := range strings.Split(path, ".") {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, fmt.Errorf("empty request path segment")
		}
		if next, ok := current[part].(map[string]any); ok {
			current = next
			continue
		}
		if current[part] != nil {
			return nil, fmt.Errorf("request path %q is not an object", path)
		}
		next := Object{}
		current[part] = next
		current = next
	}
	return current, nil
}

func applyMCPFilter(request Object, filter Predicate) error {
	if filter.Not != nil || len(filter.Or) > 0 {
		return fmt.Errorf("public MCP request mapping supports conjunctive parameter filters only")
	}
	for _, child := range filter.And {
		if err := applyMCPFilter(request, child); err != nil {
			return err
		}
	}
	if strings.TrimSpace(filter.Field) == "" {
		return nil
	}
	if filter.Op != "" && filter.Op != "eq" && filter.Op != "in" {
		return fmt.Errorf("parameter filter %q uses unsupported operator %q", filter.Field, filter.Op)
	}
	parts := strings.Split(filter.Field, ".")
	name := parts[len(parts)-1]
	target := request
	if len(parts) > 1 {
		var err error
		target, err = ensureObjectPath(request, strings.Join(parts[:len(parts)-1], "."))
		if err != nil {
			return err
		}
	}
	target[name] = filter.Value
	return nil
}

// ExecuteFixture is the report query transform for the shared mock MCP server.
func (p *Package) ExecuteFixture(id string, q Query, body json.RawMessage) (*Result, error) {
	d, ok := p.Extension.DataSources[id]
	if !ok {
		return nil, fail("MissingDataSource", id, "", "unknown fixture")
	}
	f, err := decodeFixture(id, body, d, p.Sources[id])
	if err != nil {
		return nil, err
	}
	copy := *p
	copy.executeRemote = nil
	copy.fixtures = map[string]*fixture{}
	for key, v := range p.fixtures {
		copy.fixtures[key] = v
	}
	copy.fixtures[id] = f
	return copy.Execute(id, q)
}
func (p *Package) decodeMCPResponse(id string, q Query, body json.RawMessage) (*Result, error) {
	source := p.Sources[id]
	d := p.Extension.DataSources[id]
	d.InputContract = source.ResultContract
	d.Dimensions = nil
	d.Measures = nil
	if source.ResultContract.ResultName != "" {
		d.ResultName = source.ResultContract.ResultName
	}
	if q.Projection != nil {
		columns := []Object{}
		fields := append(append(append([]Field{}, q.Projection.Fields...), q.Projection.Dimensions...), q.Projection.Measures...)
		for _, field := range fields {
			var c Object
			for _, candidate := range source.Columns {
				if candidate["name"] == field.Field {
					c = clone(candidate)
					break
				}
			}
			if c == nil {
				return nil, fail("UnknownColumn", id, field.Field, "MCP projection references undeclared column")
			}
			c["name"] = field.name()
			if field.Aggregation != "" && field.Aggregation != "none" {
				c["nullable"] = true
			}
			if field.Aggregation == "count" || field.Aggregation == "countDistinct" {
				c["nullable"] = false
				c["type"] = "integer"
			}
			if field.Aggregation == "average" {
				c["type"] = "number"
			}
			columns = append(columns, c)
		}
		source.Columns = columns
	}
	// Tabular output is self-describing; records retain the declared result schema.
	var envelope Object
	_ = json.Unmarshal(body, &envelope)
	if d.InputContract.Shape == "tabular" && envelope["status"] != "error" {
		source.Columns = nil
	}
	f, err := decodeFixture(id, body, d, source)
	if err != nil {
		return nil, err
	}
	request, _ := json.Marshal(q)
	return &Result{Response: f.raw, Rows: f.rows, Columns: f.columns, HasMore: f.hasMore, Fingerprint: hash(append(body, request...))}, nil
}
