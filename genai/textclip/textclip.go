package textclip

import (
	"errors"
)

// IntRange represents a half-open interval [From, To) with 0-based indices.
// A nil range, or missing endpoints, is considered invalid by helpers.
type IntRange struct {
	From *int `json:"from,omitempty"`
	To   *int `json:"to,omitempty"`
}

func rngOK(r *IntRange) bool {
	return r != nil && r.From != nil && r.To != nil && *r.From >= 0 && *r.To >= *r.From
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
