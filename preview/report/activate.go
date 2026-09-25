package reportpreview

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/viant/agently-core/service/reportdefinition"
	"github.com/viant/agently/preview/datasource"
	forgehtml "github.com/viant/forge/backend/reporting/export/html"
	forgepdf "github.com/viant/forge/backend/reporting/export/pdf"
	reportprint "github.com/viant/forge/backend/reporting/print"
)

const defaultActivateTool = "aistudio_reports_activate"

// activateCaller is the transport boundary; tests can supply a caller without
// changing the CLI's decoding and rendering path.
type activateCaller func(context.Context, string, string, map[string]any) (json.RawMessage, error)

func callActivatedMCP(ctx context.Context, endpoint, tool string, args map[string]any) (json.RawMessage, error) {
	client, err := datasource.Dial(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	return client.Call(ctx, tool, args)
}

func runActivateCLI(ctx context.Context, args []string, out, errOut io.Writer, call activateCaller) error {
	fs := flag.NewFlagSet("report-preview activate", flag.ContinueOnError)
	fs.SetOutput(errOut)
	endpoint := fs.String("mcp-url", "", "MCP streamable endpoint URL")
	tool := fs.String("tool", defaultActivateTool, "MCP activation tool name")
	groupID := fs.String("group-id", "", "report group identity")
	reportID := fs.String("report-id", "", "report identity")
	contextFile := fs.String("context", "", "context JSON object file")
	parametersFile := fs.String("parameters", "", "parameters JSON object file")
	limit := fs.Int("limit", 1000, "maximum rows per dataset")
	output := fs.String("out", "", "JSON, HTML, or PDF output file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("activate does not take a fixture folder")
	}
	if strings.TrimSpace(*endpoint) == "" || strings.TrimSpace(*tool) == "" || strings.TrimSpace(*groupID) == "" || strings.TrimSpace(*reportID) == "" {
		return fmt.Errorf("activate requires --mcp-url, --group-id, --report-id and a nonempty --tool")
	}
	if *limit < 1 || *limit > 1000 {
		return fmt.Errorf("--limit must be 1..1000")
	}
	contextValue, err := readActivateObject(*contextFile)
	if err != nil {
		return fmt.Errorf("--context: %w", err)
	}
	if err := validateActivationContext(contextValue); err != nil {
		return fmt.Errorf("--context: %w", err)
	}
	parameters, err := readActivateObject(*parametersFile)
	if err != nil {
		return fmt.Errorf("--parameters: %w", err)
	}
	body, err := call(ctx, *endpoint, *tool, map[string]any{
		"groupId": *groupID, "reportId": *reportID,
		"context": contextValue, "parameters": parameters, "limit": *limit,
	})
	if err != nil {
		return fmt.Errorf("activate MCP tool %q: %w", *tool, err)
	}
	activated, err := reportdefinition.DecodeActivatedMCPReport(body, *groupID, *reportID)
	if err != nil {
		return err
	}
	var rendered []byte
	switch strings.ToLower(filepath.Ext(*output)) {
	case ".html", ".pdf":
		print, err := reportprint.DecodeJSON(activated.ReportPrint)
		if err != nil {
			return err
		}
		if strings.EqualFold(filepath.Ext(*output), ".html") {
			rendered, err = forgehtml.Render(print)
		} else {
			var result *forgepdf.RenderResult
			result, err = forgepdf.Render(print, forgepdf.Options{ReportSpec: activated.ReportSpec})
			if err == nil {
				rendered = result.Bytes
			}
		}
		if err != nil {
			return err
		}
	default:
		rendered, err = json.MarshalIndent(activated, "", "  ")
		if err != nil {
			return err
		}
		rendered = append(rendered, '\n')
	}
	if *output != "" {
		if err := os.WriteFile(*output, rendered, 0600); err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, *output)
		return err
	}
	_, err = out.Write(rendered)
	return err
}

func readActivateObject(path string) (map[string]any, error) {
	if path == "" {
		return map[string]any{}, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, (64<<10)+1))
	if err != nil {
		return nil, err
	}
	if len(body) > 64<<10 {
		return nil, fmt.Errorf("JSON object file exceeds 64 KiB")
	}
	var value map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if value == nil {
		return nil, fmt.Errorf("expected a JSON object")
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, fmt.Errorf("expected a single JSON object")
	}
	return value, nil
}

func validateActivationContext(value map[string]any) error {
	for key, raw := range value {
		if key != "entities" {
			return fmt.Errorf("only entities may narrow the report context")
		}
		entities, ok := raw.([]any)
		if !ok || len(entities) > 1000 {
			return fmt.Errorf("entities must be an array of at most 1000 selections")
		}
		for _, entry := range entities {
			entity, ok := entry.(map[string]any)
			if !ok || len(entity) != 2 {
				return fmt.Errorf("each entity requires type and id")
			}
			entityType, typeOK := entity["type"].(string)
			entityID, idOK := entity["id"].(string)
			if !typeOK || !idOK || strings.TrimSpace(entityType) == "" || strings.TrimSpace(entityID) == "" {
				return fmt.Errorf("each entity requires nonempty type and id")
			}
		}
	}
	return nil
}
