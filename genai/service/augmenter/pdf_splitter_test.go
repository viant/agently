package augmenter

import (
	"github.com/stretchr/testify/assert"
	fsplitter "github.com/viant/embedius/indexer/fs/splitter"
	"testing"
)

func TestPDFSplitter_ExtractsPrintableText(t *testing.T) {
	s := NewPDFSplitter(128)
	// Mix of binary and printable content simulating a PDF stream
	data := append([]byte{0x25, 0x50, 0x44, 0x46, 0x2D}, []byte("\nHello PDF World!\n\x01\x02NonPrintable\x00\nThe End.")...)
	frags := s.Split(data, map[string]interface{}{"path": "dummy.pdf"})
	// Expect at least one fragment containing printable subsets
	assert.NotEqual(t, 0, len(frags))
}

func TestPDFSplitter_DelegatesChunking(t *testing.T) {
	s := NewPDFSplitter(8) // small chunk size to force multiple fragments
	txt := []byte("This is a long enough line to force multiple chunks.")
	frags := s.Split(txt, map[string]interface{}{"path": "dummy.pdf"})
	// Using embedius size splitter, should produce > 1 fragment
	assert.Greater(t, len(frags), 1)
	_ = fsplitter.NewSizeSplitter // keep import
}
