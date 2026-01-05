package hygine

// LineRange represents a 1-based inclusive line window in a text document.
// Start <= 0 or Count <= 0 indicate that no explicit line window is requested.
type LineRange struct {
	Start int // 1-based start line
	Count int // number of lines from Start; 0 means unspecified
}

// ByteRange represents a byte window in a text document.
// Offset is 0-based; Length <= 0 indicates that the caller did not
// request a specific length beyond an external cap (e.g. MaxBytes).
type ByteRange struct {
	Offset int64
	Length int
}

// Snippet models a text fragment selected from a larger document.
// It is intended to be reusable by tools that present match previews
// (e.g. grep-style search over files or conversation messages).
type Snippet struct {
	// StartLine and EndLine are 1-based line numbers within the source
	// document. When unknown, they may be left as 0.
	StartLine int
	EndLine   int

	// OffsetBytes is the byte offset of the first byte of Text within
	// the original document. LengthBytes is the size of Text in bytes.
	OffsetBytes int64
	LengthBytes int

	// Text contains the snippet content. Implementations may apply
	// additional truncation to enforce byte/line limits.
	Text string

	// Hits optionally records match positions as (lineOffset, columnOffset)
	// pairs relative to StartLine within this snippet.
	Hits [][2]int

	// Cut indicates that the snippet was truncated due to configured
	// limits (e.g. max bytes or max lines).
	Cut bool
}

// GrepStats summarizes a grep-style search across one or more files.
// It is generic enough to be reused by resources- and message-level
// tools that perform lexical search.
type GrepStats struct {
	Scanned   int  // total files scanned
	Matched   int  // files with at least one match
	Truncated bool // true when limits prevented a full scan
}

// GrepFile groups snippets belonging to a single source file.
type GrepFile struct {
	// Path is the logical path under the effective root (e.g. workspace-
	// relative or root-relative). URI may carry the canonical address
	// when needed by higher layers (e.g. workspace://... or file://...).
	Path string
	URI  string

	// SearchHash is a stable hash of the grep query parameters used to
	// produce this result set (e.g. pattern, root, path, filters). It allows
	// UIs to de-duplicate or uniquely identify rows across searches.
	SearchHash string

	// RangeKey is an optional, human-readable range identifier derived from
	// the first snippet window (e.g. "12-24"). It is intended as a secondary
	// disambiguator when needed by UIs.
	RangeKey string

	Matches  int     // number of matches in this file
	Score    float32 // optional heuristic score (e.g. density of matches)
	Snippets []Snippet
	Omitted  int // number of snippets or matches omitted due to limits
}
