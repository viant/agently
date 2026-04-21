package agently

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	toolschema "github.com/viant/agently-core/protocol/tool/schema"
	"github.com/viant/agently-core/sdk"
)

type MCPListCmd struct {
	Name         string `short:"n" long:"name" description:"Exact tool name to show full definition"`
	Service      string `short:"s" long:"service" description:"Filter tools by service/prefix namespace"`
	API          string `long:"api" description:"Server URL (skip local auto-detect)"`
	Token        string `long:"token" description:"Bearer token for API requests (overrides AGENTLY_TOKEN)"`
	Session      string `long:"session" description:"Session cookie value for API requests (agently_session)"`
	OOB          string `long:"oob" description:"Use local scy OAuth2 out-of-band login with the supplied secrets URL"`
	OAuthCfg     string `long:"oauth-config" description:"Optional scy OAuth config URL override for client-side OOB login"`
	OAuthScp     string `long:"oauth-scopes" description:"comma-separated OAuth scopes for OOB login"`
	JSON         bool   `long:"json" description:"Print result as JSON instead of plain text"`
	Example      bool   `long:"example" description:"Include an example request derived from the tool input schema"`
	Schema       bool   `long:"schema" description:"Include input schema in plain-text output"`
	SchemaFormat string `long:"schema-format" choice:"go" choice:"json" default:"go" description:"Schema rendering format for plain-text --schema output"`
}

type mcpToolView struct {
	Name           string                 `json:"name"`
	Service        string                 `json:"service,omitempty"`
	Description    string                 `json:"description,omitempty"`
	InputSchema    map[string]interface{} `json:"inputSchema,omitempty"`
	Required       []string               `json:"required,omitempty"`
	OutputSchema   map[string]interface{} `json:"outputSchema,omitempty"`
	Cacheable      bool                   `json:"cacheable,omitempty"`
	ExampleRequest interface{}            `json:"exampleRequest,omitempty"`
}

func (c *MCPListCmd) Execute(_ []string) error {
	ctx := context.Background()

	baseURL, err := resolveToolBaseURL(ctx, strings.TrimSpace(c.API))
	if err != nil {
		return fmt.Errorf("cannot find agently server: %w", err)
	}
	providers, _ := fetchAuthProviders(ctx, baseURL)

	httpClient := &http.Client{Jar: cliCookieJar()}
	opts := []sdk.HTTPOption{sdk.WithHTTPClient(httpClient)}
	if token := resolvedToken(c.Token); token != "" {
		opts = append(opts, sdk.WithAuthToken(token))
	}
	client, err := sdk.NewHTTP(baseURL, opts...)
	if err != nil {
		return fmt.Errorf("sdk client: %w", err)
	}
	if err := ensureToolAuth(ctx, client, providers, c.Token, c.Session, c.OOB, c.OAuthCfg, c.OAuthScp); err != nil {
		return err
	}

	defs, err := client.ListToolDefinitions(ctx)
	if err != nil {
		return fmt.Errorf("list mcp tools: %w", err)
	}
	defs = filterToolDefinitions(defs, strings.TrimSpace(c.Service))

	if c.Name != "" {
		for _, d := range defs {
			if d.Name == c.Name {
				return c.printTool(toolViewFromDefinition(d, c.Example))
			}
		}
		return fmt.Errorf("tool %q not found", c.Name)
	}

	views := make([]mcpToolView, 0, len(defs))
	for _, def := range defs {
		views = append(views, toolViewFromDefinition(def, c.Example))
	}
	sort.Slice(views, func(i, j int) bool { return views[i].Name < views[j].Name })

	if c.JSON {
		data, _ := json.MarshalIndent(views, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	if len(views) == 0 {
		fmt.Println("no tools registered")
		return nil
	}

	for _, view := range views {
		if !c.Example && !c.Schema {
			fmt.Printf("%s\t%s\n", view.Name, view.Description)
			continue
		}
		example := "{}"
		if view.ExampleRequest != nil {
			if data, err := json.MarshalIndent(view.ExampleRequest, "", "  "); err == nil {
				example = string(data)
			}
		}
		inputSchema := "{}"
		if c.Schema && len(view.InputSchema) > 0 {
			inputSchema = renderSchema(view.InputSchema, c.SchemaFormat)
		}
		outputSchema := "{}"
		if c.Schema && len(view.OutputSchema) > 0 {
			outputSchema = renderSchema(view.OutputSchema, c.SchemaFormat)
		}
		fmt.Printf("Name         : %s\n", view.Name)
		if view.Service != "" {
			fmt.Printf("Service      : %s\n", view.Service)
		}
		fmt.Printf("Description  : %s\n", view.Description)
		if c.Example {
			fmt.Printf("Example Request:\n%s\n", indentBlock(example, "  "))
		}
		if c.Schema {
			fmt.Printf("Input Schema:\n%s\n", indentBlock(inputSchema, "  "))
			fmt.Printf("Output Schema:\n%s\n", indentBlock(outputSchema, "  "))
		}
		fmt.Println()
	}
	return nil
}

func (c *MCPListCmd) printTool(view mcpToolView) error {
	if c.JSON {
		data, _ := json.MarshalIndent(view, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	fmt.Printf("Name         : %s\n", view.Name)
	if view.Service != "" {
		fmt.Printf("Service      : %s\n", view.Service)
	}
	fmt.Printf("Description  : %s\n", view.Description)
	if len(view.InputSchema) > 0 {
		fmt.Printf("Input Schema:\n%s\n", indentBlock(renderSchema(view.InputSchema, c.SchemaFormat), "  "))
	}
	if len(view.Required) > 0 {
		fmt.Printf("Required     : %v\n", view.Required)
	}
	if len(view.OutputSchema) > 0 {
		fmt.Printf("Output Schema:\n%s\n", indentBlock(renderSchema(view.OutputSchema, c.SchemaFormat), "  "))
	}
	if c.Example {
		if data, err := json.MarshalIndent(view.ExampleRequest, "", "  "); err == nil {
			fmt.Printf("Example Request:\n%s\n", indentBlock(string(data), "  "))
		}
	}
	return nil
}

func toolViewFromDefinition(def sdk.ToolDefinitionInfo, includeExample bool) mcpToolView {
	view := mcpToolView{
		Name:         def.Name,
		Service:      toolServiceNamespace(def.Name),
		Description:  def.Description,
		InputSchema:  def.Parameters,
		Required:     def.Required,
		OutputSchema: def.OutputSchema,
		Cacheable:    def.Cacheable,
	}
	if includeExample {
		view.ExampleRequest = buildExampleRequest(def)
	}
	return view
}

func buildExampleRequest(def sdk.ToolDefinitionInfo) interface{} {
	if len(def.Parameters) == 0 {
		return map[string]interface{}{}
	}
	schema := cloneSchemaMap(def.Parameters)
	if _, ok := schema["required"]; !ok && len(def.Required) > 0 {
		required := make([]interface{}, 0, len(def.Required))
		for _, item := range def.Required {
			required = append(required, item)
		}
		schema["required"] = required
	}
	return buildSchemaExample(schema, false)
}

func buildSchemaExample(schema interface{}, requiredOnly bool) interface{} {
	s, ok := schema.(map[string]interface{})
	if !ok || len(s) == 0 {
		return map[string]interface{}{}
	}
	if value, ok := explicitSchemaExample(s); ok {
		return value
	}

	typ := strings.ToLower(strings.TrimSpace(asString(s["type"])))
	if typ == "" {
		if _, ok := s["properties"].(map[string]interface{}); ok {
			typ = "object"
		} else if s["items"] != nil {
			typ = "array"
		}
	}

	switch typ {
	case "object":
		props, _ := s["properties"].(map[string]interface{})
		if len(props) == 0 {
			return map[string]interface{}{}
		}
		required := requiredSet(s["required"])
		result := map[string]interface{}{}
		keys := sortedKeys(props)
		for _, key := range keys {
			prop := props[key]
			if requiredOnly {
				if _, ok := required[key]; !ok && !hasInformativeSchemaValue(prop) {
					continue
				}
			}
			result[key] = buildSchemaExample(prop, requiredOnly)
		}
		return result
	case "array":
		if items := s["items"]; items != nil {
			return []interface{}{buildSchemaExample(items, requiredOnly)}
		}
		return []interface{}{}
	case "integer", "number":
		return 1
	case "boolean":
		return false
	case "string":
		if format, ok := s["format"].(string); ok {
			switch strings.ToLower(strings.TrimSpace(format)) {
			case "date-time":
				return "2026-01-01T00:00:00Z"
			case "date":
				return "2026-01-01"
			case "uri", "url":
				return "https://example.com"
			case "email":
				return "user@example.com"
			}
		}
		return " "
	default:
		if _, ok := s["enum"].([]interface{}); ok {
			if value, ok := explicitSchemaExample(s); ok {
				return value
			}
		}
		return ""
	}
}

func explicitSchemaExample(schema map[string]interface{}) (interface{}, bool) {
	for _, key := range []string{"example", "default"} {
		if value, ok := schema[key]; ok {
			return value, true
		}
	}
	if raw, ok := schema["examples"]; ok {
		switch actual := raw.(type) {
		case []interface{}:
			if len(actual) > 0 {
				return actual[0], true
			}
		case []string:
			if len(actual) > 0 {
				return actual[0], true
			}
		}
	}
	if raw, ok := schema["enum"]; ok {
		switch actual := raw.(type) {
		case []interface{}:
			if len(actual) > 0 {
				return actual[0], true
			}
		case []string:
			if len(actual) > 0 {
				return actual[0], true
			}
		}
	}
	return nil, false
}

func hasInformativeSchemaValue(schema interface{}) bool {
	s, ok := schema.(map[string]interface{})
	if !ok {
		return false
	}
	_, ok = explicitSchemaExample(s)
	return ok
}

func requiredSet(raw interface{}) map[string]struct{} {
	result := map[string]struct{}{}
	switch actual := raw.(type) {
	case []interface{}:
		for _, item := range actual {
			if text := strings.TrimSpace(asString(item)); text != "" {
				result[text] = struct{}{}
			}
		}
	case []string:
		for _, item := range actual {
			if text := strings.TrimSpace(item); text != "" {
				result[text] = struct{}{}
			}
		}
	}
	return result
}

func sortedKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func cloneSchemaMap(src map[string]interface{}) map[string]interface{} {
	if len(src) == 0 {
		return map[string]interface{}{}
	}
	dst := make(map[string]interface{}, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func asString(value interface{}) string {
	switch actual := value.(type) {
	case string:
		return actual
	default:
		return fmt.Sprintf("%v", actual)
	}
}

func renderSchema(schema map[string]interface{}, format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "go":
		if shape, err := toolschema.GoShapeFromSchemaMap(schema); err == nil && strings.TrimSpace(shape) != "" {
			return shape
		}
		return schemaToGoShape(schema)
	case "json":
		data, err := json.Marshal(schema)
		if err != nil {
			return "{}"
		}
		return string(data)
	default:
		data, err := json.Marshal(schema)
		if err != nil {
			return "{}"
		}
		return string(data)
	}
}

func indentBlock(value, prefix string) string {
	value = strings.TrimRight(value, "\n")
	if value == "" {
		return prefix + "{}"
	}
	lines := strings.Split(value, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}

func schemaToGoShape(schema interface{}) string {
	return schemaToGoType(schema) + "{}"
}

func schemaToGoType(schema interface{}) string {
	s, ok := schema.(map[string]interface{})
	if !ok || len(s) == 0 {
		return "interface{}"
	}
	if raw, ok := explicitSchemaExample(s); ok {
		return goTypeForValue(raw)
	}
	typ := strings.ToLower(strings.TrimSpace(asString(s["type"])))
	if typ == "" {
		if _, ok := s["properties"].(map[string]interface{}); ok {
			typ = "object"
		} else if s["items"] != nil {
			typ = "array"
		}
	}
	switch typ {
	case "object":
		props, _ := s["properties"].(map[string]interface{})
		if len(props) == 0 {
			return "map[string]interface{}"
		}
		keys := sortedKeys(props)
		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			fieldName := goFieldName(key)
			fieldType := schemaToGoType(props[key])
			parts = append(parts, fieldName+" "+fieldType)
		}
		return "struct { " + strings.Join(parts, "; ") + " }"
	case "array":
		items := s["items"]
		return "[]" + schemaToGoType(items)
	case "integer":
		return "int"
	case "number":
		return "float64"
	case "boolean":
		return "bool"
	case "string":
		return "string"
	default:
		return "interface{}"
	}
}

func goTypeForValue(value interface{}) string {
	switch actual := value.(type) {
	case nil:
		return "interface{}"
	case bool:
		return "bool"
	case float64:
		if actual == float64(int64(actual)) {
			return "int"
		}
		return "float64"
	case float32:
		return "float64"
	case int, int8, int16, int32, int64:
		return "int"
	case uint, uint8, uint16, uint32, uint64:
		return "int"
	case string:
		return "string"
	case []interface{}:
		if len(actual) == 0 {
			return "[]interface{}"
		}
		return "[]" + goTypeForValue(actual[0])
	case []string:
		return "[]string"
	case map[string]interface{}:
		return schemaToGoType(map[string]interface{}{"type": "object", "properties": actual})
	default:
		return "interface{}"
	}
}

var nonIdentifierChars = regexp.MustCompile(`[^a-zA-Z0-9]+`)

func goFieldName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "Field"
	}
	parts := nonIdentifierChars.Split(name, -1)
	var builder strings.Builder
	for _, part := range parts {
		if part == "" {
			continue
		}
		runes := []rune(strings.ToLower(part))
		runes[0] = unicode.ToUpper(runes[0])
		builder.WriteString(string(runes))
	}
	result := builder.String()
	if result == "" {
		result = "Field"
	}
	if _, err := strconv.Atoi(string(result[0])); err == nil {
		result = "Field" + result
	}
	return result
}
