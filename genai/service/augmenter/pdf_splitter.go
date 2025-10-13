package augmenter

import (
	"bytes"
	"unicode/utf8"

	"github.com/viant/embedius/document"
	fsplitter "github.com/viant/embedius/indexer/fs/splitter"
)

// pdfSplitter is a lightweight splitter for PDF files that attempts to
// extract printable text without requiring external PDF libraries. It is
// intentionally simple to avoid new dependencies in restricted environments.
// For best results, consider swapping with a richer extractor via embedius
// factory if available.
type pdfSplitter struct {
	// delegate performs final chunking after text extraction.
	delegate fsplitter.Splitter
}

// NewPDFSplitter returns a Splitter that extracts printable text and delegates
// to a size-based splitter for chunking.
func NewPDFSplitter(maxChunk int) fsplitter.Splitter {
	if maxChunk <= 0 {
		maxChunk = 4096
	}
	return &pdfSplitter{delegate: fsplitter.NewSizeSplitter(maxChunk)}
}

func (p *pdfSplitter) Split(data []byte, metadata map[string]interface{}) []*document.Fragment {
	// Extract a best-effort text stream from binary PDF bytes by filtering
	// printable UTF-8 runes and common whitespace. This is not a full PDF
	// parser but provides usable text for indexing in many cases.
	text := extractPrintableText(data)
	return p.delegate.Split(text, metadata)
}

// extractPrintableText filters data to printable UTF-8 with newline/tab retained.
func extractPrintableText(in []byte) []byte {
	// Fast path: assume input may contain valid UTF-8 sequences; fall back to
	// byte-wise filter on decode failure.
	var out bytes.Buffer
	for len(in) > 0 {
		r, size := utf8.DecodeRune(in)
		if r == utf8.RuneError && size == 1 { // invalid rune; drop byte
			b := in[0]
			if isPrintableASCII(b) {
				out.WriteByte(b)
			}
			in = in[1:]
			continue
		}
		in = in[size:]
		if isPrintableRune(r) {
			out.WriteRune(r)
		}
	}
	return out.Bytes()
}

func isPrintableASCII(b byte) bool {
	// Allow common whitespace and printable ASCII
	return b == '\n' || b == '\r' || b == '\t' || (b >= 32 && b < 127)
}

func isPrintableRune(r rune) bool {
	// Keep standard whitespace and visible characters; skip control codes.
	if r == '\n' || r == '\r' || r == '\t' {
		return true
	}
	// Basic ASCII visible range
	if r >= 32 && r < 127 {
		return true
	}
	// Allow letters/numbers/punctuation for common UTF-8 (best-effort)
	if r >= 127 && r <= 0x10FFFF {
		return true
	}
	return false
}
