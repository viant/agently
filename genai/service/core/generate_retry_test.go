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
	"github.com/viant/agently/genai/llm/provider/base"
	"github.com/viant/agently/genai/prompt"
)

func TestService_Generate_RetriesOnTransientErrorAndSucceeds(t *testing.T) {
	model := &generateSequenceModel{
		outcomes: []generateOutcome{
			{err: fmt.Errorf("OpenAI API error (status 500): The server had an error processing your request. (type=server_error)")},
			{response: textGenerateResponse("ok")},
		},
		advisor: func(err error, _ int) (time.Duration, bool) {
			if err != nil && strings.Contains(strings.ToLower(err.Error()), "status 500") {
				return 0, true
			}
			return 0, false
		},
	}

	svc := &Service{llmFinder: &generateFixedFinder{model: model}}
	out := &GenerateOutput{}
	err := svc.Generate(context.Background(), newGenerateInput(), out)
	require.NoError(t, err)
	assert.Equal(t, 2, model.calls)
	require.NotNil(t, out.Response)
	assert.Equal(t, "ok", out.Content)
}

func TestService_Generate_RetriesOnTransient503WithoutAdvisor(t *testing.T) {
	model := &generateSequenceModel{
		outcomes: []generateOutcome{
			{err: fmt.Errorf("provider unavailable (status 503)")},
			{response: textGenerateResponse("ok-503")},
		},
	}

	svc := &Service{llmFinder: &generateFixedFinder{model: model}}
	out := &GenerateOutput{}
	err := svc.Generate(context.Background(), newGenerateInput(), out)
	require.NoError(t, err)
	assert.Equal(t, 2, model.calls)
	require.NotNil(t, out.Response)
	assert.Equal(t, "ok-503", out.Content)
}

func TestService_Generate_RetriesWhenAdvisorRequestsIt(t *testing.T) {
	model := &generateSequenceModel{
		outcomes: []generateOutcome{
			{err: fmt.Errorf("provider throttlingexception")},
			{response: textGenerateResponse("advisor-ok")},
		},
		advisor: func(err error, _ int) (time.Duration, bool) {
			if err != nil && strings.Contains(strings.ToLower(err.Error()), "throttling") {
				return 0, true
			}
			return 0, false
		},
	}

	svc := &Service{llmFinder: &generateFixedFinder{model: model}}
	out := &GenerateOutput{}
	err := svc.Generate(context.Background(), newGenerateInput(), out)
	require.NoError(t, err)
	assert.Equal(t, 2, model.calls)
	assert.Equal(t, "advisor-ok", out.Content)
}

func TestService_Generate_DoesNotRetryOnNonTransientError(t *testing.T) {
	model := &generateSequenceModel{
		outcomes: []generateOutcome{
			{err: fmt.Errorf("OpenAI API error (status 400): invalid request")},
		},
	}

	svc := &Service{llmFinder: &generateFixedFinder{model: model}}
	out := &GenerateOutput{}
	err := svc.Generate(context.Background(), newGenerateInput(), out)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to generate content")
	assert.Equal(t, 1, model.calls)
}

func TestService_Generate_DoesNotRetryOnContextLimitError(t *testing.T) {
	model := &generateSequenceModel{
		outcomes: []generateOutcome{
			{err: fmt.Errorf("maximum context length exceeded")},
		},
	}

	svc := &Service{llmFinder: &generateFixedFinder{model: model}}
	out := &GenerateOutput{}
	err := svc.Generate(context.Background(), newGenerateInput(), out)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrContextLimitExceeded))
	assert.Equal(t, 1, model.calls)
}

func newGenerateInput() *GenerateInput {
	return &GenerateInput{
		ModelSelection: llm.ModelSelection{Model: "mock-model"},
		UserID:         "user-1",
		Prompt:         &prompt.Prompt{Text: "hello"},
		Binding:        &prompt.Binding{},
	}
}

func textGenerateResponse(content string) *llm.GenerateResponse {
	return &llm.GenerateResponse{
		Choices: []llm.Choice{
			{
				Message: llm.Message{
					Role:    llm.RoleAssistant,
					Content: content,
				},
				FinishReason: "stop",
			},
		},
	}
}

type generateFixedFinder struct {
	model llm.Model
	err   error
}

func (f *generateFixedFinder) Find(_ context.Context, _ string) (llm.Model, error) {
	return f.model, f.err
}

type generateOutcome struct {
	response *llm.GenerateResponse
	err      error
}

type generateSequenceModel struct {
	outcomes []generateOutcome
	advisor  func(err error, attempt int) (time.Duration, bool)
	calls    int
}

func (m *generateSequenceModel) Generate(_ context.Context, _ *llm.GenerateRequest) (*llm.GenerateResponse, error) {
	if m.calls >= len(m.outcomes) {
		return nil, fmt.Errorf("unexpected Generate call #%d", m.calls+1)
	}
	outcome := m.outcomes[m.calls]
	m.calls++
	return outcome.response, outcome.err
}

func (m *generateSequenceModel) Implements(feature string) bool {
	// Disable continuation path for focused retry tests.
	if feature == base.SupportsContextContinuation {
		return false
	}
	return true
}

func (m *generateSequenceModel) AdviseBackoff(err error, attempt int) (time.Duration, bool) {
	if m.advisor == nil {
		return 0, false
	}
	return m.advisor(err, attempt)
}
