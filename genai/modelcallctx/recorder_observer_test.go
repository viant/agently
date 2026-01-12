package modelcallctx

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/memory"
	convmem "github.com/viant/agently/internal/service/conversation/memory"
	convw "github.com/viant/agently/pkg/agently/conversation/write"
)

// TestFinishModelCallSetsCost_DataDriven verifies cost calculation is stored
// with per-1k pricing using a data-driven table of scenarios.
func TestFinishModelCallSetsCost_DataDriven(t *testing.T) {
	type tc struct {
		name   string
		model  string
		inP    float64
		outP   float64
		cacheP float64
		pt     int
		ct     int
		cached int
	}

	cases := []tc{
		{
			name:   "openai o3",
			model:  "openai_o3",
			inP:    0.002, // $2 per 1M → 0.002 per 1k
			outP:   0.008, // $8 per 1M → 0.008 per 1k
			cacheP: 0,
			pt:     1000,
			ct:     500,
			cached: 0,
		},
		{
			name:   "bedrock claude 4-5 with cache",
			model:  "bedrock_claude_4-5",
			inP:    0.003,  // $3 per 1M → 0.003 per 1k
			outP:   0.015,  // $15 per 1M → 0.015 per 1k
			cacheP: 0.0003, // 10% of input per 1k (≈ $0.30 per 1M)
			pt:     200,
			ct:     300,
			cached: 100,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// In-memory conversation client
			client := convmem.New()

			// Conversation context with id
			base := memory.WithConversationID(context.Background(), "conv-1")
			// Price provider returns per-1k prices
			provider := staticPriceProvider{model: c.model, inP: c.inP, outP: c.outP, cacheP: c.cacheP}
			// Ensure conversation exists in the client store
			if err := client.PatchConversations(base, convw.NewConversationStatus("conv-1", "")); err != nil {
				t.Fatalf("failed to seed conversation: %v", err)
			}

			ctx := WithRecorderObserverWithPrice(base, client, provider)

			// Start the call and capture ctx with message id set
			ob := ObserverFromContext(ctx)
			if ob == nil {
				t.Fatalf("observer not injected")
			}
			ctx2, err := ob.OnCallStart(ctx, Info{Provider: "test", Model: c.model, LLMRequest: &llm.GenerateRequest{Options: &llm.Options{Mode: "chat"}}})
			if err != nil {
				t.Fatalf("OnCallStart error: %v", err)
			}

			// Finish the call with usage
			usage := &llm.Usage{PromptTokens: c.pt, CompletionTokens: c.ct, CachedTokens: c.cached}
			if err := ob.OnCallEnd(ctx2, Info{Model: c.model, LLMResponse: &llm.GenerateResponse{}, Usage: usage}); err != nil {
				t.Fatalf("OnCallEnd error: %v", err)
			}

			// Fetch message and verify stored cost
			msgID := memory.ModelMessageIDFromContext(ctx2)
			if msgID == "" {
				t.Fatalf("message id not set in context")
			}
			msg, err := client.GetMessage(context.Background(), msgID)
			if err != nil || msg == nil || msg.ModelCall == nil || msg.ModelCall.Cost == nil {
				t.Fatalf("missing model call cost: %v", err)
			}

			// Expected cost formula with per-1k prices
			expected := (float64(c.pt)*c.inP + float64(c.ct)*c.outP + float64(c.cached)*c.cacheP) / 1000.0
			assert.EqualValues(t, expected, *msg.ModelCall.Cost)
		})
	}
}

type staticPriceProvider struct {
	model             string
	inP, outP, cacheP float64
}

func (s staticPriceProvider) TokenPrices(model string) (float64, float64, float64, bool) {
	if model == s.model {
		return s.inP, s.outP, s.cacheP, true
	}
	return 0, 0, 0, false
}

func TestRecorderObserver_PersistsAssistantContent_DataDriven(t *testing.T) {
	type testCase struct {
		name          string
		resp          *llm.GenerateResponse
		responseJSON  []byte
		expected      string
		expectRaw     bool
		expectInterim int
	}

	cases := []testCase{
		{
			name:     "content field",
			resp:     &llm.GenerateResponse{Choices: []llm.Choice{{Message: llm.Message{Role: llm.RoleAssistant, Content: "hello"}}}},
			expected: "hello",
		},
		{
			name:     "content items",
			resp:     &llm.GenerateResponse{Choices: []llm.Choice{{Message: llm.Message{Role: llm.RoleAssistant, ContentItems: []llm.ContentItem{{Type: llm.ContentTypeText, Text: "from items"}}}}}},
			expected: "from items",
		},
		{
			name:          "tool calls store raw_content",
			resp:          &llm.GenerateResponse{Choices: []llm.Choice{{Message: llm.Message{Role: llm.RoleAssistant, Content: "plan", ToolCalls: []llm.ToolCall{{ID: "call_1", Name: "resources-roots"}}}}}},
			expected:      "plan",
			expectRaw:     true,
			expectInterim: 1,
		},
		{
			name: "response json fallback",
			responseJSON: func() []byte {
				raw, _ := json.Marshal(&llm.GenerateResponse{Choices: []llm.Choice{{Message: llm.Message{Role: llm.RoleAssistant, Content: "from json"}}}})
				return raw
			}(),
			expected: "from json",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := convmem.New()
			base := memory.WithConversationID(context.Background(), "conv-1")
			if err := client.PatchConversations(base, convw.NewConversationStatus("conv-1", "")); err != nil {
				t.Fatalf("failed to seed conversation: %v", err)
			}

			ctx := WithRecorderObserver(base, client)
			ob := ObserverFromContext(ctx)
			if ob == nil {
				t.Fatalf("observer not injected")
			}
			ctx2, err := ob.OnCallStart(ctx, Info{Provider: "test", Model: "test-model", LLMRequest: &llm.GenerateRequest{Options: &llm.Options{Mode: "chat"}}})
			if err != nil {
				t.Fatalf("OnCallStart error: %v", err)
			}

			if err := ob.OnCallEnd(ctx2, Info{Model: "test-model", LLMResponse: tc.resp, ResponseJSON: tc.responseJSON}); err != nil {
				t.Fatalf("OnCallEnd error: %v", err)
			}

			msgID := memory.ModelMessageIDFromContext(ctx2)
			if msgID == "" {
				t.Fatalf("message id not set in context")
			}
			msg, err := client.GetMessage(context.Background(), msgID)
			if err != nil || msg == nil {
				t.Fatalf("failed to fetch message: %v", err)
			}
			actualContent := ""
			if msg.Content != nil {
				actualContent = *msg.Content
			}
			actualRaw := ""
			if msg.RawContent != nil {
				actualRaw = *msg.RawContent
			}
			assert.EqualValues(t, tc.expected, actualContent)
			if tc.expectRaw {
				assert.EqualValues(t, tc.expected, actualRaw)
			} else {
				assert.EqualValues(t, "", actualRaw)
			}
			assert.EqualValues(t, tc.expectInterim, msg.Interim)
		})
	}
}
