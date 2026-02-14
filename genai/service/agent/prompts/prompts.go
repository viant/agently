package prompts

import (
	_ "embed"
	"strings"
)

//go:embed summary.md
var Summary string

//go:embed compact.md
var Compact string

//go:embed prune_prompt.md
var Prune string

//go:embed router.md
var Router string

//go:embed capability.md
var Capability string

func RouterPrompt(outputKey string) string {
	key := strings.TrimSpace(outputKey)
	if key == "" {
		key = "agentId"
	}
	return strings.ReplaceAll(Router, "{{outputKey}}", key)
}

func CapabilityPrompt() string {
	return Capability
}
