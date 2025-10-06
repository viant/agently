package agent

import (
	"strings"

	"github.com/viant/agently/genai/agent"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/service/core"
)

func EnsureGenerateOptions(i *core.GenerateInput, agent *agent.Agent) {
	// Propagate agent-level temperature to per-request options if not explicitly set.
	// Keep any existing options provided via model selection.
	if i.Options == nil {
		i.Options = &llm.Options{}
	}

	if i.Options.Temperature == 0 && agent.Temperature != 0 {
		i.Options.Temperature = agent.Temperature
	}
	// Carry agent-level parallel tool-calls preference; capability gating
	// happens later in core.updateFlags based on provider/model support.
	i.Options.ParallelToolCalls = agent.ParallelToolCalls
	// Pass attach mode as metadata so providers can honor ref vs inline.
	if i.Options.Metadata == nil {
		i.Options.Metadata = map[string]interface{}{}
	}
	mode := strings.TrimSpace(strings.ToLower(agent.AttachMode))
	if mode == "" {
		mode = "ref"
	}
	i.Options.Metadata["attachMode"] = mode
	// Use agentId for provider-side scoping (uploads, telemetry). Agent name is reserved for prompt identity only.
	i.Options.Metadata["agentId"] = agent.ID
	// Optional TTL for attachments (in seconds)
	if agent.AttachmentTTLSec > 0 {
		i.Options.Metadata["attachmentTTLSec"] = agent.AttachmentTTLSec
	}
}
