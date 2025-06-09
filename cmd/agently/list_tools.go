package agently

import (
	"encoding/json"
	"fmt"
	tool2 "github.com/viant/fluxor-mcp/mcp/tool"
	"sort"

	"github.com/viant/agently/genai/llm"
)

// ListToolsCmd prints all registered tools (service/method) with optional description.
// ListToolsCmd prints all registered tools or details for a single tool.
//
// Without any flags it prints a table containing tool name and description.
//
// When -n/--name is supplied it looks up that specific tool and prints its
// definition.  Use --json to emit the definition as prettified JSON rather than
// a human-readable format.  When --json is provided without --name the full
// catalogue is printed as JSON.
type ListToolsCmd struct {
	Name string `short:"n" long:"name" description:"Tool name (service_method) to show full definition"`
	JSON bool   `long:"json" description:"Print result as JSON instead of table/plain text"`
}

func (c *ListToolsCmd) Execute(_ []string) error {
	// Initialise executor & obtain tool definitions in one go.
	svc := executorSingleton()

	if c.Name != "" {
		canonical := tool2.Canonical(c.Name)
		tool, err := svc.Orchestration().LookupTool(canonical)
		if err != nil {
			return fmt.Errorf("tool %q not found", c.Name)
		}
		llmTool := llm.ToolDefinitionFromMcpTool(&tool.Metadata)
		return c.printToolDefinition(llmTool)
	}

	defs := svc.LLMCore().ToolDefinitions()

	if len(defs) == 0 {
		fmt.Println("no tools registered")
		return nil
	}

	// ------------------------------------------------------------------
	// Narrow down to specific tool (if requested)
	// ------------------------------------------------------------------
	if c.Name != "" {
		for _, d := range defs {
			if d.Name == c.Name {
				return c.printToolDefinition(&d)
			}
		}
		return fmt.Errorf("tool %q not found", c.Name)
	}
	// ------------------------------------------------------------------
	// Narrow down to specific tool (if requested)
	// ------------------------------------------------------------------
	if c.Name != "" {
		for _, d := range defs {
			if d.Name == c.Name {
				return c.printToolDefinition(&d)
			}
		}
		return fmt.Errorf("tool %q not found", c.Name)
	}

	// ------------------------------------------------------------------
	// Full catalogue (list or JSON)
	// ------------------------------------------------------------------
	if c.JSON {
		data, _ := json.MarshalIndent(defs, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	sort.Slice(defs, func(i, j int) bool { return defs[i].Name < defs[j].Name })
	for _, d := range defs {
		fmt.Printf("%s\t%s\n", d.Name, d.Description)
	}
	return nil
}

// printToolDefinition dumps a single tool definition either as JSON or a compact
// human-readable representation.
func (c *ListToolsCmd) printToolDefinition(def *llm.ToolDefinition) error {
	if c.JSON {
		data, _ := json.MarshalIndent(def, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	// Plain format
	fmt.Printf("Name        : %s\n", def.Name)
	fmt.Printf("Description : %s\n", def.Description)
	if len(def.Parameters) > 0 {
		if data, err := json.MarshalIndent(def.Parameters, "", "  "); err == nil {
			fmt.Printf("Parameters  : %s\n", string(data))
		}
	}
	if len(def.Required) > 0 {
		fmt.Printf("Required    : %v\n", def.Required)
	}
	if len(def.OutputSchema) > 0 {
		if data, err := json.MarshalIndent(def.OutputSchema, "", "  "); err == nil {
			fmt.Printf("OutputSchema: %s\n", string(data))
		}
	}
	return nil
}
