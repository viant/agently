package extractor

import (
	"bytes"
	"fmt"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	gmext "github.com/yuin/goldmark/extension"
	gmast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"strings"
)

// ParseLLMResponse configures Goldmark to parse tables, etc.
func ParseLLMResponse(source []byte) (*Section, error) {
	md := goldmark.New(
		goldmark.WithExtensions(gmext.GFM), // includes GFM table support
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
		goldmark.WithRendererOptions(html.WithHardWraps()),
	)

	reader := text.NewReader(source)
	doc := md.Parser().Parse(reader)

	// We'll convert the root document into a single top-level Section
	// with possible subSections.
	rootSection := parseSections(doc, source, 0)
	return &rootSection, nil
}

// parseSections iterates over the siblings in `parent` at a given `currentLevel`.
// It collects content/headings as a single Section. If we see a heading with
// level <= currentLevel (and currentLevel != 0), we end this section.
func parseSections(parent ast.Node, source []byte, currentLevel int) Section {
	section := Section{
		Metadata: &Metadata{},
		Content:  &Content{},
	}

	for n := parent.FirstChild(); n != nil; {
		next := n.NextSibling()

		switch node := n.(type) {
		case *ast.Heading:
			headingLevel := node.Level
			headingText := extractPlainText(node, source)

			// If heading level <= currentLevel and we're not at the root,
			// this ends the current section. Return so the parent can handle it.
			if headingLevel <= currentLevel && currentLevel != 0 {
				return section
			}

			// Build a sub-section for this heading
			subSec := Section{
				Metadata: &Metadata{
					Title: headingText,
				},
				Content: &Content{},
			}

			// Collect all paragraphs/lists/etc. that belong under this heading,
			// until the next heading of level <= headingLevel
			subSec, consumed := gatherSubsection(subSec, n, source)
			section.SubSections = append(section.SubSections, subSec)

			// Move `n` forward until we reach `consumed`
			for n != consumed && n != nil {
				n = n.NextSibling()
			}
			continue

		case *ast.Paragraph:
			paraText := strings.TrimSpace(extractPlainText(node, source))
			location := extractLocation(paraText)
			// If a location is found and the next node is a code block, attach the location.
			if location != "" && next != nil {
				if _, ok := next.(*ast.CodeBlock); ok || (func() bool {
					_, ok := next.(*ast.FencedCodeBlock)
					return ok
				}()) {
					cb := parseCodeBlock(next, source)
					cb.Location = location

					// Check if the paraText looks like a separator (contains "──────")
					if strings.Contains(paraText, "────") {
						// Create a paragraph with the separator text and a sub-section containing the code block
						ensureContent(&section)
						p := Paragraph{
							Text: paraText,
							SubNodes: []Section{
								{
									CodeBlocks: []CodeBlock{cb},
								},
							},
						}
						section.Content.Paragraphs = append(section.Content.Paragraphs, p)
					} else {
						// Add code block directly to the section
						section.CodeBlocks = append(section.CodeBlocks, cb)
					}
					// Skip both the file marker and the code block.
					n = next.NextSibling()
					continue
				}
			}

			ensureContent(&section)
			p := parseParagraph(node, source)
			section.Content.Paragraphs = append(section.Content.Paragraphs, p)

		case *ast.List:
			lst, err := parseList(node, source)
			if err == nil {
				ensureContent(&section)
				section.Content.Lists = append(section.Content.Lists, *lst)
			}

		case *ast.CodeBlock, *ast.FencedCodeBlock:
			cb := parseCodeBlock(node, source)
			section.CodeBlocks = append(section.CodeBlocks, cb)

		case *gmast.Table:
			tbl, err := parseTable(node, source)
			if err == nil {
				ensureContent(&section)
				section.Content.Tables = append(section.Content.Tables, *tbl)
			}

		default:
			// Ignore or recursively handle other node types as needed
		}

		n = next
	}

	return section
}

// extractLocation parses different location patterns from paragraph text
func extractLocation(paraText string) string {
	// Remove any leading dashes
	cleanedText := strings.TrimLeft(paraText, "-")
	cleanedText = strings.TrimSpace(cleanedText)

	lower := strings.ToLower(cleanedText)
	var location string

	if index := strings.Index(lower, "file:"); index != -1 {
		location = strings.TrimLeft(cleanedText[index+len("file:"):], " ")

		if index := strings.Index(location, "\n"); index != -1 {
			return location[:index]
		}
		if index := strings.Index(location, " "); index != -1 {
			return location[:index]
		}
		return strings.TrimSpace(location)
	}

	if strings.HasPrefix(lower, "file:") {
		location = strings.TrimSpace(cleanedText[len("file:"):])
	} else if strings.HasPrefix(cleanedText, "**File:**") {
		location = strings.TrimSpace(cleanedText[len("**File:**"):])
		// Remove wrapping backticks if any.
		location = strings.Trim(location, "`")
	} else if strings.Contains(lower, "file://") {
		start := strings.Index(lower, "file://")
		end := strings.Index(lower[start:], "\n")
		if end == -1 {
			end = len(lower)
		} else {
			end += start
		}
		location = strings.TrimSpace(cleanedText[start:end])
	}

	return location
}

func gatherSubsection(sec Section, headingNode ast.Node, source []byte) (Section, ast.Node) {
	h, ok := headingNode.(*ast.Heading)
	if !ok {
		return sec, headingNode
	}

	currentLevel := h.Level
	n := headingNode.NextSibling()
	for n != nil {
		if h2, isHeading := n.(*ast.Heading); isHeading {
			// If heading level is same or higher (numerically lower or equal), this section ends.
			if h2.Level <= currentLevel {
				return sec, n
			}
			// Otherwise, it's a deeper subsection.
			deeperSec := Section{
				Metadata: &Metadata{
					Title: extractPlainText(h2, source),
				},
				Content: &Content{},
			}
			var consumed ast.Node
			deeperSec, consumed = gatherSubsection(deeperSec, n, source)
			sec.SubSections = append(sec.SubSections, deeperSec)
			n = consumed
			continue
		}

		// Process non-heading nodes as part of current section content.
		switch node := n.(type) {
		case *ast.Paragraph:
			ensureContent(&sec)
			p := parseParagraph(node, source)
			sec.Content.Paragraphs = append(sec.Content.Paragraphs, p)
		case *ast.List:
			lst, err := parseList(node, source)
			if err == nil {
				ensureContent(&sec)
				sec.Content.Lists = append(sec.Content.Lists, *lst)
			}
		case *ast.CodeBlock, *ast.FencedCodeBlock:
			cb := parseCodeBlock(node, source)
			sec.CodeBlocks = append(sec.CodeBlocks, cb)
		case *gmast.Table:
			tbl, err := parseTable(node, source)
			if err == nil {
				ensureContent(&sec)
				sec.Content.Tables = append(sec.Content.Tables, *tbl)
			}
		}
		n = n.NextSibling()
	}

	return sec, nil
}

// ensureContent initializes the Content field if it's nil
func ensureContent(section *Section) {
	if section.Content == nil {
		section.Content = &Content{}
	}
}

// -----------------------------------------------------------------------------
// PARAGRAPH & LINK PARSING
// -----------------------------------------------------------------------------
func parseParagraph(parNode *ast.Paragraph, source []byte) Paragraph {
	p := Paragraph{}

	// We'll track if we are "inside a link" so we don't re-append child text.
	var insideLink bool

	ast.Walk(parNode, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			switch typed := n.(type) {
			case *ast.Link:
				insideLink = true
				// Extract the link text from its children
				linkText := extractPlainText(typed, source)
				// Append to paragraph text just once
				p.Text += linkText
				// Add to paragraph's list of links
				p.Links = append(p.Links, Link{
					Text: linkText,
					URL:  string(typed.Destination),
				})
				// **Skip** walking children so we don't see ast.Text again
				return ast.WalkSkipChildren, nil

			case *ast.Text:
				// Add text only if not inside a link
				if !insideLink {
					p.Text += string(typed.Segment.Value(source))
				}
			}
		} else {
			if _, isLink := n.(*ast.Link); isLink {
				insideLink = false
			}
		}
		return ast.WalkContinue, nil
	})

	return p
}

// extractPlainText is a helper that finds only text nodes under `n` and
// concatenates them. It's used for heading text, link text, etc.
func extractPlainText(n ast.Node, source []byte) string {
	var buf bytes.Buffer
	ast.Walk(n, func(child ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			if t, ok := child.(*ast.Text); ok {
				buf.Write(t.Segment.Value(source))
			}
		}
		return ast.WalkContinue, nil
	})
	return buf.String()
}

// -----------------------------------------------------------------------------
// LIST, CODE BLOCK, TABLE PARSERS
// -----------------------------------------------------------------------------

// parseList calls gatherListItemText(...) for each item.
func parseList(n ast.Node, source []byte) (*List, error) {
	listNode, ok := n.(*ast.List)
	if !ok {
		return nil, fmt.Errorf("node is not a *ast.List")
	}

	var listType ListType
	if listNode.IsOrdered() {
		listType = OrderedList
	} else {
		listType = BulletList
	}

	var items []ListItem
	for li := listNode.FirstChild(); li != nil; li = li.NextSibling() {
		itemText := gatherListItemText(li, source)
		items = append(items, ListItem{Text: itemText})
	}

	return &List{
		Type:  listType,
		Items: items,
	}, nil
}

func gatherListItemText(node ast.Node, source []byte) string {
	var buf bytes.Buffer

	// A small inline traversal to capture text (including checkboxes)
	ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			switch typed := n.(type) {
			case *gmast.TaskCheckBox:
				// Reconstruct the literal bracket text
				if typed.IsChecked {
					buf.WriteString("[x] ")
				} else {
					buf.WriteString("[ ] ")
				}

			case *ast.Text:
				buf.Write(typed.Segment.Value(source))
			}
		}
		return ast.WalkContinue, nil
	})

	return buf.String()
}

func parseCodeBlock(n ast.Node, source []byte) CodeBlock {
	var code, language string

	switch node := n.(type) {
	case *ast.CodeBlock:
		code = string(node.Text(source))
	case *ast.FencedCodeBlock:
		code = string(node.Text(source))
		language = string(node.Language(source))
	}
	return CodeBlock{
		Language: language,
		Content:  code,
	}
}

func parseTable(n ast.Node, source []byte) (*Table, error) {
	tableNode, ok := n.(*gmast.Table)
	if !ok {
		return nil, fmt.Errorf("node is not a *gmast.Table")
	}

	var headers []string
	var rows []Row

	// First child is typically TableHeader
	if header := tableNode.FirstChild(); header != nil {
		if headerRow, ok := header.(*gmast.TableHeader); ok {
			for cell := headerRow.FirstChild(); cell != nil; cell = cell.NextSibling() {
				headers = append(headers, extractPlainText(cell, source))
			}
		}
	}

	// Then we have TableBody with TableRow children
	for body := tableNode.FirstChild().NextSibling(); body != nil; body = body.NextSibling() {
		if bodyRow, ok := body.(*gmast.TableRow); ok {
			var cells []string
			for cell := bodyRow.FirstChild(); cell != nil; cell = cell.NextSibling() {
				cells = append(cells, extractPlainText(cell, source))
			}
			rows = append(rows, Row{Cells: cells})
		}
	}

	return &Table{
		Headers: headers,
		Rows:    rows,
	}, nil
}
