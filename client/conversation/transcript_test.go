package conversation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	agconv "github.com/viant/agently/pkg/agently/conversation"
)

func TestTranscript_LastAssistantMessageWithModelCall_SkipsSummaryMode(t *testing.T) {
	now := time.Now().UTC()

	respMain := "resp-main"
	respSummary := "resp-summary"
	payloadMain := "payload-main"
	payloadSummary := "payload-summary"
	modePlan := "plan"
	modeSummary := "summary"

	main := &Message{
		Id:        "msg-main",
		Role:      "assistant",
		Type:      "text",
		CreatedAt: now.Add(-1 * time.Second),
		Mode:      &modePlan,
		ModelCall: &agconv.ModelCallView{ProviderResponsePayloadId: &payloadMain, TraceId: &respMain},
	}

	summary := &Message{
		Id:        "msg-summary",
		Role:      "assistant",
		Type:      "text",
		CreatedAt: now,
		Mode:      &modeSummary,
		ModelCall: &agconv.ModelCallView{ProviderResponsePayloadId: &payloadSummary, TraceId: &respSummary},
	}

	turn := &Turn{Id: "turn-1", Message: []*agconv.MessageView{(*agconv.MessageView)(main), (*agconv.MessageView)(summary)}}
	tr := Transcript{turn}

	got := (&tr).LastAssistantMessageWithModelCall()
	require.NotNil(t, got)
	require.NotNil(t, got.ModelCall)
	require.NotNil(t, got.ModelCall.TraceId)
	require.Equal(t, respMain, *got.ModelCall.TraceId)
}

func TestTranscript_LastAssistantMessageWithModelCall_SkipsSummaryStatus(t *testing.T) {
	now := time.Now().UTC()

	respMain := "resp-main"
	respSummary := "resp-summary"
	payloadMain := "payload-main"
	payloadSummary := "payload-summary"
	statusSummary := "summary"

	main := &Message{
		Id:        "msg-main",
		Role:      "assistant",
		Type:      "text",
		CreatedAt: now.Add(-1 * time.Second),
		ModelCall: &agconv.ModelCallView{ProviderResponsePayloadId: &payloadMain, TraceId: &respMain},
	}

	summary := &Message{
		Id:        "msg-summary",
		Role:      "assistant",
		Type:      "text",
		CreatedAt: now,
		Status:    &statusSummary,
		ModelCall: &agconv.ModelCallView{ProviderResponsePayloadId: &payloadSummary, TraceId: &respSummary},
	}

	turn := &Turn{Id: "turn-1", Message: []*agconv.MessageView{(*agconv.MessageView)(main), (*agconv.MessageView)(summary)}}
	tr := Transcript{turn}

	got := (&tr).LastAssistantMessageWithModelCall()
	require.NotNil(t, got)
	require.NotNil(t, got.ModelCall)
	require.NotNil(t, got.ModelCall.TraceId)
	require.Equal(t, respMain, *got.ModelCall.TraceId)
}
