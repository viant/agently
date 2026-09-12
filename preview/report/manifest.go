package reportpreview

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type previewManifest struct {
	Version  int    `yaml:"version"`
	GroupID  string `yaml:"groupId"`
	ReportID string `yaml:"reportId"`
	MCP      struct {
		Tool        string `yaml:"tool"`
		DefaultFile string `yaml:"defaultFile"`
		Routes      []struct {
			Match map[string]any `yaml:"match"`
			File  string         `yaml:"file"`
		} `yaml:"routes"`
		Query struct {
			Dimensions string `yaml:"dimensions"`
			Measures   string `yaml:"measures"`
			Filter     string `yaml:"filter"`
			OrderBy    string `yaml:"orderBy"`
			Limit      string `yaml:"limit"`
			Offset     string `yaml:"offset"`
		} `yaml:"query"`
		Definition struct {
			Tool      string         `yaml:"tool"`
			Arguments map[string]any `yaml:"arguments"`
			Match     map[string]any `yaml:"match"`
		} `yaml:"definition"`
	} `yaml:"mcp"`
}

func manifestValue(arguments map[string]any, dottedPath string) (any, bool) {
	if dottedPath == "" {
		return nil, false
	}
	var current any = arguments
	for _, part := range strings.Split(dottedPath, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func loadPreviewManifest(folder, groupID, reportID string) (*previewManifest, error) {
	body, err := os.ReadFile(filepath.Join(folder, "preview.yaml"))
	if err != nil {
		return nil, fmt.Errorf("load report preview manifest: %w", err)
	}
	manifest := &previewManifest{}
	if err = yaml.Unmarshal(body, manifest); err != nil {
		return nil, fmt.Errorf("decode report preview manifest: %w", err)
	}
	if manifest.Version != 1 || strings.TrimSpace(manifest.MCP.Tool) == "" {
		return nil, fmt.Errorf("report preview manifest requires version 1 and mcp.tool")
	}
	if manifest.GroupID != "" && groupID != "" && manifest.GroupID != groupID {
		return nil, fmt.Errorf("group ID %q does not match preview manifest %q", groupID, manifest.GroupID)
	}
	if manifest.ReportID != "" && reportID != "" && manifest.ReportID != reportID {
		return nil, fmt.Errorf("report ID %q does not match preview manifest %q", reportID, manifest.ReportID)
	}
	if manifest.MCP.Definition.Tool == "" {
		manifest.MCP.Definition.Tool = manifest.MCP.Tool
	}
	return manifest, nil
}

func expandManifestValue(value any, groupID, reportID string) any {
	switch actual := value.(type) {
	case string:
		return strings.ReplaceAll(strings.ReplaceAll(actual, "${groupId}", groupID), "${reportId}", reportID)
	case []any:
		result := make([]any, len(actual))
		for i, item := range actual {
			result[i] = expandManifestValue(item, groupID, reportID)
		}
		return result
	case map[string]any:
		result := map[string]any{}
		for key, item := range actual {
			result[key] = expandManifestValue(item, groupID, reportID)
		}
		return result
	default:
		return value
	}
}
