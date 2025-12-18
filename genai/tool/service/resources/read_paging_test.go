package resources

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/viant/agently/genai/textclip"
)

func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	return "file://" + path
}

// TestRead_HeadPaging_NextRange verifies head-mode paging with MaxBytes emits a
// correct continuation hint (next byte offset/length) and accurate remaining.
func TestRead_HeadPaging_NextRange(t *testing.T) {
	uri := writeTempFile(t, strings.Repeat("x", 26)) // size=26
	svc := New(nil)
	in := &ReadInput{URI: uri, MaxBytes: 10}
	var out ReadOutput
	err := svc.read(context.Background(), in, &out)
	assert.NoError(t, err)
	assert.Equal(t, 26, out.Size)
	assert.Equal(t, 10, out.Returned)
	assert.Equal(t, 16, out.Remaining)
	if assert.NotNil(t, out.Continuation) && assert.NotNil(t, out.Continuation.NextRange) {
		assert.Equal(t, 10, out.Continuation.NextRange.Bytes.Offset)
		assert.Equal(t, 10, out.Continuation.NextRange.Bytes.Length)
	}
}

// TestRead_ByteRange_PagingSequence validates multi-page byte-range reads where
// nextOffset advances by returned size and nextLength shrinks on the final page.
func TestRead_ByteRange_PagingSequence(t *testing.T) {
	uri := writeTempFile(t, strings.Repeat("a", 26)) // size=26
	svc := New(nil)

	// Page 1: [8,16)
	var out1 ReadOutput
	err := svc.read(context.Background(), &ReadInput{URI: uri, BytesRange: textclip.BytesRange{OffsetBytes: 8, LengthBytes: 8}}, &out1)
	assert.NoError(t, err)
	assert.Equal(t, 8, out1.Returned)
	assert.Equal(t, 10, out1.Remaining)
	if assert.NotNil(t, out1.Continuation) && assert.NotNil(t, out1.Continuation.NextRange) {
		assert.Equal(t, 16, out1.Continuation.NextRange.Bytes.Offset)
		assert.Equal(t, 8, out1.Continuation.NextRange.Bytes.Length)
	}

	// Page 2: [16,24)
	var out2 ReadOutput
	err = svc.read(context.Background(), &ReadInput{URI: uri, BytesRange: textclip.BytesRange{OffsetBytes: 16, LengthBytes: 8}}, &out2)
	assert.NoError(t, err)
	assert.Equal(t, 8, out2.Returned)
	assert.Equal(t, 2, out2.Remaining)
	if assert.NotNil(t, out2.Continuation) && assert.NotNil(t, out2.Continuation.NextRange) {
		assert.Equal(t, 24, out2.Continuation.NextRange.Bytes.Offset)
		assert.Equal(t, 2, out2.Continuation.NextRange.Bytes.Length)
	}

	// Page 3: [24,26)
	var out3 ReadOutput
	err = svc.read(context.Background(), &ReadInput{URI: uri, BytesRange: textclip.BytesRange{OffsetBytes: 24, LengthBytes: 8}}, &out3)
	assert.NoError(t, err)
	assert.Equal(t, 2, out3.Returned)
	assert.Equal(t, 0, out3.Remaining)
	assert.Nil(t, out3.Continuation)
}

// TestRead_HeadNoContinuation ensures no continuation is emitted when MaxBytes
// exceeds the file size and the page is not truncated.
func TestRead_HeadNoContinuation(t *testing.T) {
	uri := writeTempFile(t, strings.Repeat("z", 20))
	svc := New(nil)
	var out ReadOutput
	err := svc.read(context.Background(), &ReadInput{URI: uri, MaxBytes: 30}, &out)
	assert.NoError(t, err)
	assert.Equal(t, 20, out.Returned)
	assert.Equal(t, 0, out.Remaining)
	assert.Nil(t, out.Continuation)
}

// TestRead_TailPaging verifies tail-mode returns the last N bytes and still
// provides a continuation hint to fetch the preceding range if desired.
func TestRead_TailPaging(t *testing.T) {
	content := "abcdefghijklmnopqrstuvwxyz"
	uri := writeTempFile(t, content)
	svc := New(nil)
	var out ReadOutput
	err := svc.read(context.Background(), &ReadInput{URI: uri, Mode: "tail", MaxBytes: 10}, &out)
	assert.NoError(t, err)
	assert.Equal(t, 10, out.Returned)
	assert.Equal(t, 16, out.Remaining)
	assert.Equal(t, content[16:], out.Content)
	if assert.NotNil(t, out.Continuation) && assert.NotNil(t, out.Continuation.NextRange) {
		assert.Equal(t, 10, out.Continuation.NextRange.Bytes.Offset)
		assert.Equal(t, 10, out.Continuation.NextRange.Bytes.Length)
	}
}

// TestRead_ByteRange_OffsetPastEOF confirms offset beyond EOF yields zero bytes
// and suppresses continuation.
func TestRead_ByteRange_OffsetPastEOF(t *testing.T) {
	uri := writeTempFile(t, strings.Repeat("b", 10))
	svc := New(nil)
	var out ReadOutput
	err := svc.read(context.Background(), &ReadInput{URI: uri, BytesRange: textclip.BytesRange{OffsetBytes: 999, LengthBytes: 5}}, &out)
	assert.NoError(t, err)
	assert.Equal(t, 0, out.Returned)
	assert.Equal(t, 0, out.Remaining)
	assert.Nil(t, out.Continuation)
}

// TestRead_LineRange_PagingHints checks that line-range reads provide both byte
// and line continuation hints so callers can continue by lines without guessing.
func TestRead_LineRange_PagingHints(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 10; i++ {
		sb.WriteString("L")
		sb.WriteString(strconv.Itoa(i))
		sb.WriteString("\n")
	}
	uri := writeTempFile(t, sb.String())
	svc := New(nil)
	var out ReadOutput
	err := svc.read(context.Background(), &ReadInput{URI: uri, LineRange: textclip.LineRange{StartLine: 3, LineCount: 4}}, &out)
	assert.NoError(t, err)
	assert.Greater(t, out.Returned, 0)
	assert.Greater(t, out.Remaining, 0)
	if assert.NotNil(t, out.Continuation) && assert.NotNil(t, out.Continuation.NextRange) {
		assert.Greater(t, out.Continuation.NextRange.Bytes.Offset, 0)
		assert.Greater(t, out.Continuation.NextRange.Bytes.Length, 0)
		if assert.NotNil(t, out.Continuation.NextRange.Lines) {
			assert.Equal(t, out.EndLine+1, out.Continuation.NextRange.Lines.Start)
			assert.GreaterOrEqual(t, out.Continuation.NextRange.Lines.Count, 1)
		}
	}
}

// TestRead_BinaryGuard_NoContinuationByDefault asserts binary content is not
// inlined; instead a placeholder is returned with no continuation unless the
// caller explicitly requests paging.
func TestRead_BinaryGuard_NoContinuationByDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bin.dat")
	data := []byte{0x00, 0x01, 0x02, 0x03, 0x04}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	uri := "file://" + path
	svc := New(nil)
	var out ReadOutput
	err := svc.read(context.Background(), &ReadInput{URI: uri}, &out)
	assert.NoError(t, err)
	assert.True(t, out.Binary)
	assert.Equal(t, len(data), out.Remaining)
	assert.Equal(t, "[binary content omitted]", out.Content)
	assert.Nil(t, out.Continuation)
}
