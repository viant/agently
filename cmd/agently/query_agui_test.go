package agently

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/viant/agently-core/protocol/agui"
	"github.com/viant/agently-core/sdk"
	agentsvc "github.com/viant/agently-core/service/agent"
)

func TestCLIQueryUsesRunStreamAndExactInterruptResume(t *testing.T) {
	var runs []agui.RunAgentInput
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/conversations/native":
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "native", "aguiThreadId": " wire "})
		case "/v1/ag-ui/run":
			var input agui.RunAgentInput
			require.NoError(t, json.NewDecoder(r.Body).Decode(&input))
			runs = append(runs, input)
			require.Equal(t, " wire ", input.ThreadID)
			w.Header().Set("Content-Type", "text/event-stream")
			emit := func(event map[string]any) { raw, _ := json.Marshal(event); fmt.Fprintf(w, "data: %s\n\n", raw) }
			emit(map[string]any{"type": "RUN_STARTED", "threadId": input.ThreadID, "runId": input.RunID, "metadata": map[string]any{"agently": map[string]any{"identityVersion": "1", "nativeTurnId": "turn"}}})
			if len(runs) == 1 {
				require.Len(t, input.Messages, 1)
				emit(map[string]any{"type": "RUN_FINISHED", "threadId": input.ThreadID, "runId": input.RunID, "outcome": map[string]any{"type": "interrupt", "interrupts": []any{map[string]any{"id": "ask", "reason": "elicitation", "message": "Choose a color", "responseSchema": map[string]any{"type": "object", "properties": map[string]any{"color": map[string]string{"type": "string"}}, "required": []string{"color"}}}}}})
			} else {
				require.Len(t, runs, 2)
				require.Empty(t, input.Messages)
				require.NotEqual(t, runs[0].RunID, input.RunID)
				var answers []agui.WireResumeEntry
				require.NoError(t, json.Unmarshal(input.Resume, &answers))
				require.Len(t, answers, 1)
				require.Equal(t, "ask", answers[0].InterruptId)
				require.Equal(t, "resolved", answers[0].Status)
				require.JSONEq(t, `{"color":"blue"}`, string(*answers[0].Payload))
				require.NotContains(t, string(input.ForwardedProps), "agentId")
				emit(map[string]any{"type": "TEXT_MESSAGE_START", "messageId": "answer", "role": "assistant"})
				emit(map[string]any{"type": "TEXT_MESSAGE_CONTENT", "messageId": "answer", "delta": "done"})
				emit(map[string]any{"type": "RUN_FINISHED", "threadId": input.ThreadID, "runId": input.RunID, "outcome": map[string]string{"type": "success"}})
			}
		default:
			t.Fatalf("unexpected non-AG-UI conversation request %s", r.URL.Path)
		}
	}))
	defer server.Close()
	client, err := sdk.NewHTTP(server.URL)
	require.NoError(t, err)
	seed := map[string]interface{}{}
	result, printed, err := (&ChatCmd{}).executeQuery(context.Background(), client, &agentsvc.QueryInput{ConversationID: "native", AgentID: "agent", Query: "task"}, map[string]interface{}{"color": "blue"}, &seed)
	require.NoError(t, err)
	require.True(t, printed)
	require.Equal(t, "done", result.Content)
	require.Equal(t, "native", result.ConversationID)
	require.Equal(t, "turn", result.TurnID)
	require.Len(t, runs, 2)
	require.Equal(t, "blue", seed["color"])
}
func TestCLIInterruptedObservationDoesNotSendAnotherRun(t *testing.T) {
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/conversations/native" {
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "native"})
			return
		}
		require.Equal(t, "/v1/ag-ui/run", r.URL.Path)
		posts++
		var input agui.RunAgentInput
		require.NoError(t, json.NewDecoder(r.Body).Decode(&input))
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"type\":\"RUN_STARTED\",\"threadId\":%q,\"runId\":%q}\n\n", input.ThreadID, input.RunID)
	}))
	defer server.Close()
	client, err := sdk.NewHTTP(server.URL)
	require.NoError(t, err)
	_, _, err = (&ChatCmd{}).executeQuery(context.Background(), client, &agentsvc.QueryInput{ConversationID: "native", Query: "task"}, nil, nil)
	var interrupted *sdk.AGUIObservationError
	require.ErrorAs(t, err, &interrupted)
	require.NotEmpty(t, interrupted.RunID)
	require.Equal(t, 1, posts)
}
