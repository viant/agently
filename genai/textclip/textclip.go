package textclip

import (
	"errors"
	"regexp"
	"strings"
)

// IntRange represents a half-open interval [From, To) with 0-based indices.
// A nil range, or missing endpoints, is considered invalid by helpers.
type IntRange struct {
	From *int `json:"from,omitempty"`
	To   *int `json:"to,omitempty"`
}

type BytesRange struct {
	// byte range is ignored. When both byte and line ranges are unset, the
	// full (optionally MaxBytes-truncated) content is returned.
	OffsetBytes int64 `json:"offsetBytes,omitempty"` // 0-based byte offset; default 0
	LengthBytes int   `json:"lengthBytes,omitempty"` // bytes to read; 0 => use MaxBytes cap

}

type LineRange struct {
	// OffsetBytes and LengthBytes describe a 0-based byte window within the
	// file. When StartLine is > 0, the line range takes precedence and the

	// StartLine and LineCount describe a 1-based line slice. When StartLine > 0
	// this mode is used in preference to the byte range.
	StartLine int `json:"startLine,omitempty"` // 1-based start line
	LineCount int `json:"lineCount,omitempty"` // number of lines; 0 => until EOF or MaxBytes

}

func rngOK(r *IntRange) bool {
	return r != nil && r.From != nil && r.To != nil && *r.From >= 0 && *r.To >= *r.From
}

// ClipBytesByRange clips using BytesRange (offset+length) without exposing IntRange.
// When both fields are zero, the original slice is returned.
func ClipBytesByRange(b []byte, br BytesRange) ([]byte, int, int, error) {
	if br.OffsetBytes <= 0 && br.LengthBytes <= 0 {
		return b, 0, len(b), nil
	}
	start := int(br.OffsetBytes)
	end := start
	if br.LengthBytes > 0 {
		end = start + br.LengthBytes
	} else {
		end = len(b)
	}
	r := &IntRange{From: &start, To: &end}
	return ClipBytes(b, r)
}

// ClipLinesByRange clips using LineRange (StartLine+LineCount) without exposing IntRange.
// When both fields are zero, the original slice is returned.
func ClipLinesByRange(b []byte, lr LineRange) ([]byte, int, int, error) {
	if lr.StartLine <= 0 && lr.LineCount <= 0 {
		return b, 0, len(b), nil
	}
	from := lr.StartLine - 1
	if from < 0 {
		from = 0
	}
	to := from
	if lr.LineCount > 0 {
		to = from + lr.LineCount
	} else {
		to = from + 1_000_000_000
	}
	r := &IntRange{From: &from, To: &to}
	return ClipLines(b, r)
}

// ClipBytes returns a byte slice clipped to r and the start/end offsets.
// When r is nil, the original slice and full offsets are returned.
func ClipBytes(b []byte, r *IntRange) ([]byte, int, int, error) {
	if r == nil {
		return b, 0, len(b), nil
	}
	if !rngOK(r) {
		return nil, 0, 0, errors.New("invalid byteRange")
	}
	start := *r.From
	end := *r.To
	if start < 0 {
		start = 0
	}
	if start > len(b) {
		start = len(b)
	}
	if end < start {
		end = start
	}
	if end > len(b) {
		end = len(b)
	}
	return b[start:end], start, end, nil
}

// ClipLines returns a byte slice corresponding to line range r and the start/end
// byte offsets mapped from lines. Lines are 0-based; To is exclusive.
// When r is nil, the original slice and full offsets are returned.
func ClipLines(b []byte, r *IntRange) ([]byte, int, int, error) {
	if r == nil {
		return b, 0, len(b), nil
	}
	if !rngOK(r) {
		return nil, 0, 0, errors.New("invalid lineRange")
	}
	starts := []int{0}
	for i, c := range b {
		if c == '\n' && i+1 < len(b) {
			starts = append(starts, i+1)
		}
	}
	total := len(starts)
	from := *r.From
	to := *r.To
	if from < 0 {
		from = 0
	}
	if from > total {
		from = total
	}
	if to < from {
		to = from
	}
	if to > total {
		to = total
	}
	start := 0
	if from < total {
		start = starts[from]
	} else {
		start = len(b)
	}
	end := len(b)
	if to-1 < total-1 {
		end = starts[to] - 1
	}
	if end < start {
		end = start
	}
	return b[start:end], start, end, nil
}

// ClipHead returns the first portion of text.
// If maxLines > 0, it clips by lines only.
// Otherwise, if maxBytes > 0, it clips by bytes.
func ClipHead(text string, totalSize, maxBytes, maxLines int) (string, int, int) {
	if maxLines <= 0 {
		head := text
		if maxBytes > 0 && len(head) > maxBytes {
			head = head[:maxBytes]
		}
		return head, len(head), remaining(totalSize, len(head))
	}

	lines := strings.Split(text, "\n")
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	head := strings.Join(lines, "\n")
	return head, len(head), remaining(totalSize, len(head))
}

// ClipTail returns the last portion of text.
// If maxLines > 0, it clips by lines only.
// Otherwise, if maxBytes > 0, it clips by bytes.
func ClipTail(text string, totalSize, maxBytes, maxLines int) (string, int, int) {
	if maxLines <= 0 {
		tail := text
		if maxBytes > 0 && len(tail) > maxBytes {
			tail = tail[len(tail)-maxBytes:]
		}
		return tail, len(tail), remaining(totalSize, len(tail))
	}

	lines := strings.Split(text, "\n")
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	tail := strings.Join(lines, "\n")
	return tail, len(tail), remaining(totalSize, len(tail))
}

// ExtractSignatures collects signature-like lines and optionally bounds the response by maxBytes.
func ExtractSignatures(text string, maxBytes int) string {
	var sigs []string
	re := regexp.MustCompile(`^\s*(public|private|protected|class|interface|func|def|package|import|\w+\s+\w+\()`)
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if re.MatchString(line) {
			sigs = append(sigs, line)
		}
		if maxBytes > 0 && len(strings.Join(sigs, "\n")) >= maxBytes {
			break
		}
	}
	result := strings.Join(sigs, "\n")
	if maxBytes > 0 && len(result) > maxBytes {
		result = result[:maxBytes]
	}
	return result
}

func remaining(totalSize, returned int) int {
	if totalSize <= returned {
		return 0
	}
	return totalSize - returned
}
