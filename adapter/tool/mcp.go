package tool

import (
	"context"
	"encoding/json"
	"github.com/viant/fluxor/model/types"
	mcpSchema "github.com/viant/mcp-protocol/schema"
	mcpclient "github.com/viant/mcp/client"
	"reflect"
)

// MCPService exposes every tool of a specific MCP server as a Fluxor service
// and registers the same tools in the provided LLM tool registry.
type MCPService struct {
	name    string
	client  mcpclient.Interface
	toolMap map[string]mcpSchema.Tool
}

// ensure interface contract
var _ types.Service = (*MCPService)(nil)

func (s *MCPService) Name() string { return s.name }
func (s *MCPService) Methods() types.Signatures { // one signature per tool
	sigs := make(types.Signatures, 0, len(s.toolMap))
	for n := range s.toolMap {
		sigs = append(sigs, types.Signature{
			Name:   n,
			Input:  reflect.TypeOf(&MCPCallInput{}),
			Output: reflect.TypeOf(&MCPCallOutput{}),
		})
	}
	return sigs
}

func (s *MCPService) Method(name string) (types.Executable, error) {
	t, ok := s.toolMap[name]
	if !ok {
		return nil, types.NewMethodNotFoundError(name)
	}

	exec := func(ctx context.Context, in, out interface{}) error {
		input, ok := in.(*MCPCallInput)
		if !ok {
			return types.NewInvalidInputError(in)
		}
		output, ok := out.(*MCPCallOutput)
		if !ok {
			return types.NewInvalidOutputError(out)
		}

		if input.Args == nil {
			input.Args = map[string]interface{}{}
		}

		params := &mcpSchema.CallToolRequestParams{
			Name:      t.Name,
			Arguments: mcpSchema.CallToolRequestParamsArguments(input.Args),
		}

		result, err := s.client.CallTool(ctx, params)
		if err != nil {
			return err
		}

		if len(result.Content) == 0 {
			return nil // nothing to return
		}
		if len(result.Content) == 1 && result.Content[0].Type == "text" {
			output.Content = result.Content[0].Text
			return nil
		}

		data, err := json.Marshal(result.Content)
		if err != nil {
			return err
		}
		output.Content = string(data)
		return nil
	}

	return exec, nil
}

// MCPCallInput is the generic input passed to any MCP tool.
type MCPCallInput struct {
	Args map[string]interface{} `json:"args,omitempty"`
}

// MCPCallOutput is the generic output.
type MCPCallOutput struct {
	Content string `json:"content,omitempty"`
}
