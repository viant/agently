package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/prompt"
	stream "github.com/viant/agently/genai/service/core/stream"
)

func TestService_Stream_RetriesOnTransientStreamEventError(t *testing.T) {
	model := &scriptedStreamModel{
		attempts: []streamAttempt{
			{events: []llm.StreamEvent{{Err: fmt.Errorf("OpenAI API error (status 500): server error (type=server_error)")}}},
			{events: []llm.StreamEvent{assistantChunkEvent("ok", "stop")}},
		},
		advisor: retryByContains("status 500"),
	}

	svc := &Service{llmFinder: &fixedFinder{model: model}}
	out := &StreamOutput{}
	cleanup, err := svc.Stream(context.Background(), newStreamInput(registerErrPropagatingHandler()), out)
	require.NoError(t, err)
	require.NotNil(t, cleanup)
	defer cleanup()

	assert.Equal(t, 2, model.calls, "expected one retry after transient stream event error")
	require.NotEmpty(t, out.Events)
	assert.Equal(t, "chunk", out.Events[0].Type)
	assert.Equal(t, "ok", out.Events[0].Content)
	for _, ev := range out.Events {
		assert.NotEqual(t, "error", ev.Type, "retry should clear transient attempt error output")
	}
}

func TestService_Stream_RetriesOnTransientStartError_Non500(t *testing.T) {
	model := &scriptedStreamModel{
		attempts: []streamAttempt{
			{startErr: fmt.Errorf("provider unavailable (status 503)")},
			{events: []llm.StreamEvent{assistantChunkEvent("ok-503", "stop")}},
		},
		advisor: retryByContains("status 503"),
	}

	svc := &Service{llmFinder: &fixedFinder{model: model}}
	out := &StreamOutput{}
	cleanup, err := svc.Stream(context.Background(), newStreamInput(registerErrPropagatingHandler()), out)
	require.NoError(t, err)
	require.NotNil(t, cleanup)
	defer cleanup()

	assert.Equal(t, 2, model.calls)
	assert.Equal(t, "ok-503", out.Events[0].Content)
}

func TestService_Stream_DoesNotRetryOnNonTransientStartError(t *testing.T) {
	model := &scriptedStreamModel{
		attempts: []streamAttempt{
			{startErr: fmt.Errorf("OpenAI API error (status 400): invalid request")},
		},
	}

	svc := &Service{llmFinder: &fixedFinder{model: model}}
	out := &StreamOutput{}
	cleanup, err := svc.Stream(context.Background(), newStreamInput(registerErrPropagatingHandler()), out)
	require.Error(t, err)
	require.NotNil(t, cleanup)
	defer cleanup()

	assert.Contains(t, err.Error(), "failed to start Stream")
	assert.Equal(t, 1, model.calls)
}

func TestService_Stream_RetryExhaustedOnRetryableStartError(t *testing.T) {
	model := &scriptedStreamModel{
		attempts: []streamAttempt{
			{startErr: fmt.Errorf("ThrottlingException")},
			{startErr: fmt.Errorf("ThrottlingException")},
			{startErr: fmt.Errorf("ThrottlingException")},
		},
		advisor: retryByContains("throttling"),
	}

	svc := &Service{llmFinder: &fixedFinder{model: model}}
	out := &StreamOutput{}
	cleanup, err := svc.Stream(context.Background(), newStreamInput(registerErrPropagatingHandler()), out)
	require.Error(t, err)
	require.NotNil(t, cleanup)
	defer cleanup()

	assert.Contains(t, err.Error(), "failed to start Stream")
	assert.Equal(t, 3, model.calls)
}

func TestService_Stream_DoesNotRetryOnContextLimitStartError(t *testing.T) {
	model := &scriptedStreamModel{
		attempts: []streamAttempt{
			{startErr: fmt.Errorf("maximum context length exceeded")},
		},
	}

	svc := &Service{llmFinder: &fixedFinder{model: model}}
	out := &StreamOutput{}
	cleanup, err := svc.Stream(context.Background(), newStreamInput(registerErrPropagatingHandler()), out)
	require.Error(t, err)
	require.NotNil(t, cleanup)
	defer cleanup()

	assert.True(t, errors.Is(err, ErrContextLimitExceeded))
	assert.Equal(t, 1, model.calls)
}

func TestService_Stream_DoesNotRetryWhenConsumeProducedMeaningfulEvents(t *testing.T) {
	model := &scriptedStreamModel{
		attempts: []streamAttempt{
			{
				events: []llm.StreamEvent{
					assistantChunkEvent("partial", ""),
					{Err: fmt.Errorf("provider unavailable (status 503)")},
				},
			},
			{events: []llm.StreamEvent{assistantChunkEvent("should-not-run", "stop")}},
		},
		advisor: retryByContains("status 503"),
	}

	svc := &Service{llmFinder: &fixedFinder{model: model}}
	out := &StreamOutput{}
	cleanup, err := svc.Stream(context.Background(), newStreamInput(registerErrPropagatingHandler()), out)
	require.Error(t, err)
	require.NotNil(t, cleanup)
	defer cleanup()

	assert.Contains(t, err.Error(), "failed to handle Stream event")
	assert.Equal(t, 1, model.calls, "must not retry after meaningful output in failed attempt")
	require.NotEmpty(t, out.Events)
	assert.Equal(t, "chunk", out.Events[0].Type)
	assert.Equal(t, "partial", out.Events[0].Content)
}

func TestService_Stream_RejectsNonEmptyOutputEvents(t *testing.T) {
	svc := &Service{llmFinder: &fixedFinder{model: &scriptedStreamModel{}}}
	out := &StreamOutput{Events: []stream.Event{{Type: "error", Content: "old"}}}

	_, err := svc.Stream(context.Background(), newStreamInput("stream-id"), out)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stream output must be empty at start")
}

func TestCanRetryStreamConsume(t *testing.T) {
	err500 := fmt.Errorf("OpenAI API error (status 500): server error")
	assert.True(t, canRetryStreamConsume(err500, &StreamOutput{}))
	assert.True(t, canRetryStreamConsume(err500, &StreamOutput{Events: []stream.Event{{Type: "error", Content: "boom"}}}))
	assert.False(t, canRetryStreamConsume(err500, &StreamOutput{Events: []stream.Event{{Type: "chunk", Content: "partial"}}}))
}

func TestIsTransientNetworkError_Status500And503(t *testing.T) {
	err500 := fmt.Errorf("failed to handle Stream event: OpenAI API error (status 500): server error (type=server_error)")
	err503 := fmt.Errorf("failed to start Stream: provider unavailable (status 503)")
	assert.True(t, isTransientNetworkError(err500))
	assert.True(t, isTransientNetworkError(err503))
}

func newStreamInput(streamID string) *StreamInput {
	return &StreamInput{
		StreamID: streamID,
		GenerateInput: &GenerateInput{
			ModelSelection: llm.ModelSelection{Model: "mock-model"},
			UserID:         "user-1",
			Prompt:         &prompt.Prompt{Text: "hello"},
			Binding:        &prompt.Binding{},
		},
	}
}

func registerErrPropagatingHandler() string {
	return stream.Register(func(_ context.Context, event *llm.StreamEvent) error {
		if event == nil || event.Response != nil {
			return nil
		}
		// Mirror orchestrator behavior: propagate provider stream errors from events.
		return event.Err
	})
}

func assistantChunkEvent(content, finishReason string) llm.StreamEvent {
	return llm.StreamEvent{
		Response: &llm.GenerateResponse{
			Choices: []llm.Choice{
				{
					Message: llm.Message{
						Role:    llm.RoleAssistant,
						Content: content,
					},
					FinishReason: finishReason,
				},
			},
		},
	}
}

func retryByContains(needle string) func(error, int) (time.Duration, bool) {
	return func(err error, _ int) (time.Duration, bool) {
		if err == nil {
			return 0, false
		}
		if strings.Contains(strings.ToLower(err.Error()), strings.ToLower(needle)) {
			return 0, true
		}
		return 0, false
	}
}

type fixedFinder struct {
	model llm.Model
	err   error
}

func (f *fixedFinder) Find(_ context.Context, _ string) (llm.Model, error) {
	return f.model, f.err
}

type streamAttempt struct {
	startErr error
	events   []llm.StreamEvent
}

type scriptedStreamModel struct {
	attempts []streamAttempt
	advisor  func(error, int) (time.Duration, bool)
	calls    int
}

func (m *scriptedStreamModel) Generate(_ context.Context, _ *llm.GenerateRequest) (*llm.GenerateResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *scriptedStreamModel) Implements(_ string) bool {
	return true
}

func (m *scriptedStreamModel) Stream(_ context.Context, _ *llm.GenerateRequest) (<-chan llm.StreamEvent, error) {
	if m.calls >= len(m.attempts) {
		return nil, fmt.Errorf("unexpected Stream call #%d", m.calls+1)
	}
	attempt := m.attempts[m.calls]
	m.calls++
	if attempt.startErr != nil {
		return nil, attempt.startErr
	}
	ch := make(chan llm.StreamEvent, len(attempt.events))
	for _, ev := range attempt.events {
		ch <- ev
	}
	close(ch)
	return ch, nil
}

func (m *scriptedStreamModel) AdviseBackoff(err error, attempt int) (time.Duration, bool) {
	if m.advisor == nil {
		return 0, false
	}
	return m.advisor(err, attempt)
}
