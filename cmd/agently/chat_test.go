package agently

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
