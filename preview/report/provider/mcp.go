package preview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

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
		body, err := client.Call(ctx, tool, map[string]any{"query": q, "variant": p.Variant})
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
