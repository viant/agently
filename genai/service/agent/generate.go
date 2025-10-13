package agent

import (
	"context"
	"strings"

	"github.com/viant/agently/genai/agent"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/service/core"
	"github.com/viant/agently/internal/auth"
)

func EnsureGenerateOptions(ctx context.Context, i *core.GenerateInput, agent *agent.Agent) {
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
	mode := "ref"
	if agent.Attachment != nil {
		if m := strings.TrimSpace(strings.ToLower(agent.Attachment.Mode)); m != "" {
			mode = m
		}
		if agent.Attachment.TTLSec > 0 {
			i.Options.Metadata["attachmentTTLSec"] = agent.Attachment.TTLSec
		}
		if agent.Attachment.ToolCallConversionThreshold > 0 {
			if _, exists := i.Options.Metadata["toolAttachmentThresholdBytes"]; !exists {
				i.Options.Metadata["toolAttachmentThresholdBytes"] = agent.Attachment.ToolCallConversionThreshold
			}
		}
	}

	// No additional defaults here; Agent.Init sets defaults in a single place
	i.Options.Metadata["attachMode"] = mode
	// Use agentId for provider-side scoping (uploads, telemetry). Agent name is reserved for prompt identity only.
	i.Options.Metadata["agentId"] = agent.ID

	if ui := auth.User(ctx); ui != nil {
		uname := strings.TrimSpace(ui.Subject)
		if uname == "" {
			uname = strings.TrimSpace(ui.Email)
		}
		i.UserID = uname
	}

}
