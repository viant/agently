package resources

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	aug "github.com/viant/agently/genai/service/augmenter"
	"github.com/viant/agently/genai/textclip"
)

func iptr(v int) *int { return &v }

func TestService_Read_RangeVariants(t *testing.T) {
	// Prepare workspace test folder under a temp directory to avoid permission issues
	tempDir := t.TempDir()
	base := filepath.Join(tempDir, "test_resources_ranges")
	_ = os.MkdirAll(base, 0755)

	// File for byte-range test
	fileBytes := filepath.Join(base, "bytes.txt")
	bytesContent := []byte("abcdefghijklmnopqrstuvwxyz")
	if err := os.WriteFile(fileBytes, bytesContent, 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	// File for line-range test
	fileLines := filepath.Join(base, "lines.txt")
	linesContent := "a\nb\nc\nd\n"
	if err := os.WriteFile(fileLines, []byte(linesContent), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	// File for default limit truncation test (>8KB)
	largeContent := strings.Repeat("0123456789", 900)
	largeFile := filepath.Join(base, "large.txt")
	if err := os.WriteFile(largeFile, []byte(largeContent), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	service := New(aug.New(nil))
	ctx := context.Background()
	rootURI := "file://" + base

	type got struct {
		Content   string
		StartLine int
		EndLine   int
		Size      int
	}

	cases := []struct {
		name               string
		input              *ReadInput
		expected           got
		expectContinuation bool
	}{
		{
			name:               "byteRange from 2 to 5",
			input:              &ReadInput{RootURI: rootURI, Path: "bytes.txt", BytesRange: textclip.BytesRange{OffsetBytes: 2, LengthBytes: 3}},
			expected:           got{Content: "cde", StartLine: 0, EndLine: 0, Size: len(bytesContent)},
			expectContinuation: true,
		},
		{
			name:               "lineRange StartLine=2, Count=2",
			input:              &ReadInput{RootURI: rootURI, Path: "lines.txt", LineRange: textclip.LineRange{StartLine: 2, LineCount: 2}},
			expected:           got{Content: "b\nc", StartLine: 2, EndLine: 3, Size: len(linesContent)},
			expectContinuation: true,
		},
		{
			name: "maxBytes truncation adds continuation",
			input: &ReadInput{
				RootURI:  rootURI,
				Path:     "bytes.txt",
				MaxBytes: 5,
			},
			expected:           got{Content: "abcde", StartLine: 0, EndLine: 0, Size: len(bytesContent)},
			expectContinuation: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := &ReadOutput{}
			if err := service.read(ctx, tc.input, out); err != nil {
				t.Fatalf("read returned error: %v", err)
			}
			actual := got{Content: out.Content, StartLine: out.StartLine, EndLine: out.EndLine, Size: out.Size}
			assert.EqualValues(t, tc.expected, actual)
			if tc.expectContinuation {
				if assert.NotNil(t, out.Continuation, "expected continuation to be set") {
					assert.True(t, out.Continuation.HasMore, "expected continuation.HasMore to be true")
				}
			} else {
				assert.Nil(t, out.Continuation)
			}
		})
	}

	t.Run("default limit does not expose continuation", func(t *testing.T) {
		out := &ReadOutput{}
		input := &ReadInput{RootURI: rootURI, Path: "large.txt"}
		if err := service.read(ctx, input, out); err != nil {
			t.Fatalf("read returned error: %v", err)
		}
		assert.Nil(t, out.Continuation, "default limit should not set continuation")
		assert.True(t, len(out.Content) > 0)
	})
}
