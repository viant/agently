package pdf

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"strings"
)

// WriterOptions controls pagination and font settings.
type WriterOptions struct {
	// MultiPage enables splitting content across multiple pages.
	MultiPage bool
	// ScaleToFit tries to shrink font to fit all content on a single page.
	// If it cannot fit within MinFontSize, it falls back to MultiPage.
	ScaleToFit bool

	// Font settings (points)
	MaxFontSize int // default 8
	MinFontSize int // default 6

	// Margins (points)
	MarginLeft   int // default 40
	MarginRight  int // default 40
	MarginTop    int // default 40
	MarginBottom int // default 40

	// Page size (points). Defaults to A4 portrait: 595 x 842
	PageWidth  int
	PageHeight int

	// FontName is a PDF core font name. Use Courier for monospaced wrapping.
	FontName string // default "Courier"
}

// WriteText creates a PDF from the provided text with default options:
// - MultiPage pagination enabled
// - Courier 8pt, A4 page size
func WriteText(ctx context.Context, title string, content string) ([]byte, error) {
	return WriteTextWithOptions(ctx, title, content, nil)
}

// WriteTextWithOptions creates a PDF from the provided text using WriterOptions.
func WriteTextWithOptions(_ context.Context, title string, content string, opts *WriterOptions) ([]byte, error) {
	o := normalizeOptions(opts)

	// Decide font size and wrapping
	fontSize := o.MaxFontSize
	leading := int(math.Ceil(float64(fontSize) * 1.2))
	cols := columnsPerLine(o, fontSize)
	lines := wrapText(content, cols)

	if o.ScaleToFit && !o.MultiPage {
		fitted := false
		for fs := o.MaxFontSize; fs >= o.MinFontSize; fs-- {
			l := int(math.Ceil(float64(fs) * 1.2))
			c := columnsPerLine(o, fs)
			w := wrapText(content, c)
			if len(w) <= linesPerPage(o, l) {
				fontSize, leading, cols, lines = fs, l, c, w
				fitted = true
				break
			}
		}
		if !fitted {
			// Fallback: use MinFontSize and paginate
			fontSize = o.MinFontSize
			leading = int(math.Ceil(float64(fontSize) * 1.2))
			cols = columnsPerLine(o, fontSize)
			lines = wrapText(content, cols)
			o.MultiPage = true
		}
	}

	// Prepare pages
	var pages [][]string
	if o.MultiPage {
		lpp := linesPerPage(o, leading)
		pages = paginate(lines, lpp)
	} else {
		pages = [][]string{lines}
	}

	// Build page content streams
	streams := make([][]byte, len(pages))
	for i := range pages {
		streams[i] = makeStream(pages[i], o, fontSize, leading)
	}

	// Build PDF objects: 1 Catalog, 2 Pages, N Page, N Content, 1 Font, 1 Info
	var buf bytes.Buffer
	write := func(s string) { _, _ = buf.WriteString(s) }
	offsets := make([]int, 0, 4+2*len(streams))
	xref := func() { offsets = append(offsets, buf.Len()) }

	write("%PDF-1.4\n")
	// Catalog
	xref()
	write("1 0 obj\n")
	write("<< /Type /Catalog /Pages 2 0 R >>\n")
	write("endobj\n")

	// Pages
	xref()
	write("2 0 obj\n")
	write(fmt.Sprintf("<< /Type /Pages /Count %d /Kids [", len(streams)))
	for i := 0; i < len(streams); i++ {
		write(fmt.Sprintf(" %d 0 R", 3+i))
	}
	write(" ] >>\n")
	write("endobj\n")

	// Page objects
	for i := range streams {
		xref()
		write(fmt.Sprintf("%d 0 obj\n", 3+i))
		write(fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %d %d] ", o.PageWidth, o.PageHeight))
		write("/Resources << /Font << /F1 ")
		fontObjID := 3 + len(streams) + len(streams)
		write(fmt.Sprintf("%d 0 R >> >> ", fontObjID))
		contentObjID := 3 + len(streams) + i
		write(fmt.Sprintf("/Contents %d 0 R >>\n", contentObjID))
		write("endobj\n")
	}

	// Content objects
	for i, stream := range streams {
		xref()
		write(fmt.Sprintf("%d 0 obj\n", 3+len(streams)+i))
		write(fmt.Sprintf("<< /Length %d >>\n", len(stream)))
		write("stream\n")
		_, _ = buf.Write(stream)
		write("endstream\n")
		write("endobj\n")
	}

	// Font
	xref()
	fontObjID := 3 + len(streams) + len(streams)
	write(fmt.Sprintf("%d 0 obj\n", fontObjID))
	write("<< /Type /Font /Subtype /Type1 /BaseFont /")
	write(o.FontName)
	write(" >>\n")
	write("endobj\n")

	// Info (title)
	xref()
	infoObjID := fontObjID + 1
	write(fmt.Sprintf("%d 0 obj\n", infoObjID))
	if title != "" {
		write("<< /Title (")
		write(escapePDFString(title))
		write(") >>\n")
	} else {
		write("<< >>\n")
	}
	write("endobj\n")

	// XRef
	xrefStart := buf.Len()
	write("xref\n")
	write(fmt.Sprintf("0 %d\n", len(offsets)+1))
	write("0000000000 65535 f \n")
	for _, off := range offsets {
		write(fmt.Sprintf("%010d 00000 n \n", off))
	}
	write("trailer\n")
	write(fmt.Sprintf("<< /Size %d /Root 1 0 R /Info %d 0 R >>\n", len(offsets)+1, infoObjID))
	write("startxref\n")
	write(fmt.Sprintf("%d\n", xrefStart))
	write("%EOF\n")

	return buf.Bytes(), nil
}

func normalizeOptions(in *WriterOptions) *WriterOptions {
	o := &WriterOptions{}
	if in != nil {
		*o = *in
	}
	if o.PageWidth == 0 {
		o.PageWidth = 595
	}
	if o.PageHeight == 0 {
		o.PageHeight = 842
	}
	if o.MaxFontSize == 0 {
		o.MaxFontSize = 8
	}
	if o.MinFontSize == 0 {
		o.MinFontSize = 6
	}
	if o.MarginLeft == 0 {
		o.MarginLeft = 40
	}
	if o.MarginRight == 0 {
		o.MarginRight = 40
	}
	if o.MarginTop == 0 {
		o.MarginTop = 40
	}
	if o.MarginBottom == 0 {
		o.MarginBottom = 40
	}
	if o.FontName == "" {
		o.FontName = "Courier"
	}
	// Default to multipage if neither option is set
	if !o.MultiPage && !o.ScaleToFit {
		o.MultiPage = true
	}
	// Ensure Min <= Max
	if o.MinFontSize > o.MaxFontSize {
		o.MinFontSize = o.MaxFontSize
	}
	return o
}

func usableWidth(o *WriterOptions) int  { return o.PageWidth - o.MarginLeft - o.MarginRight }
func usableHeight(o *WriterOptions) int { return o.PageHeight - o.MarginTop - o.MarginBottom }

// columnsPerLine estimates columns for monospaced font (Courier).
// Courier char width ≈ 600 units per 1000 em; width in points ≈ 0.6 * fontSize per char.
func columnsPerLine(o *WriterOptions, fontSize int) int {
	if fontSize <= 0 {
		return 1
	}
	perChar := 0.6 * float64(fontSize)
	cols := int(float64(usableWidth(o)) / perChar)
	if cols < 1 {
		cols = 1
	}
	return cols
}

// linesPerPage computes how many lines fit vertically for given leading.
func linesPerPage(o *WriterOptions, leading int) int {
	if leading <= 0 {
		leading = 1
	}
	return int(float64(usableHeight(o)) / float64(leading))
}

// wrapText hard-wraps at the given column width with a simple word-aware strategy.
func wrapText(text string, cols int) []string {
	if cols <= 1 {
		cols = 1
	}
	var out []string
	for _, rawLine := range strings.Split(text, "\n") {
		line := strings.TrimRight(rawLine, "\r")
		if len(line) == 0 {
			out = append(out, "")
			continue
		}
		for len(line) > cols {
			// Try to break at last space within limit
			cut := lastSpaceBefore(line, cols)
			if cut <= 0 {
				cut = cols
			}
			out = append(out, line[:cut])
			// Trim leading spaces on next line
			line = strings.TrimLeft(line[cut:], " ")
		}
		out = append(out, line)
	}
	return out
}

func lastSpaceBefore(s string, idx int) int {
	if idx > len(s) {
		idx = len(s)
	}
	for i := idx; i >= 0; i-- {
		if i < len(s) && s[i] == ' ' {
			return i
		}
	}
	return -1
}

func paginate(lines []string, lpp int) [][]string {
	if lpp <= 0 {
		return [][]string{lines}
	}
	pages := make([][]string, 0, 1+len(lines)/lpp)
	for i := 0; i < len(lines); i += lpp {
		j := i + lpp
		if j > len(lines) {
			j = len(lines)
		}
		pages = append(pages, lines[i:j])
	}
	return pages
}

// makeStream builds a page text stream for given lines.
func makeStream(lines []string, o *WriterOptions, fontSize int, leading int) []byte {
	var b bytes.Buffer
	b.WriteString("BT\n")
	fmt.Fprintf(&b, "/F1 %d Tf\n", fontSize)
	fmt.Fprintf(&b, "%d TL\n", leading)
	// Position at (left margin, top start)
	startY := o.PageHeight - o.MarginTop
	fmt.Fprintf(&b, "%d %d Td\n", o.MarginLeft, startY)
	for i, line := range lines {
		if i > 0 {
			b.WriteString("T*\n")
		}
		b.WriteString("(")
		b.WriteString(escapePDFString(line))
		b.WriteString(") Tj\n")
	}
	b.WriteString("ET\n")
	return b.Bytes()
}

// escapePDFString escapes backslashes and parentheses.
func escapePDFString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "(", `\(`)
	s = strings.ReplaceAll(s, ")", `\)`)
	return s
}
