package modelcallctx

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	apiconv "github.com/viant/agently/client/conversation"
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

func TestCloseIfOpen_ClosesStartedModelCall(t *testing.T) {
	cases := []struct {
		name           string
		cancelContext  bool
		expectedStatus string
	}{
		{name: "failed fallback", cancelContext: false, expectedStatus: "failed"},
		{name: "canceled fallback", cancelContext: true, expectedStatus: "canceled"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := convmem.New()
			base := memory.WithConversationID(context.Background(), "conv-1")
			if err := client.PatchConversations(base, convw.NewConversationStatus("conv-1", "")); err != nil {
				t.Fatalf("failed to seed conversation: %v", err)
			}

			runCtx := base
			var cancel func()
			if tc.cancelContext {
				runCtx, cancel = context.WithCancel(base)
			}
			if cancel != nil {
				defer cancel()
			}

			ctx := WithRecorderObserver(runCtx, client)
			ob := ObserverFromContext(ctx)
			if ob == nil {
				t.Fatalf("observer not injected")
			}
			ctx2, err := ob.OnCallStart(ctx, Info{Provider: "test", Model: "test-model", LLMRequest: &llm.GenerateRequest{Options: &llm.Options{Mode: "chat"}}})
			if err != nil {
				t.Fatalf("OnCallStart error: %v", err)
			}
			if tc.cancelContext && cancel != nil {
				cancel()
			}

			if err := CloseIfOpen(ctx2, Info{Err: "forced close", CompletedAt: time.Now()}); err != nil {
				t.Fatalf("CloseIfOpen error: %v", err)
			}

			msgID := memory.ModelMessageIDFromContext(ctx2)
			if msgID == "" {
				t.Fatalf("message id not set in context")
			}
			msg, err := client.GetMessage(context.Background(), msgID)
			if err != nil || msg == nil || msg.ModelCall == nil {
				t.Fatalf("missing model call after CloseIfOpen: %v", err)
			}
			assert.EqualValues(t, tc.expectedStatus, msg.ModelCall.Status)
		})
	}
}

func TestOnCallEnd_DoesNotPatchConversationWhenFinishModelCallFails(t *testing.T) {
	baseClient := convmem.New()
	base := memory.WithConversationID(context.Background(), "conv-1")
	if err := baseClient.PatchConversations(base, convw.NewConversationStatus("conv-1", "")); err != nil {
		t.Fatalf("failed to seed conversation: %v", err)
	}

	client := &failingPayloadClient{
		Client:      baseClient,
		failAtCount: 2, // first payload in OnCallStart, second in OnCallEnd
	}
	ctx := WithRecorderObserver(base, client)
	ob := ObserverFromContext(ctx)
	if ob == nil {
		t.Fatalf("observer not injected")
	}
	ctx2, err := ob.OnCallStart(ctx, Info{
		Provider:   "test",
		Model:      "test-model",
		LLMRequest: &llm.GenerateRequest{Options: &llm.Options{Mode: "chat"}},
	})
	if err != nil {
		t.Fatalf("OnCallStart error: %v", err)
	}

	endErr := ob.OnCallEnd(ctx2, Info{
		Model:        "test-model",
		LLMResponse:  &llm.GenerateResponse{},
		ResponseJSON: []byte(`{"response":{"id":"r1"}}`),
	})
	if endErr == nil {
		t.Fatalf("expected OnCallEnd error")
	}
	assert.Contains(t, strings.ToLower(endErr.Error()), "finish model call")
	assert.EqualValues(t, 0, client.patchConversationCount)
}

func TestRecorderObserver_OnStreamDelta_IgnoresCanceledPersistenceAndFinalizesAccumulatedStream(t *testing.T) {
	t.Setenv(streamPersistModeEnv, "")

	baseClient := convmem.New()
	base := memory.WithConversationID(context.Background(), "conv-1")
	if err := baseClient.PatchConversations(base, convw.NewConversationStatus("conv-1", "")); err != nil {
		t.Fatalf("failed to seed conversation: %v", err)
	}

	runCtx, cancel := context.WithCancel(base)
	client := &cancelAwarePayloadClient{Client: baseClient}
	ctx := WithRecorderObserver(runCtx, client)
	ob := ObserverFromContext(ctx)
	if ob == nil {
		t.Fatalf("observer not injected")
	}

	ctx2, err := ob.OnCallStart(ctx, Info{
		Provider:   "test",
		Model:      "test-model",
		LLMRequest: &llm.GenerateRequest{Options: &llm.Options{Mode: "chat"}},
	})
	if err != nil {
		t.Fatalf("OnCallStart error: %v", err)
	}

	cancel()

	if err := ob.OnStreamDelta(ctx2, []byte("Hello")); err != nil {
		t.Fatalf("OnStreamDelta first chunk error: %v", err)
	}
	if err := ob.OnStreamDelta(ctx2, []byte(" world")); err != nil {
		t.Fatalf("OnStreamDelta second chunk error: %v", err)
	}

	if err := CloseIfOpen(ctx2, Info{CompletedAt: time.Now()}); err != nil {
		t.Fatalf("CloseIfOpen error: %v", err)
	}

	msgID := memory.ModelMessageIDFromContext(ctx2)
	if msgID == "" {
		t.Fatalf("message id not set in context")
	}
	msg, err := baseClient.GetMessage(context.Background(), msgID)
	if err != nil || msg == nil || msg.ModelCall == nil {
		t.Fatalf("missing model call after CloseIfOpen: %v", err)
	}
	assert.EqualValues(t, "canceled", msg.ModelCall.Status)
	if msg.ModelCall.StreamPayloadId == nil || strings.TrimSpace(*msg.ModelCall.StreamPayloadId) == "" {
		t.Fatalf("expected stream payload id")
	}
	payload, err := baseClient.GetPayload(context.Background(), *msg.ModelCall.StreamPayloadId)
	if err != nil || payload == nil || payload.InlineBody == nil {
		t.Fatalf("missing stream payload: %v", err)
	}
	assert.Equal(t, "Hello world", string(*payload.InlineBody))
}

func TestRecorderObserver_OnStreamDelta_BufferedModeFlushesOnInterval(t *testing.T) {
	t.Setenv(streamPersistModeEnv, "buffered")

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
	recorder, ok := ob.(*recorderObserver)
	if !ok {
		t.Fatalf("expected recorderObserver, got %T", ob)
	}

	ctx2, err := ob.OnCallStart(ctx, Info{
		Provider:   "test",
		Model:      "test-model",
		LLMRequest: &llm.GenerateRequest{Options: &llm.Options{Mode: "chat"}},
	})
	if err != nil {
		t.Fatalf("OnCallStart error: %v", err)
	}

	if err := ob.OnStreamDelta(ctx2, []byte("Hello")); err != nil {
		t.Fatalf("OnStreamDelta first chunk error: %v", err)
	}
	if strings.TrimSpace(recorder.streamPayloadID) == "" {
		t.Fatalf("expected buffered stream payload id")
	}
	payload, err := client.GetPayload(context.Background(), recorder.streamPayloadID)
	if err != nil {
		t.Fatalf("GetPayload error: %v", err)
	}
	if payload != nil {
		t.Fatalf("expected buffered mode to defer first payload write")
	}

	recorder.lastFlushAt = time.Now().Add(-streamPersistBufferedInterval)
	if err := ob.OnStreamDelta(ctx2, []byte(" world")); err != nil {
		t.Fatalf("OnStreamDelta second chunk error: %v", err)
	}

	payload, err = client.GetPayload(context.Background(), recorder.streamPayloadID)
	if err != nil || payload == nil || payload.InlineBody == nil {
		t.Fatalf("missing buffered stream payload: %v", err)
	}
	assert.Equal(t, "Hello world", string(*payload.InlineBody))

	msgID := memory.ModelMessageIDFromContext(ctx2)
	if msgID == "" {
		t.Fatalf("message id not set in context")
	}
	msg, err := client.GetMessage(context.Background(), msgID)
	if err != nil || msg == nil || msg.ModelCall == nil {
		t.Fatalf("missing model call after buffered flush: %v", err)
	}
	if msg.ModelCall.StreamPayloadId == nil || strings.TrimSpace(*msg.ModelCall.StreamPayloadId) == "" {
		t.Fatalf("expected linked stream payload after buffered flush")
	}
	assert.Equal(t, recorder.streamPayloadID, strings.TrimSpace(*msg.ModelCall.StreamPayloadId))
}

func TestRecorderObserver_OnStreamDelta_ContextOverrideBufferedModeWinsOverEnv(t *testing.T) {
	t.Setenv(streamPersistModeEnv, "final")

	client := convmem.New()
	base := memory.WithConversationID(context.Background(), "conv-1")
	base = WithStreamPersistMode(base, "buffered")
	if err := client.PatchConversations(base, convw.NewConversationStatus("conv-1", "")); err != nil {
		t.Fatalf("failed to seed conversation: %v", err)
	}

	ctx := WithRecorderObserver(base, client)
	ob := ObserverFromContext(ctx)
	if ob == nil {
		t.Fatalf("observer not injected")
	}
	recorder, ok := ob.(*recorderObserver)
	if !ok {
		t.Fatalf("expected recorderObserver, got %T", ob)
	}

	ctx2, err := ob.OnCallStart(ctx, Info{
		Provider:   "test",
		Model:      "test-model",
		LLMRequest: &llm.GenerateRequest{Options: &llm.Options{Mode: "chat"}},
	})
	if err != nil {
		t.Fatalf("OnCallStart error: %v", err)
	}

	if err := ob.OnStreamDelta(ctx2, []byte("Hello")); err != nil {
		t.Fatalf("OnStreamDelta first chunk error: %v", err)
	}
	if strings.TrimSpace(recorder.streamPayloadID) == "" {
		t.Fatalf("expected buffered stream payload id")
	}
	payload, err := client.GetPayload(context.Background(), recorder.streamPayloadID)
	if err != nil {
		t.Fatalf("GetPayload error: %v", err)
	}
	if payload != nil {
		t.Fatalf("expected buffered override to defer first payload write")
	}

	recorder.lastFlushAt = time.Now().Add(-streamPersistBufferedInterval)
	if err := ob.OnStreamDelta(ctx2, []byte(" world")); err != nil {
		t.Fatalf("OnStreamDelta second chunk error: %v", err)
	}

	payload, err = client.GetPayload(context.Background(), recorder.streamPayloadID)
	if err != nil || payload == nil || payload.InlineBody == nil {
		t.Fatalf("missing buffered stream payload: %v", err)
	}
	assert.Equal(t, "Hello world", string(*payload.InlineBody))
}

type failingPayloadClient struct {
	apiconv.Client
	failAtCount            int
	callCount              int
	patchConversationCount int
}

func (f *failingPayloadClient) PatchPayload(ctx context.Context, payload *apiconv.MutablePayload) error {
	f.callCount++
	if f.failAtCount > 0 && f.callCount == f.failAtCount {
		return fmt.Errorf("simulated payload patch failure")
	}
	return f.Client.PatchPayload(ctx, payload)
}

func (f *failingPayloadClient) PatchConversations(ctx context.Context, conversations *apiconv.MutableConversation) error {
	f.patchConversationCount++
	return f.Client.PatchConversations(ctx, conversations)
}

type cancelAwarePayloadClient struct {
	apiconv.Client
}

func (c *cancelAwarePayloadClient) PatchPayload(ctx context.Context, payload *apiconv.MutablePayload) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return c.Client.PatchPayload(ctx, payload)
}
