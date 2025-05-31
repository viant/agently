package mcp

import (
	"context"
	"encoding/json"

	"github.com/viant/agently/genai/llm"
	agtool "github.com/viant/agently/genai/tool"
	mcpSchema "github.com/viant/mcp-protocol/schema"
	mcpclient "github.com/viant/mcp/client"
)

// MapTool converts an MCP schema.Tool to an llm.ToolDefinition.
func MapTool(tool mcpSchema.Tool, prefix string) llm.ToolDefinition {
	// Convert schema specific Properties type to a plain map[string]map[string]interface{}
	convertedProps := make(map[string]map[string]interface{}, len(tool.InputSchema.Properties))
	for k, v := range tool.InputSchema.Properties {
		convertedProps[k] = v
	}

	def := llm.ToolDefinition{
		Name:        prefix + tool.Name,
		Description: "",
		Parameters: map[string]interface{}{
			"type":       "object",
			"properties": convertedProps,
		},
		Required: tool.InputSchema.Required,
	}
	if tool.Description != nil {
		def.Description = *tool.Description
	}
	return def
}

// RegisterTools fetches tools from an MCP server and registers them with the tool registry.
func RegisterTools(ctx context.Context, client mcpclient.Interface, prefix string) error {
	// Initialize the MCP client
	if _, err := client.Initialize(ctx); err != nil {
		return err
	}
	var cursor *string
	for {
		result, err := client.ListTools(ctx, cursor)
		if err != nil {
			return err
		}
		for _, t := range result.Tools {
			toolVar := t
			def := MapTool(toolVar, prefix)
			handler := func(ctx context.Context, args map[string]interface{}) (string, error) {
				params := &mcpSchema.CallToolRequestParams{
					Name:      toolVar.Name,
					Arguments: mcpSchema.CallToolRequestParamsArguments(args),
				}
				callResult, err := client.CallTool(ctx, params)
				if err != nil {
					return "", err
				}
				// No content
				if len(callResult.Content) == 0 {
					return "", nil
				}
				// Only plain text
				if len(callResult.Content) == 1 && callResult.Content[0].Type == "text" {
					return callResult.Content[0].Text, nil
				}
				// Return full content array as JSON for non-text or multiple parts
				data, err := json.Marshal(callResult.Content)
				if err != nil {
					return "", err
				}
				return string(data), nil
			}
			agtool.Register(def, handler)
		}
		// Continue paging if next cursor is provided
		if result.NextCursor == nil || *result.NextCursor == "" {
			break
		}
		cursor = result.NextCursor
	}
	return nil
}
