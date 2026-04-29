package agently

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/viant/agently-core/sdk"
)

func TestNormalizeCLIContent(t *testing.T) {
	assert.Equal(t, "", normalizeCLIContent(""))
	assert.Equal(t, "helloworld", normalizeCLIContent(" hello   world "))
	assert.Equal(t, "Hi!emojitest😊", normalizeCLIContent("Hi!   emoji\t test   😊"))
}

func TestPickModel(t *testing.T) {
	assert.Equal(t, "", pickModel("", []string{"xai_grok-4-latest"}))
	assert.Equal(t, "openai_gpt-5.4", pickModel("openai_gpt-5.4", []string{"xai_grok-4-latest", "openai_gpt-5.4"}))
	assert.Equal(t, "openai_gpt-5.4", pickModel("openai_gpt-5.4", []string{"xai_grok-4-latest"}))
}

func TestChatStreamerFlush_SkipsWhitespaceOnlyDuplicate(t *testing.T) {
	streamer := &chatStreamer{}
	streamer.content.WriteString("Hi! I'm the coding agent here—ready to help with any code-related tasks.")
	streamer.printed = true

	output := captureStdout(t, func() {
		printed := streamer.Flush("Hi! I'm the coding agent here—ready to help with any code-related tasks. ")
		require.True(t, printed)
	})

	assert.Equal(t, "\n", output)
}

func TestChatStreamerFlush_SkipsNormalizedWhitespaceDuplicate(t *testing.T) {
	streamer := &chatStreamer{}
	streamer.content.WriteString("If this is just casual chat, I can hand it off to the chatter agent.😊")
	streamer.printed = true

	output := captureStdout(t, func() {
		printed := streamer.Flush("If this is just casual chat, I can hand it off to the chatter agent. 😊")
		require.True(t, printed)
	})

	assert.Equal(t, "\n", output)
}

func TestChatStreamerFlush_SkipsNormalizedContainedDuplicate(t *testing.T) {
	streamer := &chatStreamer{}
	streamer.content.WriteString("```json{\"HOME\":\"/Users/awitas\",\"PATH\":\"/usr/bin\"}```")
	streamer.printed = true

	output := captureStdout(t, func() {
		printed := streamer.Flush("```json\n{\"HOME\":\"/Users/awitas\",\"PATH\":\"/usr/bin\"}\n```")
		require.True(t, printed)
	})

	assert.Equal(t, "\n", output)
}

func TestChatStreamerFlush_PrintsCorrectedFinalForCompactForgeFence(t *testing.T) {
	streamer := &chatStreamer{}
	streamer.content.WriteString("```forge-data{\"id\":\"recommended_sites\"}```")
	streamer.printed = true

	output := captureStdout(t, func() {
		printed := streamer.Flush("```forge-data\n{\"id\":\"recommended_sites\"}\n```")
		require.True(t, printed)
	})

	assert.Equal(t, "\n```forge-data\n{\"id\":\"recommended_sites\"}\n```\n", output)
}

func TestNormalizeCLIStreamDelta_RewritesCompactFenceWithinChunk(t *testing.T) {
	got := normalizeCLIStreamDelta("", "```forge-data{\"id\":\"recommended_sites\"}")
	assert.Equal(t, "```forge-data\n{\"id\":\"recommended_sites\"}", got)
}

func TestNormalizeCLIStreamDelta_RewritesCompactFenceAcrossChunks(t *testing.T) {
	got := normalizeCLIStreamDelta("```forge-ui", "{\"version\":1}")
	assert.Equal(t, "\n{\"version\":1}", got)
}

func TestLatestAssistantContentFromState(t *testing.T) {
	content, ok, err := latestAssistantContentFromState(&sdk.ConversationState{
		ConversationID: "conv-1",
		Turns: []*sdk.TurnState{{
			TurnID: "turn-1",
			Status: sdk.TurnStatusCompleted,
			Assistant: &sdk.AssistantState{
				Final: &sdk.AssistantMessageState{
					MessageID: "msg-1",
					Content:   "Final answer",
				},
			},
		}},
	})
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "Final answer", content)
}

func TestLatestAssistantContentFromState_FailedTurn(t *testing.T) {
	_, ok, err := latestAssistantContentFromState(&sdk.ConversationState{
		ConversationID: "conv-1",
		Turns: []*sdk.TurnState{{
			TurnID: "turn-1",
			Status: sdk.TurnStatusFailed,
		}},
	})
	require.Error(t, err)
	assert.False(t, ok)
}

func TestIsShutdownElicitationError(t *testing.T) {
	if !isShutdownElicitationError(context.Canceled) {
		t.Fatalf("expected context.Canceled to be treated as benign shutdown error")
	}
	if !isShutdownElicitationError(context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded to be treated as benign shutdown error")
	}
	if isShutdownElicitationError(errors.New("boom")) {
		t.Fatalf("unexpected benign classification for non-shutdown error")
	}
}

func TestResolveConversationExitCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/tools/system/platform/exitCode/execute", r.URL.Path)
		_, _ = w.Write([]byte(`{"result":"{\"conversationId\":\"conv-1\",\"code\":23}"}`))
	}))
	defer server.Close()

	client, err := sdk.NewHTTP(server.URL)
	require.NoError(t, err)

	code, err := resolveConversationExitCode(context.Background(), client, "conv-1")
	require.NoError(t, err)
	assert.Equal(t, 23, code)
}

func TestResetDebugArtifacts(t *testing.T) {
	dir := t.TempDir()
	debugLog := filepath.Join(dir, "agently-debug.log")
	traceLog := filepath.Join(dir, "trace.ndjson")
	payloadDir := filepath.Join(dir, "payloads")

	originalDebug := defaultDebugLogPath
	t.Setenv(envTraceFilePath, traceLog)
	t.Setenv(envPayloadDirPath, payloadDir)

	require.NoError(t, os.WriteFile(debugLog, []byte("stale-debug"), 0o644))
	require.NoError(t, os.WriteFile(traceLog, []byte("stale-trace"), 0o644))
	require.NoError(t, os.MkdirAll(payloadDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(payloadDir, "payload.json"), []byte(`{"old":true}`), 0o644))

	defaultDebugLogPath = debugLog
	defer func() { defaultDebugLogPath = originalDebug }()

	require.NoError(t, resetDebugArtifacts())

	debugContent, err := os.ReadFile(debugLog)
	require.NoError(t, err)
	assert.Empty(t, debugContent)

	traceContent, err := os.ReadFile(traceLog)
	require.NoError(t, err)
	assert.Empty(t, traceContent)

	entries, err := os.ReadDir(payloadDir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	original := os.Stdout
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = writer
	defer func() {
		os.Stdout = original
	}()

	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, reader)
		done <- buf.String()
	}()

	fn()
	_ = writer.Close()
	return <-done
}
