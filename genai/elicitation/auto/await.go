package auto

// Auto-elicitation Awaiter that uses a helper LLM-agent to craft JSON payload
// satisfying a given schema. The implementation purposely lives in a package
// below extension/fluxor so that it can be shared by both CLI and Fluxor
// runtimes without introducing cyclic imports.

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/viant/agently/genai/agent/plan"
	"github.com/viant/agently/genai/elicitation"
	mcpproto "github.com/viant/mcp-protocol/schema"
)

// HelperFunc abstracts the call to an LLM agent so we don’t import higher
// layers and avoid cycles.
//
//	helper(ctx, agentId, prompt) → raw assistant reply or error.
type HelperFunc func(ctx context.Context, agentId, prompt string) (string, error)

// Config controls retries and timeouts.
type Config struct {
	HelperAgent string        // required
	MaxRounds   int           // ≥1, defaults to 1
	Timeout     time.Duration // per-round; 0 → 20s
}

// Awaiter implements elicitation.Awaiter using a HelperFunc.
type Awaiter struct {
	helper HelperFunc
	cfg    Config
}

// New returns an Auto Awaiter. cfg.HelperAgent must be non-empty; helper must
// be non-nil.
func New(helper HelperFunc, cfg Config) *Awaiter {
	if cfg.MaxRounds <= 0 {
		cfg.MaxRounds = 1
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Minute
	}
	return &Awaiter{helper: helper, cfg: cfg}
}

// Ensure interface compliance.
var _ elicitation.Awaiter = (*Awaiter)(nil)

func (a *Awaiter) AwaitElicitation(ctx context.Context, req *plan.Elicitation) (*plan.ElicitResult, error) {
	if a == nil || a.helper == nil {
		return nil, errors.New("auto-await: helper is nil")
	}
	if req == nil {
		return nil, errors.New("auto-await: request is nil")
	}
	prompt := buildPrompt(req)

	for round := 0; round < a.cfg.MaxRounds; round++ {
		// Run helper without deriving a child cancel; bound wait with a timer.
		type result struct {
			reply string
			err   error
		}
		ch := make(chan result, 1)
		go func() {
			r, err := a.helper(ctx, a.cfg.HelperAgent, prompt)
			// Non-blocking send to avoid goroutine leak on timeout path.
			select {
			case ch <- result{reply: r, err: err}:
			default:
			}
		}()

		var to <-chan time.Time
		if a.cfg.Timeout > 0 {
			to = time.After(a.cfg.Timeout)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-to:
			// Timeout this round; continue to next round if available.
			continue
		case res := <-ch:
			if res.err != nil {
				continue
			}
			payload, ok := extractJSON(res.reply)
			if !ok {
				continue
			}
			if validPayload(payload, &req.RequestedSchema) {
				return &plan.ElicitResult{Action: plan.ElicitResultActionAccept, Payload: payload}, nil
			}
		}
	}
	return &plan.ElicitResult{Action: plan.ElicitResultActionDecline}, nil
}

// buildPrompt crafts the instruction for the helper agent.
func buildPrompt(req *plan.Elicitation) string {
	schemaJSON, _ := json.Marshal(req.RequestedSchema)
	var b strings.Builder
	b.WriteString("You are assisting another AI. Answer ONLY with a single JSON object that satisfies the following JSON Schema.\n")
	if msg := strings.TrimSpace(req.Message); msg != "" {
		b.WriteString("Human instruction: \n")
		b.WriteString(msg)
		b.WriteString("\n\n")
	}
	b.WriteString("Schema:\n")
	b.Write(schemaJSON)
	b.WriteString("\n\nJSON:")
	return b.String()
}

var jsonBlockRE = regexp.MustCompile(`(?s)\{.*?\}`)

// extractJSON returns the first JSON object found in text.
func extractJSON(text string) (map[string]any, bool) {
	loc := jsonBlockRE.FindStringIndex(text)
	if loc == nil {
		return nil, false
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(text[loc[0]:loc[1]]), &m); err != nil {
		return nil, false
	}
	return m, true
}

// validPayload ensures every required field is present (simple heuristic).
func validPayload(p map[string]any, schema *mcpproto.ElicitRequestParamsRequestedSchema) bool {
	if schema == nil || len(schema.Required) == 0 {
		return true
	}
	for _, r := range schema.Required {
		if _, ok := p[r]; !ok {
			return false
		}
	}
	return true
}
