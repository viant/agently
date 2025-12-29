package resources

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	aug "github.com/viant/agently/genai/service/augmenter"
	"github.com/viant/agently/genai/textclip"
)

func TestService_Read_LineRange_LineCount(t *testing.T) {
	ctx := context.Background()
	svc := New(aug.New(nil))

	tempDir := t.TempDir()
	rootURI := "file://" + tempDir

	write := func(name, content string) {
		t.Helper()
		require.NoError(t, os.WriteFile(filepath.Join(tempDir, name), []byte(content), 0o644))
	}

	write("six_lines_nl.txt", "1\n2\n3\n4\n5\n6\n")
	write("four_lines_nl.txt", "a\nb\nc\nd\n")
	write("four_lines_no_nl.txt", "a\nb\nc\nd")

	type want struct {
		content         string
		startLine       int
		endLine         int
		hasContinuation bool
		nextBytesOffset int
		nextBytesLength int
		nextLinesStart  int
		nextLinesCount  int
	}

	cases := []struct {
		name string
		in   *ReadInput
		want want
	}{
		{
			name: "lineRange(start=1,count=4)",
			in: &ReadInput{
				RootURI:   rootURI,
				Path:      "six_lines_nl.txt",
				LineRange: textclip.LineRange{StartLine: 1, LineCount: 4},
			},
			want: want{
				content:         "1\n2\n3\n4",
				startLine:       1,
				endLine:         4,
				hasContinuation: true,
				nextBytesOffset: 7,
				nextBytesLength: 5,
				nextLinesStart:  5,
				nextLinesCount:  4,
			},
		},
		{
			name: "lineRange(start=1,count=4) does not depend on mode lineCount",
			in: &ReadInput{
				RootURI:   rootURI,
				Path:      "six_lines_nl.txt",
				LineRange: textclip.LineRange{StartLine: 1, LineCount: 4},
			},
			want: want{
				content:         "1\n2\n3\n4",
				startLine:       1,
				endLine:         4,
				hasContinuation: true,
				nextBytesOffset: 7,
				nextBytesLength: 5,
				nextLinesStart:  5,
				nextLinesCount:  4,
			},
		},
		{
			name: "lineRange(start=1,count=4) with maxBytes is not clipped",
			in: &ReadInput{
				RootURI:   rootURI,
				Path:      "six_lines_nl.txt",
				LineRange: textclip.LineRange{StartLine: 1, LineCount: 4},
				MaxBytes:  1,
			},
			want: want{
				content:         "1\n2\n3\n4",
				startLine:       1,
				endLine:         4,
				hasContinuation: true,
				nextBytesOffset: 7,
				nextBytesLength: 5,
				nextLinesStart:  5,
				nextLinesCount:  4,
			},
		},
		{
			name: "lineRange(start=2,count=2)",
			in: &ReadInput{
				RootURI:   rootURI,
				Path:      "six_lines_nl.txt",
				LineRange: textclip.LineRange{StartLine: 2, LineCount: 2},
			},
			want: want{
				content:         "2\n3",
				startLine:       2,
				endLine:         3,
				hasContinuation: true,
				nextBytesOffset: 5,
				nextBytesLength: 3,
				nextLinesStart:  4,
				nextLinesCount:  2,
			},
		},
		{
			name: "lineRange(start=3,count=0) to EOF",
			in: &ReadInput{
				RootURI:   rootURI,
				Path:      "six_lines_nl.txt",
				LineRange: textclip.LineRange{StartLine: 3},
			},
			want: want{
				content:         "3\n4\n5\n6\n",
				startLine:       3,
				endLine:         0,
				hasContinuation: false,
			},
		},
		{
			name: "lineRange count past EOF returns to EOF",
			in: &ReadInput{
				RootURI:   rootURI,
				Path:      "six_lines_nl.txt",
				LineRange: textclip.LineRange{StartLine: 5, LineCount: 10},
			},
			want: want{
				content:         "5\n6\n",
				startLine:       5,
				endLine:         14,
				hasContinuation: false,
			},
		},
		{
			name: "lineRange covering entire file with trailing newline has no continuation",
			in: &ReadInput{
				RootURI:   rootURI,
				Path:      "four_lines_nl.txt",
				LineRange: textclip.LineRange{StartLine: 1, LineCount: 4},
			},
			want: want{
				content:         "a\nb\nc\nd\n",
				startLine:       1,
				endLine:         4,
				hasContinuation: false,
			},
		},
		{
			name: "lineRange covering entire file without trailing newline has no continuation",
			in: &ReadInput{
				RootURI:   rootURI,
				Path:      "four_lines_no_nl.txt",
				LineRange: textclip.LineRange{StartLine: 1, LineCount: 4},
			},
			want: want{
				content:         "a\nb\nc\nd",
				startLine:       1,
				endLine:         4,
				hasContinuation: false,
			},
		},
		{
			name: "lineRange start beyond EOF yields empty and no continuation",
			in: &ReadInput{
				RootURI:   rootURI,
				Path:      "four_lines_nl.txt",
				LineRange: textclip.LineRange{StartLine: 10, LineCount: 4},
			},
			want: want{
				content:         "",
				startLine:       10,
				endLine:         13,
				hasContinuation: false,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := &ReadOutput{}
			require.NoError(t, svc.read(ctx, tc.in, out))

			assert.Equal(t, tc.want.content, out.Content)
			assert.Equal(t, tc.want.startLine, out.StartLine)
			assert.Equal(t, tc.want.endLine, out.EndLine)

			if !tc.want.hasContinuation {
				assert.Nil(t, out.Continuation)
				return
			}

			if assert.NotNil(t, out.Continuation) {
				assert.True(t, out.Continuation.HasMore)
				if assert.NotNil(t, out.Continuation.NextRange) && assert.NotNil(t, out.Continuation.NextRange.Bytes) {
					assert.Equal(t, tc.want.nextBytesOffset, out.Continuation.NextRange.Bytes.Offset)
					assert.Equal(t, tc.want.nextBytesLength, out.Continuation.NextRange.Bytes.Length)
				}
				if tc.want.endLine > 0 && assert.NotNil(t, out.Continuation.NextRange) && assert.NotNil(t, out.Continuation.NextRange.Lines) {
					assert.Equal(t, tc.want.nextLinesStart, out.Continuation.NextRange.Lines.Start)
					assert.Equal(t, tc.want.nextLinesCount, out.Continuation.NextRange.Lines.Count)
				}
			}
		})
	}
}
