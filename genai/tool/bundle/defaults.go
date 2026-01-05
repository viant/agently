package bundle

import (
	"sort"
	"strings"

	"github.com/viant/agently/genai/llm"
	mcpname "github.com/viant/agently/pkg/mcpname"
)

// DefaultIconRef returns an iconRef for well-known built-in tool services.
// UI is expected to resolve these IDs to PNG assets.
func DefaultIconRef(service string) string {
	service = strings.TrimSpace(service)
	switch service {
	case "resources":
		return "builtin:resources"
	case "system/exec":
		return "builtin:system-exec"
	case "system/patch":
		return "builtin:system-patch"
	case "system/os":
		return "builtin:system-os"
	case "system/image":
		return "builtin:system-image"
	case "llm/agents":
		return "builtin:agents"
	case "llm/exec":
		return "builtin:agents"
	case "orchestration":
		return "builtin:orchestration"
	case "internal/message":
		return "builtin:message"
	case "agentExec":
		return "builtin:agents"
	default:
		return "builtin:tools"
	}
}

// DeriveBundles groups concrete tool definitions by their service name and
// returns a default bundle list. It is used when no workspace bundles exist.
func DeriveBundles(defs []llm.ToolDefinition) []*Bundle {
	if len(defs) == 0 {
		return nil
	}
	byService := map[string]*Bundle{}
	for _, d := range defs {
		service := serviceFromToolName(d.Name)
		if service == "" {
			continue
		}
		// Collapse agentExec tools into a single bundle.
		if strings.EqualFold(service, "agentExec") {
			service = "agentExec"
		}
		b := byService[service]
		if b == nil {
			b = &Bundle{
				ID:      service,
				Title:   service,
				IconRef: DefaultIconRef(service),
				Match:   []MatchRule{{Name: service + "/*"}},
			}
			byService[service] = b
		}
	}

	out := make([]*Bundle, 0, len(byService))
	for _, b := range byService {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func serviceFromToolName(name string) string {
	can := mcpname.Canonical(strings.TrimSpace(name))
	n := mcpname.Name(can)
	return n.Service()
}
