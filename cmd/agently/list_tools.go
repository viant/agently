package agently

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/viant/agently-core/sdk"
)

// ListToolsCmd prints all registered tools (service/method) with optional description.
//
// Without any flags it prints a table containing tool name and description.
//
// When -n/--name is supplied it looks up that specific tool and prints its
// definition.  Use --json to emit the definition as prettified JSON rather than
// a human-readable format.  When --json is provided without --name the full
// catalogue is printed as JSON.
type ListToolsCmd struct {
	Name    string `short:"n" long:"name" description:"Exact tool name to show full definition"`
	Service string `short:"s" long:"service" description:"Filter tools by service/prefix namespace"`
	API     string `long:"api" description:"Server URL (skip local auto-detect)"`
	JSON    bool   `long:"json" description:"Print result as JSON instead of table/plain text"`
}

func (c *ListToolsCmd) Execute(_ []string) error {
	ctx := context.Background()

	baseURL, err := resolveToolBaseURL(ctx, strings.TrimSpace(c.API))
	if err != nil {
		return fmt.Errorf("cannot find agently server: %w", err)
	}

	httpClient := &http.Client{Jar: cliCookieJar()}
	opts := []sdk.HTTPOption{sdk.WithHTTPClient(httpClient)}
	client, err := sdk.NewHTTP(baseURL, opts...)
	if err != nil {
		return fmt.Errorf("sdk client: %w", err)
	}

	defs, err := client.ListToolDefinitions(ctx)
	if err != nil {
		return fmt.Errorf("list tools: %w", err)
	}
	defs = filterToolDefinitions(defs, strings.TrimSpace(c.Service))

	// Name-specific output
	if c.Name != "" {
		for _, d := range defs {
			if d.Name == c.Name {
				return c.printToolDefinition(&d)
			}
		}
		return fmt.Errorf("tool %q not found", c.Name)
	}

	// Full catalogue (list or JSON)
	if c.JSON {
		data, _ := json.MarshalIndent(defs, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	if len(defs) == 0 {
		fmt.Println("no tools registered")
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
func (c *ListToolsCmd) printToolDefinition(def *sdk.ToolDefinitionInfo) error {
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

func resolveToolBaseURL(ctx context.Context, api string) (string, error) {
	if strings.TrimSpace(api) != "" {
		return strings.TrimSpace(api), nil
	}
	instances, err := detectLocalInstances(ctx)
	if err == nil {
		for _, inst := range instances {
			if strings.TrimSpace(inst.BaseURL) != "" {
				return inst.BaseURL, nil
			}
		}
	}
	return "", fmt.Errorf("no running agently server found; start one with 'agently serve' or use --api")
}

func filterToolDefinitions(defs []sdk.ToolDefinitionInfo, service string) []sdk.ToolDefinitionInfo {
	service = normalizeToolNamespace(service)
	if service == "" {
		return defs
	}
	filtered := make([]sdk.ToolDefinitionInfo, 0, len(defs))
	for _, def := range defs {
		if toolMatchesNamespace(def.Name, service) {
			filtered = append(filtered, def)
		}
	}
	return filtered
}

func toolMatchesNamespace(toolName, service string) bool {
	if service == "" {
		return true
	}
	return normalizeToolNamespace(toolServiceNamespace(toolName)) == service
}

func toolServiceNamespace(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if idx := strings.Index(name, ":"); idx != -1 {
		return strings.TrimSpace(name[:idx])
	}
	if idx := strings.LastIndex(name, "."); idx != -1 {
		return strings.TrimSpace(name[:idx])
	}
	if idx := strings.LastIndex(name, "/"); idx != -1 {
		return strings.TrimSpace(name[:idx])
	}
	return name
}

func normalizeToolNamespace(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	value = strings.ReplaceAll(value, "_", "/")
	if idx := strings.Index(value, ":"); idx != -1 {
		value = strings.TrimSpace(value[:idx])
	}
	if idx := strings.LastIndex(value, "."); idx != -1 {
		value = strings.TrimSpace(value[:idx])
	}
	return value
}
