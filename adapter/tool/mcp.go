package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	mcppkg "github.com/viant/agently/genai/adapter/mcp"
	"github.com/viant/agently/genai/tool"
	"github.com/viant/fluxor/model/types"
	mcpSchema "github.com/viant/mcp-protocol/schema"
	mcpclient "github.com/viant/mcp/client"
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

// RegisterMCPServer creates a Fluxor service for the given MCP server and adds
// all its tools to the LLM registry.  The returned service must be registered
// in runtime actions by the caller.
func RegisterMCPServer(ctx context.Context, alias string, client mcpclient.Interface, registry *tool.Registry) (*MCPService, error) {
	if client == nil {
		return nil, fmt.Errorf("mcp client is nil")
	}

	if _, err := client.Initialize(ctx); err != nil {
		return nil, fmt.Errorf("mcp init: %w", err)
	}

	svc := &MCPService{
		name:    "mcp/" + strings.Trim(alias, "/"),
		client:  client,
		toolMap: map[string]mcpSchema.Tool{},
	}

	var cursor *string
	for {
		list, err := client.ListTools(ctx, cursor)
		if err != nil {
			return nil, fmt.Errorf("list tools: %w", err)
		}

		for _, td := range list.Tools {
			svc.toolMap[td.Name] = td

			if registry != nil {
				def := mcppkg.MapTool(td, alias+"_")
				// capture copy for closure
				toolCopy := td
				handler := func(ctx context.Context, args map[string]interface{}) (string, error) {
					if args == nil {
						args = map[string]interface{}{}
					}
					params := &mcpSchema.CallToolRequestParams{
						Name:      toolCopy.Name,
						Arguments: mcpSchema.CallToolRequestParamsArguments(args),
					}
					res, err := client.CallTool(ctx, params)
					if err != nil {
						return "", err
					}
					if len(res.Content) == 0 {
						return "", nil
					}
					if len(res.Content) == 1 && res.Content[0].Type == "text" {
						return res.Content[0].Text, nil
					}
					data, _ := json.Marshal(res.Content)
					return string(data), nil
				}

				registry.Register(def, handler)
			}
		}

		if list.NextCursor == nil || *list.NextCursor == "" {
			break
		}
		cursor = list.NextCursor
	}

	return svc, nil
}
