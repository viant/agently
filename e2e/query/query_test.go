package query

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	agconv "github.com/viant/agently-core/pkg/agently/conversation"
	coresdk "github.com/viant/agently-core/sdk"
	"github.com/viant/agently/e2e/internal/harness"
	"github.com/viant/scy"
	"github.com/viant/scy/auth/jwt/signer"
	"gopkg.in/yaml.v3"
)

func skipIfNoAPIKey(t *testing.T) {
	t.Helper()
	if strings.TrimSpace(getenv("OPENAI_API_KEY")) == "" {
		t.Skip("OPENAI_API_KEY not set; skipping v1 terminal e2e")
	}
}

func TestTerminalQuerySimple(t *testing.T) {
	skipIfNoAPIKey(t)
	baseURL, workspace := setupGroup(t)
	out := harness.RunChat(t, workspace, baseURL, []string{"--agent-id", "simple", "--query", "Hi, how are you?", "--user", "e2e-test"}, "")
	assert.Contains(t, out, "[agent] simple")
	assert.Contains(t, out, "[conversation-id]")
	assert.NotContains(t, strings.ToLower(out), "no response")
}

func TestTerminalQueryKnowledgeLocal(t *testing.T) {
	skipIfNoAPIKey(t)
	baseURL, workspace := setupGroup(t)
	out := harness.RunChat(t, workspace, baseURL, []string{"--agent-id", "knowledge_local", "--query", "What products does Viant make?", "--user", "e2e-test"}, "")
	lower := strings.ToLower(out)
	assert.Contains(t, out, "[conversation-id]")
	assert.True(t, strings.Contains(lower, "datly") || strings.Contains(lower, "endly") || strings.Contains(lower, "agently"), out)
}

func TestTerminalQueryKnowledgeSystem(t *testing.T) {
	skipIfNoAPIKey(t)
	baseURL, workspace := setupGroup(t)
	out := harness.RunChat(t, workspace, baseURL, []string{"--agent-id", "knowledge_system", "--query", "What are the Go error handling best practices?", "--user", "e2e-test"}, "")
	lower := strings.ToLower(out)
	assert.Contains(t, out, "[conversation-id]")
	assert.True(t,
		strings.Contains(lower, "wrap") ||
			strings.Contains(lower, "sentinel") ||
			strings.Contains(lower, "errors.is") ||
			strings.Contains(lower, "fmt.errorf"),
		out,
	)
}

func TestTerminalQueryToolUsage(t *testing.T) {
	skipIfNoAPIKey(t)
	baseURL, workspace := setupGroup(t)
	out := harness.RunChat(t, workspace, baseURL, []string{
		"--agent-id", "tool_user",
		"--query", "What is the value of the OPENAI_API_KEY environment variable? Just tell me if it exists or not and the first 10 characters.",
		"--user", "e2e-test",
	}, "")
	lower := strings.ToLower(out)
	assert.Contains(t, out, "[conversation-id]")
	assert.True(t, strings.Contains(lower, "openai") || strings.Contains(lower, "environment") || strings.Contains(lower, "sk-"), out)
}

func TestTerminalQueryMultiTurn(t *testing.T) {
	skipIfNoAPIKey(t)
	baseURL, workspace := setupGroup(t)
	stdin := "My name is Alice. Please remember that.\nWhat is my name?\nquit\n"
	out := harness.RunChat(t, workspace, baseURL, []string{"--agent-id", "simple", "--user", "e2e-test"}, stdin)
	assert.Contains(t, out, "[conversation-id]")
	assert.Contains(t, strings.ToLower(out), "alice")
}

func TestTerminalQueryElicitationDefault(t *testing.T) {
	skipIfNoAPIKey(t)
	baseURL, workspace := setupGroup(t)
	out := harness.RunChat(t, workspace, baseURL, []string{
		"--agent-id", "elicitation_favorite_color",
		"--query", "describe my favourite color in 3 sentences",
		"--user", "e2e-test",
		"--elicitation-default", `{"favoriteColor":"blue"}`,
	}, "")
	lower := strings.ToLower(out)
	assert.Contains(t, out, "[conversation-id]")
	assert.Contains(t, lower, "blue")
}

func TestTerminalQueryStreamingOutput(t *testing.T) {
	skipIfNoAPIKey(t)
	baseURL, workspace := setupGroup(t)
	out := harness.RunChat(t, workspace, baseURL, []string{
		"--agent-id", "knowledge_system",
		"--query", "Explain Go error wrapping in two sentences.",
		"--user", "e2e-stream",
	}, "")
	assert.Contains(t, out, "[conversation-id]")
	lower := strings.ToLower(out)
	assert.True(t,
		strings.Contains(lower, "wrap") ||
			strings.Contains(lower, "error") ||
			strings.Contains(lower, "errors.is") ||
			strings.Contains(lower, "fmt.errorf"),
		out,
	)

	conversationMarker := strings.LastIndex(out, "[conversation-id]")
	require.Greater(t, conversationMarker, 0, out)
	prefix := strings.TrimSpace(out[:conversationMarker])
	require.NotEmpty(t, prefix, out)
	assert.NotContains(t, prefix, "[conversation-id]")

	events := readTraceEvents(t)
	assertTraceHasEvent(t, events, "core", "stream_request")
	assertTraceHasTimelineType(t, events, "executor", "text_delta")
}

func TestTerminalQueryJWTUnauthorized(t *testing.T) {
	skipIfNoAPIKey(t)
	baseURL, workspace, _ := setupJWTAuthGroup(t)
	out, err := harness.RunChatResult(t, workspace, baseURL, []string{
		"--agent-id", "simple",
		"--query", "Hi, how are you?",
		"--user", "jwt-user",
	}, "")
	require.Error(t, err)
	lower := strings.ToLower(out + "\n" + err.Error())
	assert.Contains(t, lower, "authorization required")
}

func TestTerminalQueryJWTAuthorized(t *testing.T) {
	skipIfNoAPIKey(t)
	baseURL, workspace, token := setupJWTAuthGroup(t)
	out := harness.RunChat(t, workspace, baseURL, []string{
		"--agent-id", "simple",
		"--token", token,
		"--query", "Hi, how are you?",
		"--user", "jwt-user",
	}, "")
	assert.Contains(t, out, "[conversation-id]")
	assert.NotContains(t, strings.ToLower(out), "authorization required")

	client, err := coresdk.NewHTTP(baseURL, coresdk.WithAuthToken(token))
	require.NoError(t, err)
	conversationID := extractConversationID(t, out)
	transcript, err := client.GetTranscript(harness.Context(t), &coresdk.GetTranscriptInput{ConversationID: conversationID})
	require.NoError(t, err)
	require.NotNil(t, transcript.Conversation)
	require.NotEmpty(t, transcript.Conversation.Turns)
	require.NotNil(t, transcript.Conversation.Turns[0].User)
	assert.NotEmpty(t, transcript.Conversation.Turns[0].User.Content)
}

func TestTerminalQueryImageAttachment(t *testing.T) {
	skipIfNoAPIKey(t)
	baseURL, workspace := setupGroup(t)
	imagePath := writeTempAttachment(t, "red-dot.png", mustCreatePNG(t, color.RGBA{R: 255, A: 255}))
	out := harness.RunChat(t, workspace, baseURL, []string{
		"--agent-id", "simple",
		"--attach", imagePath,
		"--query", "What is the dominant color in the attached image? Answer with one word.",
		"--user", "e2e-image",
	}, "")
	lower := strings.ToLower(out)
	assert.Contains(t, out, "[conversation-id]")
	assert.Contains(t, lower, "red")

	conversationID := extractConversationID(t, out)
	client, err := coresdk.NewHTTP(baseURL)
	require.NoError(t, err)
	transcript, err := client.GetTranscript(harness.Context(t), &coresdk.GetTranscriptInput{ConversationID: conversationID})
	require.NoError(t, err)
	require.NotNil(t, transcript.Conversation)
	require.NotEmpty(t, transcript.Conversation.Turns)
	require.NotNil(t, transcript.Conversation.Turns[0].User)

	msgs, err := client.GetMessages(harness.Context(t), &coresdk.GetMessagesInput{
		ConversationID: conversationID,
		Roles:          []string{"user"},
		Types:          []string{"control"},
	})
	require.NoError(t, err)
	require.NotEmpty(t, msgs.Rows)
	assert.Equal(t, "red-dot.png", valueOrEmpty(msgs.Rows[0].Content))
}

func TestTerminalQueryPDFAttachment(t *testing.T) {
	skipIfNoAPIKey(t)
	baseURL, workspace := setupGroup(t)
	pdfPath := writeTempAttachment(t, "token.pdf", mustCreatePDF(t))
	out := harness.RunChat(t, workspace, baseURL, []string{
		"--agent-id", "pdf_inline",
		"--attach", pdfPath,
		"--query", "What exact token appears in the attached PDF? Answer only with the token.",
		"--user", "e2e-pdf",
	}, "")
	assert.Contains(t, out, "[conversation-id]")
	assert.Contains(t, out, "PDF_TEST_TOKEN_4729")

	conversationID := extractConversationID(t, out)
	client, err := coresdk.NewHTTP(baseURL)
	require.NoError(t, err)
	transcript, err := client.GetTranscript(harness.Context(t), &coresdk.GetTranscriptInput{ConversationID: conversationID})
	require.NoError(t, err)
	require.NotNil(t, transcript.Conversation)
	require.NotEmpty(t, transcript.Conversation.Turns)

	msgs, err := client.GetMessages(harness.Context(t), &coresdk.GetMessagesInput{
		ConversationID: conversationID,
		Roles:          []string{"user"},
		Types:          []string{"control"},
	})
	require.NoError(t, err)
	require.NotEmpty(t, msgs.Rows)
	assert.Equal(t, "token.pdf", valueOrEmpty(msgs.Rows[0].Content))
}

func TestTerminalQueryPDFRefAttachment(t *testing.T) {
	skipIfNoAPIKey(t)
	baseURL, workspace := setupGroup(t)
	pdfPath := writeTempAttachment(t, "token.pdf", mustCreatePDF(t))
	out := harness.RunChat(t, workspace, baseURL, []string{
		"--agent-id", "pdf_ref",
		"--attach", pdfPath,
		"--query", "What exact token appears in the attached PDF? Answer only with the token.",
		"--user", "e2e-pdf-ref",
	}, "")
	assert.Contains(t, out, "[conversation-id]")
	assert.Contains(t, out, "PDF_TEST_TOKEN_4729")

	conversationID := extractConversationID(t, out)
	client, err := coresdk.NewHTTP(baseURL)
	require.NoError(t, err)
	transcript, err := client.GetTranscript(harness.Context(t), &coresdk.GetTranscriptInput{ConversationID: conversationID})
	require.NoError(t, err)
	require.NotNil(t, transcript.Conversation)
	require.NotEmpty(t, transcript.Conversation.Turns)
}

func TestTerminalQueryMultiAttachment(t *testing.T) {
	skipIfNoAPIKey(t)
	baseURL, workspace := setupGroup(t)
	imagePath := writeTempAttachment(t, "red-dot.png", mustCreatePNG(t, color.RGBA{R: 255, A: 255}))
	pdfPath := writeTempAttachment(t, "token.pdf", mustCreatePDF(t))
	out := harness.RunChat(t, workspace, baseURL, []string{
		"--agent-id", "attachment_combo",
		"--attach", imagePath,
		"--attach", pdfPath,
		"--query", "Report the PDF token and the image color in the required two-line format.",
		"--user", "e2e-multi-attach",
		"--timeout", "120",
	}, "")
	lower := strings.ToLower(out)
	assert.Contains(t, out, "[conversation-id]")
	assert.Contains(t, lower, "pdf_test_token_4729")
	assert.Contains(t, lower, "red")
}

func TestTerminalQueryGeneratedImageOutput(t *testing.T) {
	skipIfNoAPIKey(t)
	baseURL, workspace := setupGroup(t)
	out := harness.RunChat(t, workspace, baseURL, []string{
		"--agent-id", "image_generator",
		"--query", "Generate a tiny red square PNG image and reply with only the filename.",
		"--user", "e2e-file",
		"--timeout", "120",
	}, "")
	assert.Contains(t, out, "[conversation-id]")

	conversationID := extractConversationID(t, out)
	client, err := coresdk.NewHTTP(baseURL)
	require.NoError(t, err)
	files, err := client.ListFiles(harness.Context(t), &coresdk.ListFilesInput{ConversationID: conversationID})
	require.NoError(t, err)
	require.NotEmpty(t, files.Files)
	assert.Equal(t, "image/png", files.Files[0].ContentType)

	fileData, err := client.DownloadFile(harness.Context(t), &coresdk.DownloadFileInput{
		ConversationID: conversationID,
		FileID:         files.Files[0].ID,
	})
	require.NoError(t, err)
	require.NotNil(t, fileData)
	assert.True(t,
		strings.Contains(strings.ToLower(fileData.ContentType), "image/png") ||
			bytes.HasPrefix(fileData.Data, []byte{0x89, 0x50, 0x4e, 0x47}),
		"expected generated image payload; contentType=%q len=%d", fileData.ContentType, len(fileData.Data),
	)
}

func TestTerminalQueryLinkedConversationCriticReview(t *testing.T) {
	skipIfNoAPIKey(t)
	baseURL, workspace := setupGroup(t)
	out := harness.RunChat(t, workspace, baseURL, []string{
		"--agent-id", "linked_story_chatter",
		"--query", "Write a story about a dog.",
		"--user", "e2e-linked",
		"--timeout", "120",
	}, "")
	assert.Contains(t, out, "[conversation-id]")
	assert.Contains(t, out, "A dog named Comet found a blue ball in the park and carried it home proudly.")

	conversationID := extractConversationID(t, out)
	client, err := coresdk.NewHTTP(baseURL)
	require.NoError(t, err)
	transcript, err := client.GetTranscript(harness.Context(t), &coresdk.GetTranscriptInput{ConversationID: conversationID})
	require.NoError(t, err)

	linkedConversationID := firstLinkedConversationIDFromCoreTranscript(transcript)
	require.NotEmpty(t, linkedConversationID, "expected linked child conversation in transcript")

	linkedPage, err := client.ListLinkedConversations(harness.Context(t), &coresdk.ListLinkedConversationsInput{
		ParentConversationID: conversationID,
	})
	require.NoError(t, err)
	require.NotEmpty(t, linkedPage.Rows)
	assert.Equal(t, linkedConversationID, linkedPage.Rows[0].ConversationID)
	assert.NotEmpty(t, linkedPage.Rows[0].Status)
	assert.Contains(t, linkedPage.Rows[0].Response, "A dog named Comet found a blue ball in the park and carried it home proudly.")

	childTranscript, err := client.GetTranscript(harness.Context(t), &coresdk.GetTranscriptInput{ConversationID: linkedConversationID})
	require.NoError(t, err)
	childText := collectCoreTranscriptText(childTranscript)
	assert.Contains(t, childText, "A dog named Comet found a blue ball in the park and carried it home proudly.")
}

func TestTerminalQueryAttachMissingFile(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), ".agently")
	out, err := harness.RunChatResult(t, workspace, "http://127.0.0.1:1", []string{
		"--agent-id", "simple",
		"--attach", filepath.Join(t.TempDir(), "missing.png"),
		"--query", "Hi",
		"--user", "e2e-invalid",
	}, "")
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "read attachment")
	assert.Contains(t, strings.ToLower(out), "read attachment")
}

func TestTerminalQueryAttachEmptyValue(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), ".agently")
	out, err := harness.RunChatResult(t, workspace, "http://127.0.0.1:1", []string{
		"--agent-id", "simple",
		"--attach", "",
		"--query", "Hi",
		"--user", "e2e-invalid",
	}, "")
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "attachment path is required")
	assert.Contains(t, strings.ToLower(out), "attachment path is required")
}

func TestTerminalQueryAttachUnsupportedType(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), ".agently")
	path := writeTempAttachment(t, "blob.unknownext", []byte("x"))
	out, err := harness.RunChatResult(t, workspace, "http://127.0.0.1:1", []string{
		"--agent-id", "simple",
		"--attach", path,
		"--query", "Hi",
		"--user", "e2e-invalid",
	}, "")
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "unsupported attachment type")
	assert.Contains(t, strings.ToLower(out), "unsupported attachment type")
}

func TestTerminalQueryInvalidContext(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), ".agently")
	contextPath := writeTempAttachment(t, "context.json", []byte("{invalid"))
	out, err := harness.RunChatResult(t, workspace, "http://127.0.0.1:1", []string{
		"--agent-id", "simple",
		"--context", "@" + contextPath,
		"--query", "Hi",
		"--user", "e2e-invalid",
	}, "")
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "parse context json")
	assert.Contains(t, strings.ToLower(out), "parse context json")
}

func TestTerminalQueryInvalidElicitationDefault(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), ".agently")
	payloadPath := writeTempAttachment(t, "elicitation.json", []byte("{invalid"))
	out, err := harness.RunChatResult(t, workspace, "http://127.0.0.1:1", []string{
		"--agent-id", "simple",
		"--elicitation-default", "@" + payloadPath,
		"--query", "Hi",
		"--user", "e2e-invalid",
	}, "")
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "parse --elicitation-default")
	assert.Contains(t, strings.ToLower(out), "parse --elicitation-default")
}

func TestTerminalQueryJWTInvalidToken(t *testing.T) {
	skipIfNoAPIKey(t)
	baseURL, workspace, _ := setupJWTAuthGroup(t)
	out, err := harness.RunChatResult(t, workspace, baseURL, []string{
		"--agent-id", "simple",
		"--token", "not-a-valid-jwt-token",
		"--query", "Hi, how are you?",
		"--user", "jwt-user",
	}, "")
	require.Error(t, err)
	lower := strings.ToLower(out + "\n" + err.Error())
	assert.Contains(t, lower, "authorization required")
}

func TestTerminalQueryCoderRepoAnalysis(t *testing.T) {
	skipIfNoAPIKey(t)
	baseURL, workspace := setupGroup(t)
	repoPath := harness.EnsureGitClone(t, "https://github.com/viant/xdatly.git", "xdatly")
	snapshot := buildRepoSnapshot(t, repoPath)
	contextJSON, err := json.Marshal(map[string]interface{}{
		"workdir":      repoPath,
		"repoSnapshot": snapshot,
	})
	require.NoError(t, err)
	out := harness.RunChat(t, workspace, baseURL, []string{
		"--agent-id", "coder",
		"--query", "Analyze the repository from context and fill the required four-line summary.",
		"--context", string(contextJSON),
		"--timeout", "90",
		"--user", "e2e-test",
	}, "")
	lower := strings.ToLower(out)
	assert.Contains(t, out, "[conversation-id]")
	assert.Contains(t, lower, "repository: xdatly")
	assert.Contains(t, lower, "primary language: go")
	assert.Contains(t, lower, "purpose:")
	assert.True(t,
		strings.Contains(lower, "notable directory: ./handler") ||
			strings.Contains(lower, "notable directory: handler") ||
			strings.Contains(lower, "notable directory: ./service") ||
			strings.Contains(lower, "notable directory: service") ||
			strings.Contains(lower, "notable directory: ./types") ||
			strings.Contains(lower, "notable directory: types") ||
			strings.Contains(lower, "notable directory: ./cmd") ||
			strings.Contains(lower, "notable directory: cmd") ||
			strings.Contains(lower, "notable directory: ./codec") ||
			strings.Contains(lower, "notable directory: codec") ||
			strings.Contains(lower, "notable directory: ./predicate") ||
			strings.Contains(lower, "notable directory: predicate") ||
			strings.Contains(lower, "notable directory: ./extension") ||
			strings.Contains(lower, "notable directory: extension"),
		out,
	)
}

func TestTerminalQueryCoderRepoAnalysisLiveTranscript(t *testing.T) {
	skipIfNoAPIKey(t)
	t.Setenv("AGENTLY_DEBUG", "1")
	t.Setenv("AGENTLY_SCHEDULER_DEBUG", "1")
	baseURL, workspace := setupGroup(t)
	repoPath := harness.EnsureGitClone(t, "https://github.com/viant/xdatly.git", "xdatly")
	contextJSON, err := json.Marshal(map[string]string{"workdir": repoPath})
	require.NoError(t, err)
	out := harness.RunChat(t, workspace, baseURL, []string{
		"--agent-id", "coder_live",
		"--query", "Inspect the repository live and return the required four-line analysis.",
		"--context", string(contextJSON),
		"--timeout", "90",
		"--user", "e2e-test",
	}, "")
	conversationID := extractConversationID(t, out)
	client, err := coresdk.NewHTTP(baseURL)
	require.NoError(t, err)
	transcript, err := client.GetTranscript(harness.Context(t), &coresdk.GetTranscriptInput{
		ConversationID:    conversationID,
		IncludeToolCalls:  true,
		IncludeModelCalls: true,
	})
	require.NoError(t, err)
	writeTranscriptDebug(t, transcript)
	toolCalls, modelCalls, iterations := transcriptStats(transcript)
	assertLiveRepoTranscriptSane(t, toolCalls, modelCalls, iterations, out)
	assert.Greater(t, len(modelCalls), 1, out)
	assert.Greater(t, len(iterations), 0, out)
	lower := strings.ToLower(out)
	assert.Contains(t, lower, "repository: xdatly")
	assert.Contains(t, lower, "primary language: go")
	assert.Contains(t, lower, "notable directories:")
}

func assertLiveRepoTranscriptSane(t *testing.T, toolCalls []*agconv.ToolCallView, modelCalls []*agconv.ModelCallView, iterations map[int]struct{}, output string) {
	t.Helper()
	assert.Greater(t, len(toolCalls), 0, output)
	assert.LessOrEqual(t, len(toolCalls), 2, output)
	assert.LessOrEqual(t, len(modelCalls), 3, output)
	assert.LessOrEqual(t, len(iterations), 3, output)

	completedTools := 0
	for _, call := range toolCalls {
		if call == nil {
			continue
		}
		status := strings.ToLower(strings.TrimSpace(call.Status))
		if status == "completed" || status == "ok" || status == "succeeded" {
			completedTools++
		}
	}
	assert.GreaterOrEqual(t, completedTools, 1, output)
}

func buildRepoSnapshot(t *testing.T, repoPath string) map[string]interface{} {
	t.Helper()
	snapshot := map[string]interface{}{
		"name":         filepath.Base(repoPath),
		"module":       readGoModule(t, repoPath),
		"topLevelDirs": topLevelDirs(t, repoPath),
		"packages":     repoPackages(t, repoPath),
	}
	return snapshot
}

func readGoModule(t *testing.T, repoPath string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoPath, "go.mod"))
	require.NoError(t, err)
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	require.NoError(t, scanner.Err())
	t.Fatalf("module declaration not found in %s", filepath.Join(repoPath, "go.mod"))
	return ""
}

func topLevelDirs(t *testing.T, repoPath string) []string {
	t.Helper()
	entries, err := os.ReadDir(repoPath)
	require.NoError(t, err)
	var result []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		result = append(result, name)
	}
	slices.Sort(result)
	return result
}

func repoPackages(t *testing.T, repoPath string) []string {
	t.Helper()
	var result []string
	err := filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
		require.NoError(t, err)
		if len(result) >= 8 {
			return filepath.SkipAll
		}
		if info.IsDir() {
			if strings.HasPrefix(info.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		file, err := os.Open(path)
		require.NoError(t, err)
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, "package ") {
				rel, relErr := filepath.Rel(repoPath, path)
				require.NoError(t, relErr)
				result = append(result, rel+":"+strings.TrimSpace(strings.TrimPrefix(line, "package ")))
				break
			}
		}
		require.NoError(t, scanner.Err())
		return nil
	})
	require.NoError(t, err)
	return result
}

func extractConversationID(t *testing.T, output string) string {
	t.Helper()
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "[conversation-id]") {
			continue
		}
		id := strings.TrimSpace(strings.TrimPrefix(line, "[conversation-id]"))
		require.NotEmpty(t, id, output)
		return id
	}
	require.NoError(t, scanner.Err())
	t.Fatalf("conversation id not found in output:\n%s", output)
	return ""
}

func transcriptStats(transcript *coresdk.ConversationStateResponse) ([]*agconv.ToolCallView, []*agconv.ModelCallView, map[int]struct{}) {
	var toolCalls []*agconv.ToolCallView
	var modelCalls []*agconv.ModelCallView
	iterations := map[int]struct{}{}
	if transcript == nil || transcript.Conversation == nil {
		return toolCalls, modelCalls, iterations
	}
	for _, turn := range transcript.Conversation.Turns {
		if turn == nil || turn.Execution == nil {
			continue
		}
		for _, page := range turn.Execution.Pages {
			if page == nil {
				continue
			}
			iterations[page.Iteration] = struct{}{}
			for _, ms := range page.ModelSteps {
				if ms == nil {
					continue
				}
				modelCalls = append(modelCalls, &agconv.ModelCallView{})
			}
			for _, ts := range page.ToolSteps {
				if ts == nil {
					continue
				}
				toolCalls = append(toolCalls, &agconv.ToolCallView{Status: ts.Status})
			}
		}
	}
	return toolCalls, modelCalls, iterations
}

func writeTranscriptDebug(t *testing.T, transcript *coresdk.ConversationStateResponse) {
	t.Helper()
	data, err := json.MarshalIndent(transcript, "", "  ")
	require.NoError(t, err)
	path := harness.DebugLogPath(t, "transcript.json")
	require.NoError(t, os.WriteFile(path, data, 0o644))
}

func setupGroup(t *testing.T) (string, string) {
	t.Helper()
	template := filepath.Join(harness.RepoRoot(), "e2e", "query", "testdata", "workspace")
	workspace := harness.CopyWorkspaceTemplate(t, template)
	baseURL := harness.StartServer(t, workspace)
	require.NotEmpty(t, baseURL)
	require.NotEmpty(t, workspace)
	return baseURL, workspace
}

func setupJWTAuthGroup(t *testing.T) (string, string, string) {
	t.Helper()
	template := filepath.Join(harness.RepoRoot(), "e2e", "query", "testdata", "workspace")
	workspace := harness.CopyWorkspaceTemplate(t, template)
	privateKeyPath, publicKeyPath := generateRSAKeyPair(t, t.TempDir())
	writeJWTWorkspaceConfig(t, workspace, publicKeyPath, privateKeyPath)
	baseURL := harness.StartServer(t, workspace)
	token := signTestJWT(t, privateKeyPath, map[string]interface{}{
		"sub":      "jwt-user",
		"username": "jwt-user",
		"email":    "jwt-user@example.com",
	}, time.Hour)
	return baseURL, workspace, token
}

func getenv(name string) string {
	return strings.TrimSpace(os.Getenv(name))
}

func generateRSAKeyPair(t *testing.T, dir string) (string, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	privPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	require.NoError(t, err)
	pubPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubDER,
	})

	privPath := filepath.Join(dir, "private.pem")
	pubPath := filepath.Join(dir, "public.pem")
	require.NoError(t, os.WriteFile(privPath, privPEM, 0o600))
	require.NoError(t, os.WriteFile(pubPath, pubPEM, 0o644))
	return privPath, pubPath
}

func signTestJWT(t *testing.T, privPath string, claims map[string]interface{}, ttl time.Duration) string {
	t.Helper()
	ctx := context.Background()
	cfg := &signer.Config{
		RSA: scy.NewResource("", privPath, ""),
	}
	s := signer.New(cfg)
	require.NoError(t, s.Init(ctx))
	token, err := s.Create(ttl, claims)
	require.NoError(t, err)
	return token
}

func writeJWTWorkspaceConfig(t *testing.T, workspacePath, publicKeyPath, privateKeyPath string) {
	t.Helper()
	configPath := filepath.Join(workspacePath, "config.yaml")
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)

	var cfg map[string]interface{}
	require.NoError(t, yaml.Unmarshal(data, &cfg))
	cfg["auth"] = map[string]interface{}{
		"enabled":         true,
		"cookieName":      "agently_session",
		"defaultUsername": "",
		"ipHashKey":       "v1-e2e-jwt-ip-hash",
		"local": map[string]interface{}{
			"enabled": false,
		},
		"jwt": map[string]interface{}{
			"enabled":       true,
			"rsa":           []string{publicKeyPath},
			"rsaPrivateKey": privateKeyPath,
		},
	}

	encoded, err := yaml.Marshal(cfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(configPath, encoded, 0o644))
}

func writeTempAttachment(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, data, 0o644))
	return path
}

func mustCreatePNG(t *testing.T, fill color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			img.SetRGBA(x, y, fill)
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

func mustCreatePDF(t *testing.T) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(validPDFBase64)
	require.NoError(t, err)
	return data
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func firstLinkedConversationIDFromCoreTranscript(transcript *coresdk.ConversationStateResponse) string {
	if transcript == nil || transcript.Conversation == nil {
		return ""
	}
	for _, turn := range transcript.Conversation.Turns {
		if turn == nil {
			continue
		}
		for _, lc := range turn.LinkedConversations {
			if lc == nil {
				continue
			}
			if value := strings.TrimSpace(lc.ConversationID); value != "" {
				return value
			}
		}
	}
	return ""
}

func collectCoreTranscriptText(transcript *coresdk.ConversationStateResponse) string {
	var parts []string
	if transcript == nil || transcript.Conversation == nil {
		return ""
	}
	for _, turn := range transcript.Conversation.Turns {
		if turn == nil || turn.Assistant == nil {
			continue
		}
		if turn.Assistant.Final != nil {
			if text := strings.TrimSpace(turn.Assistant.Final.Content); text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func readTraceEvents(t *testing.T) []map[string]interface{} {
	t.Helper()
	data, err := os.ReadFile(harness.DebugLogPath(t, "trace.ndjson"))
	require.NoError(t, err)
	var result []map[string]interface{}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var item map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(line), &item))
		result = append(result, item)
	}
	require.NoError(t, scanner.Err())
	return result
}

func assertTraceHasEvent(t *testing.T, events []map[string]interface{}, component, event string) {
	t.Helper()
	for _, item := range events {
		if strings.TrimSpace(toString(item["component"])) == component &&
			strings.TrimSpace(toString(item["event"])) == event {
			return
		}
	}
	t.Fatalf("trace missing %s/%s event", component, event)
}

func assertTraceHasTimelineType(t *testing.T, events []map[string]interface{}, component, timelineType string) {
	t.Helper()
	for _, item := range events {
		if strings.TrimSpace(toString(item["component"])) != component ||
			strings.TrimSpace(toString(item["event"])) != "timeline" {
			continue
		}
		fields, _ := item["fields"].(map[string]interface{})
		if strings.TrimSpace(toString(fields["type"])) == timelineType {
			return
		}
	}
	t.Fatalf("trace missing %s timeline type %s event", component, timelineType)
}

func toString(value interface{}) string {
	switch actual := value.(type) {
	case string:
		return actual
	default:
		return ""
	}
}

const validPDFBase64 = "JVBERi0xLjMKMyAwIG9iago8PC9UeXBlIC9QYWdlCi9QYXJlbnQgMSAwIFIKL1Jlc291cmNlcyAyIDAgUgovQ29udGVudHMgNCAwIFI+PgplbmRvYmoKNCAwIG9iago8PC9MZW5ndGggMTM2Pj4Kc3RyZWFtCjAgSgowIGoKMC41NyB3CjAuMDAwIEcKMC4wMDAgZwpCVCAvRjBhNzY3MDVkMThlMDQ5NGRkMjRjYjU3M2U1M2FhMGE4YzcxMGVjOTkgMTYuMDAgVGYgRVQKQlQgNTYuNjkgNzU2Ljg1IFRkIChQREZfVEVTVF9UT0tFTl80NzI5KSBUaiBFVAoKZW5kc3RyZWFtCmVuZG9iagoxIDAgb2JqCjw8L1R5cGUgL1BhZ2VzCi9LaWRzIFszIDAgUiBdCi9Db3VudCAxCi9NZWRpYUJveCBbMCAwIDU5NS4yOCA4NDEuODldCj4+CmVuZG9iago1IDAgb2JqCjw8L1R5cGUgL0ZvbnQKL0Jhc2VGb250IC9IZWx2ZXRpY2EKL1N1YnR5cGUgL1R5cGUxCi9FbmNvZGluZyAvV2luQW5zaUVuY29kaW5nCj4+CmVuZG9iagoyIDAgb2JqCjw8Ci9Qcm9jU2V0IFsvUERGIC9UZXh0IC9JbWFnZUIgL0ltYWdlQyAvSW1hZ2VJXQovRm9udCA8PAovRjBhNzY3MDVkMThlMDQ5NGRkMjRjYjU3M2U1M2FhMGE4YzcxMGVjOTkgNSAwIFIKPj4KL1hPYmplY3QgPDwKPj4KL0NvbG9yU3BhY2UgPDwKPj4KPj4KZW5kb2JqCjYgMCBvYmoKPDwKL1Byb2R1Y2VyICj+/wBGAFAARABGACAAMQAuADcpCi9DcmVhdGlvbkRhdGUgKEQ6MjAyNjAzMTMxNjMzMjQpCi9Nb2REYXRlIChEOjIwMjYwMzEzMTYzMzI0KQo+PgplbmRvYmoKNyAwIG9iago8PAovVHlwZSAvQ2F0YWxvZwovUGFnZXMgMSAwIFIKL05hbWVzIDw8Ci9FbWJlZGRlZEZpbGVzIDw8IC9OYW1lcyBbCiAgCl0gPj4KPj4KPj4KZW5kb2JqCnhyZWYKMCA4CjAwMDAwMDAwMDAgNjU1MzUgZiAKMDAwMDAwMDI3MiAwMDAwMCBuIAowMDAwMDAwNDU1IDAwMDAwIG4gCjAwMDAwMDAwMDkgMDAwMDAgbiAKMDAwMDAwMDA4NyAwMDAwMCBuIAowMDAwMDAwMzU5IDAwMDAwIG4gCjAwMDAwMDA2MTYgMDAwMDAgbiAKMDAwMDAwMDcyOSAwMDAwMCBuIAp0cmFpbGVyCjw8Ci9TaXplIDgKL1Jvb3QgNyAwIFIKL0luZm8gNiAwIFIKPj4Kc3RhcnR4cmVmCjgyNgolJUVPRgo="
