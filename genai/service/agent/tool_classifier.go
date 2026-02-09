package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/executor/config"
	"github.com/viant/agently/genai/llm"
	toolbundle "github.com/viant/agently/genai/tool/bundle"
)

type toolBundleSelection struct {
	ToolBundles  []string `json:"toolBundles"`
	ToolBundles2 []string `json:"tool_bundles"`
	Bundles      []string `json:"bundles"`
}

func (s *Service) maybeAutoSelectToolBundles(ctx context.Context, input *QueryInput) {
	if s == nil || input == nil || s.llm == nil || s.llm.ModelFinder() == nil || s.registry == nil {
		return
	}
	// Respect explicit caller selection.
	if len(input.ToolsAllowed) > 0 {
		return
	}
	if len(input.ToolBundles) > 0 {
		return
	}

	enabled := false
	if input.AutoSelectTools != nil {
		enabled = *input.AutoSelectTools
	} else if s.defaults != nil {
		enabled = s.defaults.ToolAutoSelection.Enabled
	}
	if !enabled {
		return
	}
	query := strings.TrimSpace(input.Query)
	if query == "" {
		return
	}
	agentID := strings.TrimSpace(input.AgentID)
	if agentID == "" && input.Agent != nil {
		agentID = strings.TrimSpace(input.Agent.ID)
	}

	conv, _ := s.fetchConversationForRouting(ctx, strings.TrimSpace(input.ConversationID))
	model, _ := s.resolveToolRouterModel(ctx, conv)
	if model == nil {
		return
	}

	bundles := s.availableToolBundles(ctx)
	if len(bundles) == 0 {
		return
	}

	maxBundles := 4
	outputKey := "toolBundles"
	if s.defaults != nil {
		if v := strings.TrimSpace(s.defaults.ToolAutoSelection.OutputKey); v != "" {
			outputKey = v
		}
		if s.defaults.ToolAutoSelection.MaxBundles > 0 {
			maxBundles = s.defaults.ToolAutoSelection.MaxBundles
		}
	}

	candidateByKey := map[string]string{}
	lines := make([]string, 0, len(bundles))
	for _, b := range bundles {
		if b == nil {
			continue
		}
		id := strings.TrimSpace(b.ID)
		if id == "" {
			continue
		}
		candidateByKey[strings.ToLower(id)] = id
		label := id
		if t := strings.TrimSpace(b.Title); t != "" && t != id {
			label = fmt.Sprintf("%s (%s)", id, t)
		}
		desc := strings.TrimSpace(b.Description)
		if desc != "" {
			lines = append(lines, fmt.Sprintf("- %s: %s", label, desc))
		} else {
			lines = append(lines, fmt.Sprintf("- %s", label))
		}
	}
	if len(lines) == 0 {
		return
	}

	systemPrompt := toolRouterSystemPrompt(s.defaults, outputKey, maxBundles)
	user := strings.Join([]string{
		"User request:",
		query,
		"",
		"Available tool bundles:",
		strings.Join(lines, "\n"),
		"",
		"JSON response:",
	}, "\n")

	req := &llm.GenerateRequest{
		Messages: []llm.Message{
			llm.NewSystemMessage(systemPrompt),
			llm.NewUserMessage(user),
		},
		Options: &llm.Options{
			Temperature:      0.0000001,
			MaxTokens:        96,
			JSONMode:         true,
			ResponseMIMEType: "application/json",
			ToolChoice:       llm.NewNoneToolChoice(),
		},
	}
	resp, err := model.Generate(ctx, req)
	if err != nil {
		return
	}
	selected := parseSelectedToolBundles(resp, outputKey)

	// Normalize + validate against candidates.
	out := make([]string, 0, len(selected))
	seen := map[string]struct{}{}
	for _, raw := range selected {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		canonical, ok := candidateByKey[strings.ToLower(id)]
		if !ok {
			continue
		}
		key := strings.ToLower(canonical)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, canonical)
		if maxBundles > 0 && len(out) >= maxBundles {
			break
		}
	}
	if len(out) == 0 {
		return
	}
	input.ToolBundles = out
}

func (s *Service) fetchConversationForRouting(ctx context.Context, conversationID string) (*apiconv.Conversation, error) {
	if s == nil || s.conversation == nil || strings.TrimSpace(conversationID) == "" {
		return nil, nil
	}
	return s.conversation.GetConversation(ctx, strings.TrimSpace(conversationID))
}

func (s *Service) resolveToolRouterModel(ctx context.Context, conv *apiconv.Conversation) (llm.Model, string) {
	if s == nil || s.llm == nil || s.llm.ModelFinder() == nil {
		return nil, ""
	}
	modelName := ""
	if conv != nil && conv.DefaultModel != nil && strings.TrimSpace(*conv.DefaultModel) != "" {
		modelName = strings.TrimSpace(*conv.DefaultModel)
	}
	if modelName == "" && s.defaults != nil {
		modelName = strings.TrimSpace(s.defaults.ToolAutoSelection.Model)
		if modelName == "" {
			modelName = strings.TrimSpace(s.defaults.Model)
		}
	}
	if modelName == "" {
		return nil, ""
	}
	m, err := s.llm.ModelFinder().Find(ctx, modelName)
	if err != nil {
		return nil, modelName
	}
	return m, modelName
}

func (s *Service) availableToolBundles(ctx context.Context) []*toolbundle.Bundle {
	if s == nil || s.registry == nil {
		return nil
	}
	var bundles []*toolbundle.Bundle
	if s.toolBundles != nil {
		if list, err := s.toolBundles(ctx); err == nil && len(list) > 0 {
			bundles = append(bundles, list...)
		}
	}
	if len(bundles) == 0 {
		bundles = toolbundle.DeriveBundles(s.registry.Definitions())
	}
	if len(bundles) == 0 {
		return nil
	}
	sort.Slice(bundles, func(i, j int) bool {
		if bundles[i] == nil || bundles[j] == nil {
			return bundles[j] != nil
		}
		pi, pj := bundles[i].Priority, bundles[j].Priority
		if pi != pj {
			return pi > pj
		}
		return strings.ToLower(strings.TrimSpace(bundles[i].ID)) < strings.ToLower(strings.TrimSpace(bundles[j].ID))
	})
	return bundles
}

func parseSelectedToolBundles(resp *llm.GenerateResponse, outputKey string) []string {
	if resp == nil || len(resp.Choices) == 0 {
		return nil
	}
	content := strings.TrimSpace(resp.Choices[0].Message.Content)
	if content == "" {
		return nil
	}
	content = strings.TrimSpace(strings.TrimPrefix(content, "```json"))
	content = strings.TrimSpace(strings.TrimPrefix(content, "```"))
	content = strings.TrimSpace(strings.TrimSuffix(content, "```"))

	var sel toolBundleSelection
	if json.Unmarshal([]byte(content), &sel) == nil {
		key := strings.ToLower(strings.TrimSpace(outputKey))
		switch key {
		case "", "toolbundles":
			if len(sel.ToolBundles) > 0 {
				return sel.ToolBundles
			}
		case "tool_bundles":
			if len(sel.ToolBundles2) > 0 {
				return sel.ToolBundles2
			}
		case "bundles":
			if len(sel.Bundles) > 0 {
				return sel.Bundles
			}
		}
		if len(sel.ToolBundles) > 0 {
			return sel.ToolBundles
		}
		if len(sel.ToolBundles2) > 0 {
			return sel.ToolBundles2
		}
		if len(sel.Bundles) > 0 {
			return sel.Bundles
		}
	}
	return nil
}

func toolRouterSystemPrompt(defaults *config.Defaults, outputKey string, maxBundles int) string {
	if defaults != nil {
		if v := strings.TrimSpace(defaults.ToolAutoSelection.Prompt); v != "" {
			return v
		}
	}
	key := strings.TrimSpace(outputKey)
	if key == "" {
		key = "toolBundles"
	}
	if maxBundles <= 0 {
		maxBundles = 4
	}
	return strings.Join([]string{
		"You are a tool-bundle router for a developer tool.",
		"Select the minimal set of tool bundle IDs needed to complete the user's request.",
		"Prefer correctness and directness; avoid selecting bundles unrelated to the task.",
		"If the request is purely conversational and no tools are needed, return an empty list.",
		fmt.Sprintf("Return ONLY valid JSON in the form: {\"%s\":[\"<id>\", ...]} with at most %d ids.", key, maxBundles),
		"Do not call tools. Do not return any other keys or text.",
	}, "\n")
}
