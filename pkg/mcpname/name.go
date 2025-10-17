package mcpname

import "strings"

// Name represents canonical tool name service_path-method.
type Name string

// Canonical normalises a tool name to service_path-method.
// Supported inputs:
//   - canonical itself (service_path-method)
//   - service/path.method
//   - service/path-method
//   - service/path/method
//   - service/path:method (extended)
func Canonical(raw string) string {
	if raw == "" {
		return ""
	}
	if strings.Contains(raw, "-") && !strings.ContainsAny(raw, "/.:") {
		return raw
	}
	var service, method string
	// Prefer dot/colon separators when present
	if idx := strings.LastIndex(raw, "."); idx != -1 {
		service, method = raw[:idx], raw[idx+1:]
	} else if idx := strings.LastIndex(raw, ":"); idx != -1 {
		service, method = raw[:idx], raw[idx+1:]
	} else if idx := strings.LastIndex(raw, "-"); idx != -1 && strings.Contains(raw, "/") {
		service, method = raw[:idx], raw[idx+1:]
	} else if idx := strings.LastIndex(raw, "/"); idx != -1 {
		service, method = raw[:idx], raw[idx+1:]
	} else {
		return raw
	}
	service = strings.ReplaceAll(service, "/", "_")
	return service + "-" + method
}

func (t Name) Service() string {
	tool := string(t)
	if idx := strings.LastIndex(tool, "-"); idx != -1 {
		return strings.ReplaceAll(tool[:idx], "_", "/")
	}
	return tool
}

func (t Name) Method() string {
	tool := string(t)
	if idx := strings.LastIndex(tool, "-"); idx != -1 {
		return tool[idx+1:]
	}
	return ""
}

func (t Name) ToolName() string {
	r := string(t)
	r = strings.ReplaceAll(r, "/", "_")
	return r
}

func (t Name) String() string { return string(t) }

func NewName(service, name string) Name {
	return Name(strings.ReplaceAll(service, "/", "_") + "-" + name)
}
