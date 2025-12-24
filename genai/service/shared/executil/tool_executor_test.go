package executil

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/memory"
)

func TestResolveToolStatus_DataDriven(t *testing.T) {
	type testCase struct {
		name     string
		err      error
		parentFn func() context.Context
		expected string
	}
	cases := []testCase{
		{name: "success", err: nil, parentFn: context.Background, expected: "completed"},
		{name: "canceled-by-parent", err: nil, parentFn: func() context.Context { ctx, cancel := context.WithCancel(context.Background()); cancel(); return ctx }, expected: "canceled"},
		{name: "exec-error", err: assert.AnError, parentFn: context.Background, expected: "failed"},
		{name: "exec-canceled", err: context.Canceled, parentFn: context.Background, expected: "canceled"},
		{name: "exec-deadline", err: context.DeadlineExceeded, parentFn: context.Background, expected: "canceled"},
	}
	for _, tc := range cases {
		ctx := tc.parentFn()
		got, _ := resolveToolStatus(tc.err, ctx)
		assert.EqualValues(t, tc.expected, got, tc.name)
	}
}

func TestToolExecContext_Timeout(t *testing.T) {
	// 50ms timeout
	_ = os.Setenv("AGENTLY_TOOLCALL_TIMEOUT", "50ms")
	defer os.Unsetenv("AGENTLY_TOOLCALL_TIMEOUT")
	ctx := context.Background()
	execCtx, cancel := toolExecContext(ctx)
	defer cancel()
	select {
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("expected timeout before 200ms")
	case <-execCtx.Done():
		// expected
		assert.Error(t, execCtx.Err())
	}
}

func TestMaybePersistSystemDocuments(t *testing.T) {
	turn := memory.TurnMeta{ConversationID: "c1", TurnID: "t1", Assistant: "agent-test"}
	result := `{"documents":[{"uri":"workspace://localhost/sys/doc.md","score":0.91},{"uri":"workspace://localhost/user/doc.md","score":0.42}]}`
	conv := &stubConv{}
	reg := newStubRegistry(map[string]string{
		"root-sys|doc.md":  "# System Playbook\nDo X\n",
		"root-user|doc.md": "user notes",
	})
	assert.NoError(t, persistDocumentsIfNeeded(context.Background(), reg, conv, turn, "resources.matchDocuments", result))
	require.Len(t, conv.insertedMessages, 2)
	msg := conv.insertedMessages[0]
	assert.Equal(t, "system", msg.Role)
	assert.Equal(t, "c1", msg.ConversationID)
	assert.Contains(t, msg.Content, "System Playbook")
	msgUser := conv.insertedMessages[1]
	assert.Equal(t, "user", msgUser.Role)
	assert.Contains(t, msgUser.Content, "user notes")

	// metadata patch for system doc
	var meta *apiconv.MutableMessage
	for _, patched := range conv.patchedMessages {
		if patched != nil && strings.EqualFold(patched.Mode, SystemDocumentMode) {
			meta = patched
			break
		}
	}
	require.NotNil(t, meta)
	assert.Equal(t, SystemDocumentMode, meta.Mode)
	assert.Equal(t, SystemDocumentTag, derefString(meta.Tags))
	assert.Equal(t, "workspace://localhost/sys/doc.md", derefString(meta.ContextSummary))

	conv2 := &stubConv{}
	assert.NoError(t, persistDocumentsIfNeeded(context.Background(), reg, conv2, turn, "resources.matchDocuments", `{"documents":[{"uri":"workspace://localhost/unknown/foo","score":0.1}]}`))
	assert.Len(t, conv2.insertedMessages, 0)

	conv3 := &stubConv{}
	assert.NoError(t, persistDocumentsIfNeeded(context.Background(), reg, conv3, turn, "resources.matchdocuments", "false"))
	assert.Len(t, conv3.insertedMessages, 0)

	convHyphen := &stubConv{}
	assert.NoError(t, persistDocumentsIfNeeded(context.Background(), reg, convHyphen, turn, "resources-matchDocuments", result))
	assert.Len(t, convHyphen.insertedMessages, 2)
}

func TestExecuteToolStep_RetryBehavior(t *testing.T) {
	step := StepInfo{
		ID:         "call-1",
		Name:       "flaky.tool",
		Args:       map[string]interface{}{"foo": "bar"},
		ResponseID: "resp-1",
	}
	cases := []struct {
		name              string
		script            []scriptedResult
		expectedAttempts  int
		expectError       bool
		thresholdOverride time.Duration
	}{
		{
			name: "retry-on-context-canceled",
			script: []scriptedResult{
				{result: "", err: context.Canceled},
				{result: `{"status":"ok"}`, err: nil},
			},
			expectedAttempts: 2,
			expectError:      false,
		},
		{
			name: "no-retry-on-non-context-error",
			script: []scriptedResult{
				{result: "", err: fmt.Errorf("invalid request")},
			},
			expectedAttempts: 1,
			expectError:      true,
		},
		{
			name: "no-retry-when-duration-exceeds-threshold",
			script: []scriptedResult{
				{result: "", err: context.Canceled, delay: 20 * time.Millisecond},
			},
			expectedAttempts:  1,
			expectError:       true,
			thresholdOverride: 10 * time.Millisecond,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			turn := memory.TurnMeta{ConversationID: "c-retry", TurnID: "t-retry", ParentMessageID: "p-retry"}
			ctx := memory.WithTurnMeta(context.Background(), turn)
			conv := &stubConv{}
			reg := &scriptedRegistry{script: tc.script}
			if tc.thresholdOverride > 0 {
				original := maxRetryDuration
				maxRetryDuration = tc.thresholdOverride
				t.Cleanup(func() { maxRetryDuration = original })
			}
			call, _, err := ExecuteToolStep(ctx, reg, step, conv)
			assert.EqualValues(t, tc.expectedAttempts, reg.calls)
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			if len(tc.script) > 0 {
				attemptIdx := reg.calls - 1
				if attemptIdx >= len(tc.script) {
					attemptIdx = len(tc.script) - 1
				}
				expectedResult := tc.script[attemptIdx].result
				if strings.TrimSpace(expectedResult) == "" && tc.script[attemptIdx].err != nil {
					expectedResult = tc.script[attemptIdx].err.Error()
				}
				assert.EqualValues(t, expectedResult, call.Result)
			}
		})
	}
}

func TestExecuteToolStep_PersistsReadImageAsAttachment(t *testing.T) {
	turn := memory.TurnMeta{ConversationID: "c-img", TurnID: "t-img", ParentMessageID: "p-img", Assistant: "agent-test"}
	ctx := memory.WithTurnMeta(context.Background(), turn)
	reg := &scriptedRegistry{script: []scriptedResult{{
		result: `{"uri":"file:///tmp/img.png","mimeType":"image/png","dataBase64":"AQID","name":"img.png"}`,
	}}}
	conv := &stubConv{}

	step := StepInfo{
		ID:         "call-img",
		Name:       "resources.readImage",
		Args:       map[string]interface{}{"path": "img.png"},
		ResponseID: "resp-1",
	}
	_, _, err := ExecuteToolStep(ctx, reg, step, conv)
	require.NoError(t, err)

	var sawAttachmentPayload bool
	for _, p := range conv.patchedPayloads {
		if p == nil || p.Has == nil || !p.Has.Kind {
			continue
		}
		if p.Kind == "attachment" && strings.EqualFold(p.MimeType, "image/png") {
			sawAttachmentPayload = true
			if p.InlineBody != nil {
				assert.EqualValues(t, []byte{1, 2, 3}, []byte(*p.InlineBody))
			}
		}
	}
	assert.EqualValues(t, true, sawAttachmentPayload)

	var sawLink bool
	for _, m := range conv.patchedMessages {
		if m == nil || m.Has == nil || !m.Has.AttachmentPayloadID {
			continue
		}
		sawLink = true
		break
	}
	assert.EqualValues(t, true, sawLink)
}

type stubConv struct {
	patchedMessages  []*apiconv.MutableMessage
	insertedMessages []*apiconv.MutableMessage
	patchedPayloads  []*apiconv.MutablePayload
}

func (s *stubConv) GetConversation(context.Context, string, ...apiconv.Option) (*apiconv.Conversation, error) {
	return nil, nil
}

func (s *stubConv) GetConversations(context.Context, *apiconv.Input) ([]*apiconv.Conversation, error) {
	return nil, nil
}

func (s *stubConv) PatchConversations(context.Context, *apiconv.MutableConversation) error {
	return nil
}

func (s *stubConv) GetPayload(context.Context, string) (*apiconv.Payload, error) {
	return nil, nil
}

func (s *stubConv) PatchPayload(_ context.Context, payload *apiconv.MutablePayload) error {
	s.patchedPayloads = append(s.patchedPayloads, payload)
	return nil
}

func (s *stubConv) PatchMessage(_ context.Context, message *apiconv.MutableMessage) error {
	s.patchedMessages = append(s.patchedMessages, message)
	if message != nil && strings.TrimSpace(message.Content) != "" {
		s.insertedMessages = append(s.insertedMessages, message)
	}
	return nil
}

func (s *stubConv) GetMessage(context.Context, string, ...apiconv.Option) (*apiconv.Message, error) {
	return nil, nil
}

func (s *stubConv) GetMessageByElicitation(context.Context, string, string) (*apiconv.Message, error) {
	return nil, nil
}

func (s *stubConv) PatchModelCall(context.Context, *apiconv.MutableModelCall) error {
	return nil
}

func (s *stubConv) PatchToolCall(context.Context, *apiconv.MutableToolCall) error {
	return nil
}

func (s *stubConv) PatchTurn(context.Context, *apiconv.MutableTurn) error {
	return nil
}

func (s *stubConv) DeleteConversation(context.Context, string) error {
	return nil
}

func (s *stubConv) DeleteMessage(context.Context, string, string) error {
	return nil
}

func derefString(ptr *string) string {
	if ptr == nil {
		return ""
	}
	return *ptr
}

type stubRegistry struct {
	mu        sync.Mutex
	documents map[string]string
}

type scriptedResult struct {
	result string
	err    error
	delay  time.Duration
}

type scriptedRegistry struct {
	mu     sync.Mutex
	script []scriptedResult
	calls  int
}

func newStubRegistry(documents map[string]string) *stubRegistry {
	return &stubRegistry{
		documents: documents,
	}
}

func (s *stubRegistry) Execute(ctx context.Context, name string, args map[string]interface{}) (string, error) {
	canonical := strings.ToLower(strings.ReplaceAll(name, "_", "."))
	switch canonical {
	case "resources.roots", "resources-roots":
		return `{"roots":[{"id":"root-sys","uri":"workspace://localhost/sys","role":"system"},{"id":"root-user","uri":"workspace://localhost/user","role":"user"}]}`, nil
	case "resources.read", "resources-read":
		rootID := fmt.Sprint(args["rootId"])
		path := fmt.Sprint(args["path"])
		if path == "" && args["uri"] != nil {
			path = fmt.Sprint(args["uri"])
		}
		key := fmt.Sprintf("%s|%s", rootID, path)
		s.mu.Lock()
		content := s.documents[key]
		s.mu.Unlock()
		return fmt.Sprintf(`{"content":%q}`, content), nil
	default:
		return "", fmt.Errorf("unexpected tool: %s", name)
	}
}

func (s *stubRegistry) Definitions() []llm.ToolDefinition                { return nil }
func (s *stubRegistry) MatchDefinition(string) []*llm.ToolDefinition     { return nil }
func (s *stubRegistry) GetDefinition(string) (*llm.ToolDefinition, bool) { return nil, false }
func (s *stubRegistry) MustHaveTools([]string) ([]llm.Tool, error)       { return nil, nil }
func (s *stubRegistry) SetDebugLogger(io.Writer)                         {}
func (s *stubRegistry) Initialize(context.Context)                       {}

func (s *scriptedRegistry) Execute(context.Context, string, map[string]interface{}) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.script) == 0 {
		s.calls++
		return "", nil
	}
	index := s.calls
	s.calls++
	if index >= len(s.script) {
		index = len(s.script) - 1
	}
	entry := s.script[index]
	delay := entry.delay
	if delay > 0 {
		time.Sleep(delay)
	}
	return entry.result, entry.err
}

func (s *scriptedRegistry) Definitions() []llm.ToolDefinition                { return nil }
func (s *scriptedRegistry) MatchDefinition(string) []*llm.ToolDefinition     { return nil }
func (s *scriptedRegistry) GetDefinition(string) (*llm.ToolDefinition, bool) { return nil, false }
func (s *scriptedRegistry) MustHaveTools([]string) ([]llm.Tool, error)       { return nil, nil }
func (s *scriptedRegistry) SetDebugLogger(io.Writer)                         {}
func (s *scriptedRegistry) Initialize(context.Context)                       {}
