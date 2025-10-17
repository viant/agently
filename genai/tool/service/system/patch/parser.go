package patch

// Package v1 provides a parser for the Codex "apply_patch" format. It mirrors
// the semantics of the Rust/TypeScript reference implementations while being
// implemented in Go with the github.com/viant/parsly tokenizer.

import (
	"fmt"
	"strings"

	"github.com/viant/parsly"
	"github.com/viant/parsly/matcher"
)

// ---------------------------------------------------------------------------
// Public model
// ---------------------------------------------------------------------------

// Hunk represents any high-level patch operation.
type Hunk interface{ hunkMarker() }

// AddFile operation (*** Add File: path).
type AddFile struct {
	Path     string
	Contents string
}

func (AddFile) hunkMarker() {}

// DeleteFile operation (*** Delete File: path).
type DeleteFile struct{ Path string }

func (DeleteFile) hunkMarker() {}

// UpdateFile operation (*** Update File: path).
type UpdateFile struct {
	Path     string
	MovePath string
	Chunks   []UpdateChunk
}

func (UpdateFile) hunkMarker() {}

// UpdateChunk represents a contiguous diff chunk inside an UpdateFile hunk.
type UpdateChunk struct {
	ChangeContext string   // optional text after @@
	OldLines      []string // '-' and ' ' lines (without prefix)
	NewLines      []string // '+' and ' ' lines (without prefix)
	IsEOF         bool     // true when *** End of File terminator is present
}

// ---------------------------------------------------------------------------
// Top-level entry
// ---------------------------------------------------------------------------

// Parse parses patchText and returns a slice of hunks.
func Parse(patchText string) ([]Hunk, error) {
	p := &parser{cursor: parsly.NewCursor("patch", []byte(strings.TrimSpace(patchText)), 0)}
	return p.parse()
}

// ---------------------------------------------------------------------------
// Internal implementation
// ---------------------------------------------------------------------------

// Token codes (arbitrary but unique; start at 1 to avoid clash with parsly.EOF).
const (
	tBeginPatch = iota + 1
	tEndPatch
	tAddFile
	tDeleteFile
	tUpdateFile
	tMoveTo
	tChunkHeaderCtx    // @@ something
	tChunkHeaderSimple // @@
	tEndOfFileMarker
)

var (
	tokWS = parsly.NewToken(0, "WS", matcher.NewWhiteSpace())

	tokBeginPatch = parsly.NewToken(tBeginPatch, "BeginPatch", matcher.NewFragment("*** Begin Patch"))
	tokEndPatch   = parsly.NewToken(tEndPatch, "EndPatch", matcher.NewFragment("*** End Patch"))

	tokAddFile    = parsly.NewToken(tAddFile, "AddFile", matcher.NewFragment("*** Add File:"))
	tokDeleteFile = parsly.NewToken(tDeleteFile, "DeleteFile", matcher.NewFragment("*** Delete File:"))
	tokUpdateFile = parsly.NewToken(tUpdateFile, "UpdateFile", matcher.NewFragment("*** Update File:"))
	tokMoveTo     = parsly.NewToken(tMoveTo, "MoveTo", matcher.NewFragment("*** Move to:"))

	tokChunkHeaderCtx    = parsly.NewToken(tChunkHeaderCtx, "ChunkCtx", matcher.NewFragment("@@ "))
	tokChunkHeaderSimple = parsly.NewToken(tChunkHeaderSimple, "ChunkSimple", matcher.NewFragment("@@"))

	tokEOFMarker = parsly.NewToken(tEndOfFileMarker, "EOFMarker", matcher.NewFragment("*** End of File"))
)

type parser struct {
	cursor *parsly.Cursor
}

func (p *parser) parse() ([]Hunk, error) {
	cur := p.cursor

	// Must start with *** Begin Patch
	if cur.MatchOne(tokBeginPatch).Code != tBeginPatch {
		return nil, cur.NewError(tokBeginPatch)
	}
	p.consumeLine() // skip to newline

	var hunks []Hunk

	for {
		match := cur.MatchAfterOptional(tokWS, tokEndPatch, tokAddFile, tokDeleteFile, tokUpdateFile)

		switch match.Code {
		case tEndPatch:
			p.consumeLine()
			// optional trailing whitespace
			p.skipWhitespace()
			if cur.HasMore() {
				return nil, fmt.Errorf("unexpected content after '*** End Patch'")
			}
			return hunks, nil

		case tAddFile:
			h, err := p.parseAddFile()
			if err != nil {
				return nil, err
			}
			hunks = append(hunks, h)

		case tDeleteFile:
			h, err := p.parseDeleteFile()
			if err != nil {
				return nil, err
			}
			hunks = append(hunks, h)

		case tUpdateFile:
			h, err := p.parseUpdateFile()
			if err != nil {
				return nil, err
			}
			hunks = append(hunks, h)

		case parsly.EOF:
			return nil, fmt.Errorf("unexpected EOF – missing '*** End Patch'")

		default:
			return nil, cur.NewError(tokAddFile, tokDeleteFile, tokUpdateFile, tokEndPatch)
		}
	}
}

// ---------------- Specific hunk parsers ----------------

func (p *parser) parseAddFile() (AddFile, error) {
	path := strings.TrimSpace(p.consumeUntil('\n'))

	var b strings.Builder
	for {
		line := p.peekLine()
		if len(line) == 0 || line[0] != '+' {
			break
		}
		p.consumeRune() // remove '+'
		b.WriteString(p.consumeUntil('\n'))
		b.WriteByte('\n')
	}
	return AddFile{Path: path, Contents: b.String()}, nil
}

func (p *parser) parseDeleteFile() (DeleteFile, error) {
	return DeleteFile{Path: strings.TrimSpace(p.consumeUntil('\n'))}, nil
}

func (p *parser) parseUpdateFile() (UpdateFile, error) {
	cur := p.cursor

	path := strings.TrimSpace(p.consumeUntil('\n'))

	// Optional move
	var movePath string
	if cur.MatchAfterOptional(tokWS, tokMoveTo).Code == tMoveTo {
		movePath = strings.TrimSpace(p.consumeUntil('\n'))
	}

	var chunks []UpdateChunk
	firstChunk := true
	for {
		if strings.HasPrefix(p.peekLine(), "***") {
			break // next hunk
		}

		// Chunk header (optional for first chunk)
		var changeCtx string
		hMatch := cur.MatchAfterOptional(tokWS, tokChunkHeaderCtx, tokChunkHeaderSimple)
		switch hMatch.Code {
		case tChunkHeaderCtx:
			changeCtx = strings.TrimSpace(p.consumeUntil('\n'))
		case tChunkHeaderSimple:
			p.consumeUntil('\n')
		default:
			if !firstChunk {
				return UpdateFile{}, fmt.Errorf("expected @@ header in update hunk for %s", path)
			}
		}
		firstChunk = false

		oldLines := []string{}
		newLines := []string{}
		isEOF := false

	lineLoop:
		for cur.HasMore() {
			l := p.peekLine()

			switch {
			case strings.HasPrefix(l, "*** End of File"):
				isEOF = true
				p.consumeLine()
				break lineLoop

			case strings.HasPrefix(l, "@@"):
				// next chunk header begins – break current chunk
				break lineLoop

			case strings.HasPrefix(l, "***"):
				// next top-level hunk
				break lineLoop

			case len(l) == 0:
				p.consumeLine()
				oldLines = append(oldLines, "")
				newLines = append(newLines, "")

			case l[0] == '+':
				p.consumeRune()
				newLines = append(newLines, p.consumeUntil('\n'))

			case l[0] == '-':
				p.consumeRune()
				oldLines = append(oldLines, p.consumeUntil('\n'))

			case l[0] == ' ':
				p.consumeRune()
				txt := p.consumeUntil('\n')
				oldLines = append(oldLines, txt)
				newLines = append(newLines, txt)

			default:
				// treat as end-of-chunk
				break lineLoop
			}
		}

		if len(oldLines) == 0 && len(newLines) == 0 {
			return UpdateFile{}, fmt.Errorf("empty update hunk for %s (hint: check for empty chunk that starts with @@ and has no content)", path)
		}

		chunks = append(chunks, UpdateChunk{
			ChangeContext: changeCtx,
			OldLines:      oldLines,
			NewLines:      newLines,
			IsEOF:         isEOF,
		})

		// If next line isn’t another @@, break outer loop
		if !strings.HasPrefix(p.peekLine(), "@@") {
			break
		}
	}

	return UpdateFile{Path: path, MovePath: movePath, Chunks: chunks}, nil
}

// ---------------- low-level helpers ----------------

func (p *parser) consumeLine() string { return p.consumeUntil('\n') }

// consumeUntil consumes bytes until delim (inclusive) or EOF and returns text
// before delim. If delim is missing, returns the remainder.
func (p *parser) consumeUntil(delim byte) string {
	cur := p.cursor
	start := cur.Pos
	for cur.Pos < cur.InputSize {
		if cur.Input[cur.Pos] == delim {
			txt := string(cur.Input[start:cur.Pos])
			cur.Pos++
			return txt
		}
		cur.Pos++
	}
	return string(cur.Input[start:])
}

func (p *parser) consumeRune() byte {
	cur := p.cursor
	ch := cur.Input[cur.Pos]
	cur.Pos++
	return ch
}

func (p *parser) peekLine() string {
	cur := p.cursor
	i := cur.Pos
	for i < cur.InputSize && cur.Input[i] != '\n' {
		i++
	}
	return string(cur.Input[cur.Pos:i])
}

func (p *parser) skipWhitespace() {
	cur := p.cursor
	for cur.Pos < cur.InputSize {
		switch cur.Input[cur.Pos] {
		case ' ', '\t', '\n', '\r', '\v', '\f':
			cur.Pos++
		default:
			return
		}
	}
}
