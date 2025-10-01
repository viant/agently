package conversation

import (
	"context"
	"encoding/json"
	"log"
	"sort"
	"strings"

	extx "github.com/viant/agently/genai/tool"
	"github.com/viant/agently/pkg/agently/tool"
	invoker "github.com/viant/agently/pkg/agently/tool/invoker"
	selres "github.com/viant/agently/pkg/agently/tool/resolver"
	mcptool "github.com/viant/fluxor-mcp/mcp/tool"
	ftypes "github.com/viant/forge/backend/types"
)

// OnRelation is invoked after related records are loaded.
// Ensure messages are ordered by CreatedAt ascending (oldest first) and compute Turn stage.
func (t *TranscriptView) OnRelation(ctx context.Context) {
	if t == nil {
		return
	}
	// Normalize messages when present to ensure deterministic order and elapsed time
	if len(t.Message) > 0 {
		t.normalizeMessages()
	}
	// Always attempt to compute tool executions. For activation.kind==tool_call
	// we may still want to invoke even when there are no recorded tool calls.
	t.updateToolFeed(ctx)
	// Compute stage for this turn
	t.Stage = computeTurnStage(t)
}

func (t *TranscriptView) updateToolFeed(ctx context.Context) {
	input := InputFromContext(ctx)
	if len(input.FeedSpec) == 0 {
		return
	}
	// Collect tool calls in this turn (oldest→newest already ensured by normalizeMessages)
	var toolCalls []*ToolCallView
	for _, m := range t.Message {
		if m != nil && m.ToolCall != nil {
			toolCalls = append(toolCalls, m.ToolCall)
		}
	}
	// Do not return early when no tool calls; tool_call activations may still
	// want to invoke live and produce execution details.

	// Helper to canonicalize tool name and extract service/method using MCP tool helpers.
	splitName := func(name string) (string, string) {
		canonical := mcptool.Canonical(strings.TrimSpace(name))
		nm := mcptool.Name(canonical)
		return nm.Service(), nm.Method()
	}

	// Resolution delegated to selector resolver package.

	// Build toolFeed per matched extension.
	var toolFeed []*tool.Feed

	// Iterate extensions and apply activation/match rules.
	for _, ext := range input.FeedSpec {
		if ext == nil {
			continue
		}
		kind := strings.ToLower(strings.TrimSpace(ext.Activation.Kind))
		if kind == "" {
			kind = "history"
		}
		scope := strings.ToLower(strings.TrimSpace(ext.Activation.Scope))
		if scope == "" {
			scope = "last"
		}

		// For v1: support history (use last) and tool_call (invoke once per extension).
		var selected []*ToolCallView
		if kind == "history" {
			selected = toolCalls
			if scope == "last" && len(selected) > 1 {
				selected = selected[len(selected)-1:]
			}
		} else if kind == "tool_call" && len(toolCalls) == 0 {
			// No recorded tool calls; attempt a direct invocation using activation or match.
			callSvc := strings.TrimSpace(ext.Activation.Service)
			callMth := strings.TrimSpace(ext.Activation.Method)
			if callSvc == "" {
				callSvc = strings.TrimSpace(ext.Match.Service)
			}
			if callMth == "" {
				callMth = strings.TrimSpace(ext.Match.Method)
			}
			if callSvc != "" && callMth != "" {
				log.Printf("[hook] tool_call invoke ext=%s service=%s method=%s", ext.ID, callSvc, callMth)
				if inv := invoker.From(ctx); inv != nil {
					if out, err := inv.Invoke(ctx, callSvc, callMth, ext.Activation.Args); err == nil {
						var outRoot interface{}
						switch v := out.(type) {
						case string:
							var any interface{}
							if json.Unmarshal([]byte(v), &any) == nil {
								outRoot = any
							}
						default:
							outRoot = v
						}
						exec := &tool.Feed{ID: ext.ID,
							UI:     &ext.UI,
							Title:  bestTitle(ext),
							Source: extx.Source{Service: callSvc, Method: callMth}}
						for _, ds := range ext.DataSource {
							if ds.Name == "" || ds.Selector == "" {
								continue
							}
							val := selres.Select(ds.Selector, nil, outRoot)
							exec.Data = append(exec.Data, extx.DataItem{Name: ds.Name, Data: val, RawSelector: ds.Selector})
						}
						toolFeed = append(toolFeed, exec)
					}
				}
			}
			continue
		} else {
			// tool_call with existing toolCalls: fall back to last recorded
			selected = toolCalls
			if len(selected) > 1 {
				selected = selected[len(selected)-1:]
			}
		}

		// For history/all we may aggregate across multiple calls; collect matching first.
		var matched []*ToolCallView
		for _, tc := range selected {
			if tc == nil {
				continue
			}
			svc, mth := splitName(tc.ToolName)

			// Match check with wildcards
			matchSvc := strings.TrimSpace(ext.Match.Service)
			matchMth := strings.TrimSpace(ext.Match.Method)
			okSvc := matchSvc == "" || matchSvc == "*" || strings.EqualFold(matchSvc, svc)
			okMth := matchMth == "" || matchMth == "*" || strings.EqualFold(matchMth, mth)
			if !okSvc || !okMth {
				continue
			}

			matched = append(matched, tc)

			// For history/last we keep only the last matched; continue to collect to know last

		}

		if len(matched) == 0 {
			continue
		}
		// Determine source from the last matched call
		last := matched[len(matched)-1]
		svc, mth := splitName(last.ToolName)

		// History scope: all vs last
		if kind == "history" && scope == "all" {
			// Aggregate per data-source across all matched calls
			agg := map[string][]interface{}{}
			// Iterate in order
			for _, tc := range matched {
				var outRoot interface{}
				var inRoot interface{}
				if tc.ResponsePayload != nil && tc.ResponsePayload.InlineBody != nil {
					_ = json.Unmarshal([]byte(*tc.ResponsePayload.InlineBody), &outRoot)
				} else if tc.ResponseSnapshot != nil {
					_ = json.Unmarshal([]byte(*tc.ResponseSnapshot), &outRoot)
				}
				if tc.RequestPayload != nil && tc.RequestPayload.InlineBody != nil {
					_ = json.Unmarshal([]byte(*tc.RequestPayload.InlineBody), &inRoot)
				} else if tc.RequestSnapshot != nil {
					_ = json.Unmarshal([]byte(*tc.RequestSnapshot), &inRoot)
				}
				for _, ds := range ext.DataSource {
					if ds.Name == "" || ds.Selector == "" {
						continue
					}
					val := selres.Select(ds.Selector, inRoot, outRoot)
					if val == nil {
						continue
					}
					switch arr := val.(type) {
					case []interface{}:
						agg[ds.Name] = append(agg[ds.Name], arr...)
					default:
						agg[ds.Name] = append(agg[ds.Name], val)
					}
				}
			}
			exec := &tool.Feed{ID: ext.ID, Title: bestTitle(ext), Source: extx.Source{Service: svc, Method: mth}, UI: &ext.UI}
			// Preserve declared data-source order in output
			for _, ds := range ext.DataSource {
				if ds.Name == "" || ds.Selector == "" {
					continue
				}
				if vals, ok := agg[ds.Name]; ok {
					exec.Data = append(exec.Data, extx.DataItem{Name: ds.Name, Data: vals, RawSelector: ds.Selector})
				} else {
					exec.Data = append(exec.Data, extx.DataItem{Name: ds.Name, Data: nil, RawSelector: ds.Selector})
				}
			}
			toolFeed = append(toolFeed, exec)
			continue
		}

		// Default path (history/last or tool_call): evaluate last matched
		tc := last
		var outRoot interface{}
		var inRoot interface{}
		if tc.ResponsePayload != nil && tc.ResponsePayload.InlineBody != nil {
			_ = json.Unmarshal([]byte(*tc.ResponsePayload.InlineBody), &outRoot)
		} else if tc.ResponseSnapshot != nil {
			_ = json.Unmarshal([]byte(*tc.ResponseSnapshot), &outRoot)
		}
		if tc.RequestPayload != nil && tc.RequestPayload.InlineBody != nil {
			_ = json.Unmarshal([]byte(*tc.RequestPayload.InlineBody), &inRoot)
		} else if tc.RequestSnapshot != nil {
			_ = json.Unmarshal([]byte(*tc.RequestSnapshot), &inRoot)
		}
		if kind == "tool_call" && outRoot == nil {
			if inv := invoker.From(ctx); inv != nil {
				args := ext.Activation.Args
				callSvc := strings.TrimSpace(ext.Activation.Service)
				callMth := strings.TrimSpace(ext.Activation.Method)
				if callSvc == "" {
					callSvc = svc
				}
				if callMth == "" {
					callMth = mth
				}
				log.Printf("[hook] tool_call invoke ext=%s service=%s method=%s (no recorded payload)", ext.ID, callSvc, callMth)
				if out, err := inv.Invoke(ctx, callSvc, callMth, args); err == nil {
					switch v := out.(type) {
					case string:
						var any interface{}
						if json.Unmarshal([]byte(v), &any) == nil {
							outRoot = any
						}
					default:
						outRoot = v
					}
				}
			}
		}
		exec := &tool.Feed{ID: ext.ID, Title: bestTitle(ext), Source: extx.Source{Service: svc, Method: mth}, UI: &ext.UI}
		for _, ds := range ext.DataSource {
			if ds.Name == "" || ds.Selector == "" {
				continue
			}
			val := selres.Select(ds.Selector, inRoot, outRoot)
			exec.Data = append(exec.Data, extx.DataItem{Name: ds.Name, Data: val, RawSelector: ds.Selector})
		}
		toolFeed = append(toolFeed, exec)
	}

	t.ToolFeed = toolFeed
}

func (t *TranscriptView) normalizeMessages() {
	// Stable sort to preserve original order for equal timestamps.
	sort.SliceStable(t.Message, func(i, j int) bool {
		mi, mj := t.Message[i], t.Message[j]
		if mi == nil || mj == nil {
			// Keep non-nil before nil
			return mj == nil && mi != nil
		}
		if mi.CreatedAt.Equal(mj.CreatedAt) {
			if mi.ToolCall != nil {
				return true
			}
			// Use Sequence if available as a tie breaker
			if mi.Sequence != nil && mj.Sequence != nil {
				return *mi.Sequence < *mj.Sequence
			}
			// Fall back to ID to ensure deterministic ordering
			return mi.Id < mj.Id
		}
		return mi.CreatedAt.Before(mj.CreatedAt)
	})
	minTime := t.Message[0].CreatedAt
	maxTime := t.Message[len(t.Message)-1].CreatedAt
	t.ElapsedInSec = int(maxTime.Sub(minTime).Seconds())
	for _, m := range t.Message {
		if m.ModelCall != nil {
			m.Status = &m.ModelCall.Status
		}
		if m.ToolCall != nil {
			m.Status = &m.ToolCall.Status
		}
	}
}

// toContainer converts a Forge view definition into a single renderable container.
// It supports both single-container views and multi-container views by picking
// the primary container when multiple are defined. It uses a JSON round-trip to
// tolerate structural differences between types.View and types.Container.
func toContainer(v ftypes.View) *ftypes.Container {
	var c ftypes.Container
	if b, err := json.Marshal(v); err == nil {
		_ = json.Unmarshal(b, &c)
	}
	return &c
}

// bestTitle prefers UI/View title or label; falls back to metadata Title.
func bestTitle(ext *tool.FeedSpec) string {
	if ext == nil {
		return ""
	}
	if b, err := json.Marshal(ext.UI); err == nil {
		var any map[string]interface{}
		if json.Unmarshal(b, &any) == nil && any != nil {
			if v, ok := any["title"].(string); ok && strings.TrimSpace(v) != "" {
				return v
			}
			if v, ok := any["label"].(string); ok && strings.TrimSpace(v) != "" {
				return v
			}
		}
	}
	return ext.Title
}

// computeTurnStage determines the stage of a single turn based on its latest non-interim message.
func computeTurnStage(t *TranscriptView) string {
	if t == nil || len(t.Message) == 0 {
		return StageWaiting
	}
	// If turn itself is canceled, treat as completed
	if strings.EqualFold(strings.TrimSpace(t.Status), "canceled") {
		return StageDone
	}
	lastRole := ""
	lastAssistantElic := false
	lastToolRunning := false
	lastToolFailed := false
	lastModelRunning := false
	lastAssistantCanceled := false

	// Iterate messages backwards to find cancellation or the latest non-interim one
	for i := len(t.Message) - 1; i >= 0; i-- {
		m := t.Message[i]
		if m == nil {
			continue
		}
		// If the latest assistant message is explicitly canceled (even interim), drop to waiting
		if strings.EqualFold(strings.TrimSpace(m.Role), "assistant") && m.Status != nil && strings.EqualFold(strings.TrimSpace(*m.Status), "canceled") {
			lastAssistantCanceled = true
			break
		}
		if m.Interim != 0 {
			continue
		}
		r := strings.ToLower(strings.TrimSpace(m.Role))
		if lastRole == "" {
			lastRole = r
		}
		if m.ToolCall != nil {
			status := strings.ToLower(strings.TrimSpace(m.ToolCall.Status))
			if status == "running" || m.ToolCall.CompletedAt == nil {
				lastToolRunning = true
			}
			if status == "failed" {
				lastToolFailed = true
			}
		}
		if m.ModelCall != nil {
			mstatus := strings.ToLower(strings.TrimSpace(m.ModelCall.Status))
			if mstatus == "running" || m.ModelCall.CompletedAt == nil {
				lastModelRunning = true
			}
		}
		if r == "assistant" && m.ElicitationId != nil && strings.TrimSpace(*m.ElicitationId) != "" {
			lastAssistantElic = true
		}
		break
	}

	switch {
	case lastAssistantCanceled:
		return StageDone
	case lastToolRunning:
		return StageExecuting
	case lastAssistantElic:
		return StageEliciting
	case lastModelRunning:
		return StageThinking
	case lastRole == "user":
		return StageThinking
	case lastToolFailed:
		return StageError
	default:
		return StageDone
	}
}
