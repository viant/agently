package config

import (
	"fmt"
	"strconv"
	"strings"
)

// ResourceRoot describes a root entry under MCP client metadata.resources.roots.
type ResourceRoot struct {
	ID           string
	URI          string
	Description  string
	Vectorize    bool
	Snapshot     bool
	SnapshotMD5  bool
	AllowGrep    bool
	UpstreamRef  string
	SnapshotURI  string
	SnapshotRoot string
}

// Upstream describes an upstream database used for embedius sync/bootstrap.
type Upstream struct {
	Name               string
	Driver             string
	DSN                string
	Shadow             string
	Batch              int
	Force              bool
	Enabled            bool
	MinIntervalSeconds int
}

// ResourceRoots extracts resource root metadata from a generic metadata map.
func ResourceRoots(meta map[string]interface{}) []ResourceRoot {
	if len(meta) == 0 {
		return nil
	}
	resources := toStringMap(meta["resources"])
	if len(resources) == 0 {
		return nil
	}
	rootsVal, ok := resources["roots"]
	if !ok || rootsVal == nil {
		return nil
	}
	list, ok := rootsVal.([]interface{})
	if !ok {
		return nil
	}
	out := make([]ResourceRoot, 0, len(list))
	for _, item := range list {
		m := toStringMap(item)
		if len(m) == 0 {
			continue
		}
		root := ResourceRoot{
			ID:           getString(m, "id", "ID"),
			URI:          getString(m, "uri", "URI"),
			Description:  getString(m, "description", "Description"),
			Vectorize:    getBool(m, "vectorization", "vectorize"),
			Snapshot:     getBool(m, "snapshot"),
			SnapshotMD5:  getBool(m, "snapshotManifest", "snapshot_manifest", "snapshotMD5", "snapshot_md5"),
			AllowGrep:    getBool(m, "allowGrep", "allow_grep"),
			UpstreamRef:  getString(m, "upstreamRef", "upstream_ref"),
			SnapshotURI:  getString(m, "snapshotUri", "snapshotURI"),
			SnapshotRoot: getString(m, "snapshotRoot", "snapshot_root"),
		}
		if strings.TrimSpace(root.URI) == "" {
			continue
		}
		out = append(out, root)
	}
	return out
}

// Upstreams extracts upstream definitions from metadata.
func Upstreams(meta map[string]interface{}) map[string]Upstream {
	if len(meta) == 0 {
		return nil
	}
	resources := toStringMap(meta["resources"])
	if len(resources) == 0 {
		return nil
	}
	upVal, ok := resources["upstreams"]
	if !ok || upVal == nil {
		return nil
	}
	list, ok := upVal.([]interface{})
	if !ok {
		return nil
	}
	out := make(map[string]Upstream, len(list))
	for _, item := range list {
		m := toStringMap(item)
		if len(m) == 0 {
			continue
		}
		enabled := true
		if v, ok := getOptionalBool(m, "enabled", "sync", "upstream"); ok {
			enabled = v
		}
		up := Upstream{
			Name:               getString(m, "name", "Name"),
			Driver:             getString(m, "driver", "Driver"),
			DSN:                getString(m, "dsn", "DSN"),
			Shadow:             getString(m, "shadow", "shadowTable"),
			Batch:              getInt(m, "batch", "syncBatch"),
			Force:              getBool(m, "forceSync", "force"),
			Enabled:            enabled,
			MinIntervalSeconds: getInt(m, "minIntervalSeconds", "minIntervalSecs", "syncIntervalSeconds", "syncIntervalSecs"),
		}
		if strings.TrimSpace(up.Name) == "" {
			continue
		}
		out[up.Name] = up
	}
	return out
}

// ResolveUpstream returns the upstream referenced by a root, if any.
func ResolveUpstream(meta map[string]interface{}, root ResourceRoot) (*Upstream, bool) {
	ref := strings.TrimSpace(root.UpstreamRef)
	if ref == "" {
		return nil, false
	}
	upstreams := Upstreams(meta)
	if len(upstreams) == 0 {
		return nil, false
	}
	up, ok := upstreams[ref]
	if !ok {
		return nil, false
	}
	return &up, true
}

// ValidateResourceRoots returns warnings for invalid resource root settings.
func ValidateResourceRoots(meta map[string]interface{}) []string {
	var warnings []string
	roots := ResourceRoots(meta)
	upstreams := Upstreams(meta)
	for _, root := range roots {
		if (root.Vectorize || root.AllowGrep) && !root.Snapshot {
			warnings = append(warnings, fmt.Sprintf("resource root %q enables vectorization/grep without snapshot support", root.URI))
		}
		if root.SnapshotMD5 && !root.Snapshot {
			warnings = append(warnings, fmt.Sprintf("resource root %q enables snapshot MD5 without snapshot support", root.URI))
		}
		ref := strings.TrimSpace(root.UpstreamRef)
		if ref != "" {
			up, ok := upstreams[ref]
			if !ok {
				warnings = append(warnings, fmt.Sprintf("resource root %q references unknown upstream %q", root.URI, ref))
				continue
			}
			if strings.TrimSpace(up.Driver) == "" || strings.TrimSpace(up.DSN) == "" {
				warnings = append(warnings, fmt.Sprintf("resource root %q upstream %q missing driver or dsn", root.URI, ref))
			}
		}
	}
	return warnings
}

func toStringMap(v interface{}) map[string]interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		return val
	case map[interface{}]interface{}:
		out := make(map[string]interface{}, len(val))
		for k, v := range val {
			if ks, ok := k.(string); ok {
				out[ks] = v
			}
		}
		return out
	default:
		return nil
	}
}

func getString(m map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if v, ok := m[key]; ok && v != nil {
			switch s := v.(type) {
			case string:
				if strings.TrimSpace(s) != "" {
					return s
				}
			}
		}
	}
	return ""
}

func getBool(m map[string]interface{}, keys ...string) bool {
	for _, key := range keys {
		if v, ok := m[key]; ok && v != nil {
			switch b := v.(type) {
			case bool:
				return b
			case string:
				switch strings.ToLower(strings.TrimSpace(b)) {
				case "true", "yes", "1", "on":
					return true
				}
			}
		}
	}
	return false
}

func getOptionalBool(m map[string]interface{}, keys ...string) (bool, bool) {
	for _, key := range keys {
		if v, ok := m[key]; ok && v != nil {
			switch b := v.(type) {
			case bool:
				return b, true
			case string:
				switch strings.ToLower(strings.TrimSpace(b)) {
				case "true", "yes", "1", "on":
					return true, true
				case "false", "no", "0", "off":
					return false, true
				}
			}
		}
	}
	return false, false
}

func getInt(m map[string]interface{}, keys ...string) int {
	for _, key := range keys {
		if v, ok := m[key]; ok && v != nil {
			switch n := v.(type) {
			case int:
				return n
			case int64:
				return int(n)
			case float64:
				return int(n)
			case string:
				if val := strings.TrimSpace(n); val != "" {
					if parsed, err := strconv.Atoi(val); err == nil {
						return parsed
					}
				}
			}
		}
	}
	return 0
}
