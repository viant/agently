package extractor_test

import (
	"encoding/json"
	extractor2 "github.com/viant/agently/genai/io/extractor"
	"testing"
)

// TestParseLLMResponse is a data-driven test that checks how various Markdown
// constructs (headings, paragraphs, lists, code blocks, tables, etc.) are
// parsed into the extractor data structures.
func TestParseLLMResponse(t *testing.T) {
	tests := []struct {
		name    string
		inputMD string
		want    extractor2.Section
		wantErr bool
	}{
		{
			name:    "Code separated",
			inputMD: "──────────────────────────────\nfile: /Users/awitas/go/src/github.com/viant/agently/service/extractor/parser_test.go\n```go\npackage extractor_test\n\nimport (\n\t\"encoding/json\"\n\t\"testing\"\n\n\t\"github.com/viant/agently/service/extractor\"\n)\n```\n",
			want: extractor2.Section{
				Metadata: &extractor2.Metadata{},
				Content: &extractor2.Content{
					Paragraphs: []extractor2.Paragraph{{
						Text: "──────────────────────────────file: /Users/awitas/go/src/github.com/viant/agently/service/extractor/parser_test.go",
						SubNodes: []extractor2.Section{{
							CodeBlocks: []extractor2.CodeBlock{
								{
									Language: "go",
									Location: "/Users/awitas/go/src/github.com/viant/agently/service/extractor/parser_test.go",
									Content:  "package extractor_test\n\nimport (\n\t\"encoding/json\"\n\t\"testing\"\n\n\t\"github.com/viant/agently/service/extractor\"\n)\n",
								},
							},
						}},
					}},
				},
			},
		},
		{
			name:    "Single paragraph (no heading)",
			inputMD: `This is a single paragraph with **bold** text.`,
			want: extractor2.Section{
				Metadata: &extractor2.Metadata{},
				Content: &extractor2.Content{
					Paragraphs: []extractor2.Paragraph{
						{
							Text: "This is a single paragraph with bold text.",
							// There's no explicit link in this text, so Links is empty
							Links: nil,
						},
					},
				},
			},
		},
		{
			name: "Heading + paragraph",
			inputMD: `# Heading Level 1

This is a paragraph under heading 1.`,
			want: extractor2.Section{
				Metadata: &extractor2.Metadata{},
				Content:  &extractor2.Content{},
				// Because there's a heading, it becomes a SubSection
				SubSections: []extractor2.Section{
					{
						Metadata: &extractor2.Metadata{
							Title: "Heading Level 1",
						},
						Content: &extractor2.Content{
							Paragraphs: []extractor2.Paragraph{
								{
									Text: "This is a paragraph under heading 1.",
								},
							},
						},
					},
				},
			},
		},
		{
			name:    "Paragraph with link",
			inputMD: `Here is some text with a [link](https://example.com).`,
			want: extractor2.Section{
				Metadata: &extractor2.Metadata{},
				Content: &extractor2.Content{
					Paragraphs: []extractor2.Paragraph{
						{
							Text: "Here is some text with a link.",
							Links: []extractor2.Link{
								{
									Text: "link",
									URL:  "https://example.com",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Unordered list",
			inputMD: `- Item A
- Item B
- Item C`,
			want: extractor2.Section{
				Metadata: &extractor2.Metadata{},
				Content: &extractor2.Content{
					Lists: []extractor2.List{
						{
							Type: extractor2.BulletList,
							Items: []extractor2.ListItem{
								{Text: "Item A"},
								{Text: "Item B"},
								{Text: "Item C"},
							},
						},
					},
				},
			},
		},
		{
			name: "Ordered list",
			inputMD: `1. First
2. Second
3. Third`,
			want: extractor2.Section{
				Metadata: &extractor2.Metadata{},
				Content: &extractor2.Content{
					Lists: []extractor2.List{
						{
							Type: extractor2.OrderedList,
							Items: []extractor2.ListItem{
								{Text: "First"},
								{Text: "Second"},
								{Text: "Third"},
							},
						},
					},
				},
			},
		},
		{
			name: "Checklist-like list (not standard GFM, but possible extension)",
			inputMD: `- [x] Done item
- [ ] Pending item`,
			want: extractor2.Section{
				Metadata: &extractor2.Metadata{},
				Content: &extractor2.Content{
					// The parser does not differentiate GFM checklists from bullet lists
					// unless you customize it. By default, these become bullet items:
					Lists: []extractor2.List{
						{
							Type: extractor2.BulletList,
							Items: []extractor2.ListItem{
								{Text: "[x] Done item"},
								{Text: "[ ] Pending item"},
							},
						},
					},
				},
			},
		},

		{
			name:    "Code block (fenced)",
			inputMD: "file:main.js\n```go\nfmt.Println(\"Hello, world!\")\n```",
			want: extractor2.Section{
				Metadata: &extractor2.Metadata{},
				Content:  &extractor2.Content{},
				CodeBlocks: []extractor2.CodeBlock{
					{
						Language: "go",
						Location: "main.js",
						Content:  "fmt.Println(\"Hello, world!\")\n",
					},
				},
			},
		},
		{
			name: "Markdown table",
			inputMD: `| Column A | Column B |
|----------|---------|
| A1       | B1      |
| A2       | B2      |`,
			want: extractor2.Section{
				Metadata: &extractor2.Metadata{},
				Content: &extractor2.Content{
					Tables: []extractor2.Table{
						{
							Headers: []string{"Column A", "Column B"},
							Rows: []extractor2.Row{
								{Cells: []string{"A1", "B1"}},
								{Cells: []string{"A2", "B2"}},
							},
						},
					},
				},
			},
		},
		{
			name: "Nested headings (subsections)",
			inputMD: `# Top Heading
Some paragraph under top heading

## Subheading
Sub-paragraph A

## Another Sub
Sub-paragraph B

# Another Top
Another top-level paragraph`,
			want: extractor2.Section{
				Metadata: &extractor2.Metadata{},
				Content:  &extractor2.Content{},
				SubSections: []extractor2.Section{
					{
						Metadata: &extractor2.Metadata{
							Title: "Top Heading",
						},
						Content: &extractor2.Content{
							Paragraphs: []extractor2.Paragraph{
								{Text: "Some paragraph under top heading"},
							},
						},
						SubSections: []extractor2.Section{
							{
								Metadata: &extractor2.Metadata{
									Title: "Subheading",
								},
								Content: &extractor2.Content{
									Paragraphs: []extractor2.Paragraph{
										{Text: "Sub-paragraph A"},
									},
								},
							},
							{
								Metadata: &extractor2.Metadata{
									Title: "Another Sub",
								},
								Content: &extractor2.Content{
									Paragraphs: []extractor2.Paragraph{
										{Text: "Sub-paragraph B"},
									},
								},
							},
						},
					},
					{
						Metadata: &extractor2.Metadata{
							Title: "Another Top",
						},
						Content: &extractor2.Content{
							Paragraphs: []extractor2.Paragraph{
								{Text: "Another top-level paragraph"},
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractor2.ParseLLMResponse([]byte(tt.inputMD))
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseLLMResponse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			gotJSON, _ := json.Marshal(got)
			wantJSON, _ := json.Marshal(&tt.want)
			if string(gotJSON) != string(wantJSON) {
				t.Errorf("ParseLLMResponse() mismatch.\nGot:  %s\nWant: %s", gotJSON, wantJSON)
			}
		})
	}
}
