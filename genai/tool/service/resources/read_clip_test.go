package resources

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	aug "github.com/viant/agently/genai/service/augmenter"
	"github.com/viant/agently/genai/textclip"
)

// This test suite targets the resource read selection paths that delegate to
// textclip helpers: ClipHead, ClipTail, and ClipLines. It drives them via the
// Service.read API using lineRange (startLine/lineCount) and/or MaxBytes.
func TestService_Read_ClipVariants(t *testing.T) {
	ctx := context.Background()
	svc := New(aug.New(nil))

	tempDir := t.TempDir()
	rootURI := "file://" + tempDir

	// File with no trailing newline to validate head/tail behavior precisely.
	headTailName := "head_tail.txt"
	headTailContent := "a\nb\nc\nd" // no trailing newline
	if err := os.WriteFile(filepath.Join(tempDir, headTailName), []byte(headTailContent), 0644); err != nil {
		t.Fatalf("failed to write %s: %v", headTailName, err)
	}

	// File with trailing newline for ClipLines mapping edge at EOF.
	linesName := "lines.txt"
	linesContent := "a\nb\nc\nd\n" // trailing newline
	if err := os.WriteFile(filepath.Join(tempDir, linesName), []byte(linesContent), 0644); err != nil {
		t.Fatalf("failed to write %s: %v", linesName, err)
	}

	t.Run("head with lineCount only (ClipHead)", func(t *testing.T) {
		out := &ReadOutput{}
		in := &ReadInput{RootURI: rootURI, Path: headTailName, LineRange: textclip.LineRange{LineCount: 2}}
		if err := svc.read(ctx, in, out); err != nil {
			t.Fatalf("read error: %v", err)
		}
		assert.Equal(t, "a\nb", out.Content)
		assert.Equal(t, 0, out.StartLine)
		assert.Equal(t, 0, out.EndLine)
		assert.Equal(t, "head", out.ModeApplied)
		if assert.NotNil(t, out.Continuation) {
			assert.True(t, out.Continuation.HasMore)
		}
	})

	t.Run("tail with lineCount only (ClipTail)", func(t *testing.T) {
		out := &ReadOutput{}
		in := &ReadInput{RootURI: rootURI, Path: headTailName, Mode: "tail", LineRange: textclip.LineRange{LineCount: 2}}
		if err := svc.read(ctx, in, out); err != nil {
			t.Fatalf("read error: %v", err)
		}
		assert.Equal(t, "c\nd", out.Content)
		assert.Equal(t, "tail", out.ModeApplied)
		if assert.NotNil(t, out.Continuation) {
			assert.True(t, out.Continuation.HasMore)
		}
	})

	t.Run("line range start+count (ClipLines)", func(t *testing.T) {
		out := &ReadOutput{}
		in := &ReadInput{RootURI: rootURI, Path: linesName, LineRange: textclip.LineRange{StartLine: 2, LineCount: 2}}
		if err := svc.read(ctx, in, out); err != nil {
			t.Fatalf("read error: %v", err)
		}
		assert.Equal(t, "b\nc", out.Content)
		assert.Equal(t, 2, out.StartLine)
		assert.Equal(t, 3, out.EndLine)
		if assert.NotNil(t, out.Continuation) {
			assert.True(t, out.Continuation.HasMore)
			if assert.NotNil(t, out.Continuation.NextRange) {
				if assert.NotNil(t, out.Continuation.NextRange.Bytes) {
					assert.Equal(t, 5, out.Continuation.NextRange.Bytes.Offset)
					assert.Equal(t, 3, out.Continuation.NextRange.Bytes.Length)
				}
				// When line range is applied, continuation should include line hints.
				if assert.NotNil(t, out.Continuation.NextRange.Lines) {
					assert.Equal(t, 4, out.Continuation.NextRange.Lines.Start)
					assert.Equal(t, 2, out.Continuation.NextRange.Lines.Count)
				}
			}
		}
	})

	t.Run("line range start only to EOF (ClipLines)", func(t *testing.T) {
		out := &ReadOutput{}
		in := &ReadInput{RootURI: rootURI, Path: linesName, LineRange: textclip.LineRange{StartLine: 4}}
		if err := svc.read(ctx, in, out); err != nil {
			t.Fatalf("read error: %v", err)
		}
		// from line 4 to EOF includes trailing newline of the last line
		assert.Equal(t, "d\n", out.Content)
		assert.Equal(t, 4, out.StartLine)
		assert.Equal(t, 0, out.EndLine) // EndLine is only set when LineCount > 0
		assert.Nil(t, out.Continuation)
	})

	t.Run("line range precedence over maxBytes", func(t *testing.T) {
		out := &ReadOutput{}
		// lineRange selection must take precedence and not be further clipped
		in := &ReadInput{
			RootURI:   rootURI,
			Path:      linesName,
			LineRange: textclip.LineRange{StartLine: 2, LineCount: 2},
			MaxBytes:  1,
		}
		if err := svc.read(ctx, in, out); err != nil {
			t.Fatalf("read error: %v", err)
		}
		// Expect exact 2-line slice, ignoring max cap
		assert.Equal(t, "b\nc", out.Content)
		assert.Equal(t, 2, out.StartLine)
		assert.Equal(t, 3, out.EndLine)
		if assert.NotNil(t, out.Continuation) && assert.NotNil(t, out.Continuation.NextRange) {
			assert.True(t, out.Continuation.HasMore)
			if assert.NotNil(t, out.Continuation.NextRange.Lines) {
				assert.Equal(t, 4, out.Continuation.NextRange.Lines.Start)
				assert.Equal(t, 2, out.Continuation.NextRange.Lines.Count)
			}
		}
	})

	t.Run("line range start-only ignores maxBytes cap", func(t *testing.T) {
		out := &ReadOutput{}
		in := &ReadInput{RootURI: rootURI, Path: linesName, LineRange: textclip.LineRange{StartLine: 3}, MaxBytes: 1}
		if err := svc.read(ctx, in, out); err != nil {
			t.Fatalf("read error: %v", err)
		}
		// From line 3 to EOF should not be clipped by MaxBytes
		assert.Equal(t, "c\nd\n", out.Content)
		assert.Equal(t, 3, out.StartLine)
		assert.Equal(t, 0, out.EndLine)
		assert.Nil(t, out.Continuation)
	})

	t.Run("line range start beyond EOF yields empty and no continuation", func(t *testing.T) {
		out := &ReadOutput{}
		in := &ReadInput{RootURI: rootURI, Path: linesName, LineRange: textclip.LineRange{StartLine: 100}, MaxBytes: 5}
		if err := svc.read(ctx, in, out); err != nil {
			t.Fatalf("read error: %v", err)
		}
		assert.Equal(t, "", out.Content)
		assert.Equal(t, 100, out.StartLine)
		assert.Equal(t, 0, out.EndLine)
		assert.Equal(t, 0, out.Returned)
		assert.Equal(t, 0, out.Remaining)
		assert.Nil(t, out.Continuation)
	})
}
