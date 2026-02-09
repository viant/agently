package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/genai/prompt"
	"github.com/viant/agently/genai/service/agent/prompts"
	core2 "github.com/viant/agently/genai/service/core"
	"github.com/viant/agently/internal/auth"
)

// freeMessageTokensLLM kicks off a focused plan run with the guidance note as
// the user prompt so the assistant can immediately use internal/message tools
// (e.g., remove/summarize) to free tokens and continue the conversation.
func (s *Service) freeMessageTokensLLM(ctx context.Context, conv *apiconv.Conversation, instruction string, oldGenInput *core2.GenerateInput, overlimit int) error {
	if s == nil || s.llm == nil || conv == nil {
		return fmt.Errorf("missing llm or conversation")
	}
	// Ensure turn meta so recorder/stream handler can attach artifacts properly.
	turn := s.ensureTurnMeta(ctx, conv)
	ctx = memory.WithTurnMeta(ctx, turn)

	// Prefer an injected builder that mirrors agent.runPlanLoop with `instruction` as the user query.
	var genInput *core2.GenerateInput

	if s.buildPlanInput == nil {
		return fmt.Errorf("missing buildPlanInput function for freeMessageTokensLLM")
	}

	var err error
	genInput, err = s.buildPlanInput(ctx, conv, instruction)
	if err != nil {
		return err
	}

	// Attribute participants for naming and validation
	if uid := auth.EffectiveUserID(ctx); strings.TrimSpace(uid) != "" {
		genInput.UserID = uid
	} else {
		genInput.UserID = "system"
	}

	if genInput.Options == nil {
		genInput.Options = &llm.Options{}
	}
	genInput.Options.Mode = "plan"

	//// Strip system content and configure minimal tool set for recovery
	s.stripSystemMessages(genInput)

	s.adjustToolDefinitions(genInput)

	// Compare old vs new request footprint and prune history if needed
	tokenDelta, err := s.computeTokenDiff(ctx, genInput, oldGenInput)
	if err != nil {
		return fmt.Errorf("failed to compute token diff: %v", err)
	}

	adjustInputIfNeeded(tokenDelta, overlimit, genInput)
	//fmt.Printf("[debug] freeMessageTokensLLM: tokenDelta=%d, overlimit=%d\n", tokenDelta, overlimit) // TODO delete

	genOutput := &core2.GenerateOutput{}
	// Mark this Run as continuation/recovery so DuplicateGuard remains
	// disabled and internal/message tools can iterate as needed.
	ctx = context.WithValue(ctx, ctxKeyContinuationMode, true)
	if _, err := s.Run(ctx, genInput, genOutput); err != nil {
		return err
	}
	return nil
}

// compactHistoryLLM performs a full compaction pass using the compact prompt
// and internal_message-remove to archive older messages.
func (s *Service) compactHistoryLLM(ctx context.Context, conv *apiconv.Conversation, errMessage string, oldGenInput *core2.GenerateInput, overlimit int) error {
	if conv == nil {
		return fmt.Errorf("missing conversation")
	}
	lines, ids := s.buildRemovalCandidates(ctx, conv, compactCandidateLimit)
	if len(lines) == 0 {
		lines = []string{"(no removable items identified)"}
	}
	instruction := s.composeCompactPrompt(errMessage, lines, ids)
	return s.freeMessageTokensLLM(ctx, conv, instruction, oldGenInput, overlimit)
}

func (s *Service) adjustToolDefinitions(genInput *core2.GenerateInput) {
	genInput.Options.Tools = []llm.Tool{}
	genInput.Binding.Tools = prompt.Tools{}
	if s.registry != nil {
		for _, def := range s.registry.MatchDefinition("internal/message") {
			if def == nil {
				continue
			}

			if strings.Contains(def.Name, "remove") {
				tmpDef := *def
				tmpDef.Name = strings.Replace(tmpDef.Name, "/", "_", 1)
				tmpDef.Name = strings.Replace(tmpDef.Name, "/", "-", 1) // should give internal_message:remove
				genInput.Options.Tools = append(genInput.Options.Tools, llm.Tool{Type: "function", Definition: tmpDef})
			}
		}
	}
}

func adjustInputIfNeeded(tokenDelta int, overlimit int, genInput *core2.GenerateInput) {

	if tokenDelta > overlimit {
		// Enough savings from rebuilt request; proceed without pruning history.
	} else {
		// Remove the oldest user messages from binding history until we
		// cover the remaining deficit. Operate only on persisted Past
		// turns; Current is reserved for the in-flight step.
		deficit := overlimit - tokenDelta
		if genInput != nil && genInput.Binding != nil && deficit > 0 {
			pruneOldUserMessages(&genInput.Binding.History, deficit)
		}
	}
}

// pruneOldUserMessages removes oldest user messages from History.Past
// until the approximate token deficit is covered. It does not touch
// History.Current.
func pruneOldUserMessages(h *prompt.History, deficit int) {
	if h == nil || deficit <= 0 {
		return
	}
	removedTokens := 0
	for ti := 0; ti < len(h.Past) && removedTokens < deficit; ti++ {
		turn := h.Past[ti]
		if turn == nil || len(turn.Messages) == 0 {
			continue
		}
		kept := make([]*prompt.Message, 0, len(turn.Messages))
		for _, m := range turn.Messages {
			if m == nil {
				continue
			}
			role := strings.ToLower(strings.TrimSpace(m.Role))
			if removedTokens < deficit && role == "user" && strings.TrimSpace(m.Content) != "" {
				removedTokens += estimateTokens(m.Content)
				continue
			}
			kept = append(kept, m)
		}
		turn.Messages = kept
	}
}

func (s *Service) computeTokenDiff(ctx context.Context, genInput *core2.GenerateInput, oldGenInput *core2.GenerateInput) (int, error) {

	newInput := *genInput
	err := newInput.Init(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to init generate input for token diff: %v\n", err)
	}

	toolDiffInTokens := toolDiff(oldGenInput, &newInput)

	msgDiffInTokens := msgDiff(oldGenInput, &newInput)

	return toolDiffInTokens + msgDiffInTokens, nil
}

func msgDiff(oldInput *core2.GenerateInput, newInput *core2.GenerateInput) int {
	oldMsgByteSize := 0
	for _, m := range oldInput.Message {
		oldMsgByteSize += len(m.Content)
	}

	newMsgByteSize := 0
	for _, m := range newInput.Message {
		newMsgByteSize += len(m.Content)
	}

	msgBytesDiff := oldMsgByteSize - newMsgByteSize
	msgDiffInTokens := 0
	if msgBytesDiff > 0 {
		msgDiffInTokens = estimateTokensInt(msgBytesDiff)
	} else {
		msgDiffInTokens = estimateTokensInt(-1*msgBytesDiff) * -1
	}
	return msgDiffInTokens
}

func toolDiff(oldInput *core2.GenerateInput, newInput *core2.GenerateInput) int {
	// Compare tool definitions between the original failed request and the rebuilt input
	oldToolsBytes := 0
	if oldInput != nil && oldInput.Options != nil && len(oldInput.Options.Tools) > 0 {
		if data, tErr := json.Marshal(oldInput.Options.Tools); tErr == nil {
			oldToolsBytes = len(data)
		}
	}

	newToolsBytes := 0
	if newInput.Options != nil && len(newInput.Options.Tools) > 0 {
		if data, tErr := json.Marshal(newInput.Options.Tools); tErr == nil {
			newToolsBytes = len(data)
		}
	}

	toolBytesDiff := oldToolsBytes - newToolsBytes
	toolDiffInTokens := 0
	if toolBytesDiff > 0 {
		toolDiffInTokens = estimateTokensInt(toolBytesDiff)
	} else {
		toolDiffInTokens = estimateTokensInt(-1*toolBytesDiff) * -1
	}
	return toolDiffInTokens
}

// stripSystemMessages removes system-originated inputs from a GenerateInput so that
// freeMessageTokensLLM can operate on a reduced context. It clears the SystemPrompt
// and any SystemDocuments attached via the binding. It also prunes any pre-populated
// system-role entries in Binding.History (defensive; history normally holds user/assistant only).
func (s *Service) stripSystemMessages(in *core2.GenerateInput) {
	if in == nil {
		return
	}
	// Remove explicit system prompt
	in.SystemPrompt = nil
	// Remove system documents
	if in.Binding != nil {
		in.Binding.SystemDocuments.Items = nil
		// Prune any system-role messages from persisted history.
		if len(in.Binding.History.Past) > 0 {
			for _, t := range in.Binding.History.Past {
				if t == nil || len(t.Messages) == 0 {
					continue
				}
				kept := make([]*prompt.Message, 0, len(t.Messages))
				for _, m := range t.Messages {
					if m == nil {
						continue
					}
					if strings.EqualFold(strings.TrimSpace(m.Role), "system") {
						continue
					}
					kept = append(kept, m)
				}
				t.Messages = kept
			}
		}
	}
	// If Message was already initialized elsewhere, prune any system messages
	if len(in.Message) > 0 {
		filtered := make([]llm.Message, 0, len(in.Message))
		for _, m := range in.Message {
			if strings.EqualFold(string(m.Role), "system") {
				continue
			}
			filtered = append(filtered, m)
		}
		in.Message = filtered
	}
}

// composeFreeTokenPrompt renders the context-limit guidance template with the error
// message and candidate list. It does not mutate the embedded template.
func (s *Service) composeFreeTokenPrompt(errMessage string, lines []string, ids []string) string {
	tpl := freeTokenPrompt
	tpl = strings.Replace(tpl, "{{ERROR_MESSAGE}}", errMessage, 1)
	tpl = strings.ReplaceAll(tpl, "{{REMOVE_MIN}}", strconv.Itoa(pruneMinRemove))
	tpl = strings.ReplaceAll(tpl, "{{REMOVE_MAX}}", strconv.Itoa(pruneMaxRemove))
	var buf bytes.Buffer
	if len(ids) > 0 {
		buf.WriteString("The following message IDs are provided inside a fenced code block.\n")
		buf.WriteString("Copy them exactly in tool args; do not alter formatting.\n\n")
		buf.WriteString("```text\n")
		for _, id := range ids {
			buf.WriteString(id)
			buf.WriteByte('\n')
		}
		buf.WriteString("```\n\n")
		buf.WriteString("Candidates for removal:\n")
	}
	for _, l := range lines {
		buf.WriteString(l)
		buf.WriteByte('\n')
	}
	return strings.Replace(tpl, "{{CANDIDATES}}", buf.String(), 1)
}

func (s *Service) composeCompactPrompt(errMessage string, lines []string, ids []string) string {
	var buf bytes.Buffer
	buf.WriteString("The last LLM call failed due to context overflow. Here is the exact error:\n")
	buf.WriteString("ERROR_MESSAGE: ")
	buf.WriteString(errMessage)
	buf.WriteString("\n\n")
	buf.WriteString("You must produce a compact handoff summary following these rules:\n\n")
	buf.WriteString(prompts.Compact)
	buf.WriteString("\n\n")
	if len(ids) > 0 {
		buf.WriteString("The following message IDs are provided inside a fenced code block.\n")
		buf.WriteString("Copy them exactly in tool args; do not alter formatting.\n\n")
		buf.WriteString("```text\n")
		for _, id := range ids {
			buf.WriteString(id)
			buf.WriteByte('\n')
		}
		buf.WriteString("```\n\n")
		buf.WriteString("Candidates for removal:\n")
	}
	for _, l := range lines {
		buf.WriteString(l)
		buf.WriteByte('\n')
	}
	buf.WriteString("\n")
	buf.WriteString("Use the candidates above to select messages for removal and replace them with a single compact summary.\n")
	buf.WriteString("Return ONLY a call to function tool \"internal_message-remove\" with:\n")
	buf.WriteString("```json\n")
	buf.WriteString("{\n")
	buf.WriteString("  \"tuples\": [\n")
	buf.WriteString("    {\n")
	buf.WriteString("      \"messageIds\": [\"<id1>\", \"<id2>\", ...],\n")
	buf.WriteString("      \"role\": \"assistant\",\n")
	buf.WriteString("      \"summary\": \"<handoff summary following the required sections>\"\n")
	buf.WriteString("    }\n")
	buf.WriteString("  ]\n")
	buf.WriteString("}\n")
	buf.WriteString("```\n")
	buf.WriteString("Output only the tool call, no additional text.\n")
	return buf.String()
}

// extractOverlimitTokens tries to compute how many tokens over the limit the request was,
// based on provider error messages. It supports common phrases from OpenAI-like errors.
func extractOverlimitTokens(msg string) (int, bool) {
	s := strings.TrimSpace(msg)
	if s == "" {
		return 0, false
	}
	// Pattern: "maximum context length is <max> tokens ... requested <req> tokens"
	re := regexp.MustCompile(`(?i)maximum\s+context\s+length\s+is\s+(\d+)\s+tokens[\s\S]*?requested\s+(\d+)\s+tokens`)
	if m := re.FindStringSubmatch(s); len(m) == 3 {
		maxTok, _ := strconv.Atoi(m[1])
		reqTok, _ := strconv.Atoi(m[2])
		if maxTok > 0 && reqTok > 0 {
			if reqTok > maxTok {
				return reqTok - maxTok, true
			}
			return 0, true
		}
	}
	// Alternate: "context window ... is/of <max> tokens ... requested <req> tokens"
	re2 := regexp.MustCompile(`(?i)context\s+window[\s\S]*?(?:is|of)\s+(\d+)\s+tokens[\s\S]*?requested\s+(\d+)\s+tokens`)
	if m := re2.FindStringSubmatch(s); len(m) == 3 {
		maxTok, _ := strconv.Atoi(m[1])
		reqTok, _ := strconv.Atoi(m[2])
		if maxTok > 0 && reqTok > 0 {
			if reqTok > maxTok {
				return reqTok - maxTok, true
			}
			return 0, true
		}
	}

	return 0, false
}
