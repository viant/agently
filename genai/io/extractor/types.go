package extractor

import (
	"fmt"
	"strings"
	"time"
)

type Section struct {
	Metadata    *Metadata   `json:"metadata,omitempty"`
	Content     *Content    `json:"content,omitempty"`
	SubSections []Section   `json:"subSections,omitempty"`
	CodeBlocks  []CodeBlock `json:"codeBlocks,omitempty"`
}

func (s *Section) Markup() string {
	var builder strings.Builder

	builder.WriteString("### Section\n")

	// Metadata
	if s.Metadata != nil {
		builder.WriteString("#### Metadata\n")
		builder.WriteString(fmt.Sprintf("- **Title**: %s\n", s.Metadata.Title))
		if s.Metadata.Description != "" {
			builder.WriteString(fmt.Sprintf("- **Description**: %s\n", s.Metadata.Description))
		}
		if len(s.Metadata.Tags) > 0 {
			builder.WriteString(fmt.Sprintf("- **Tags**: %v\n", s.Metadata.Tags))
		}
		builder.WriteString("\n")
	}

	// Result
	if s.Content != nil {
		builder.WriteString("#### Result\n")
		for _, para := range s.Content.Paragraphs {
			builder.WriteString(fmt.Sprintf("- %s\n", para.Text))
		}
		builder.WriteString("\n")
	}

	// Code Blocks
	if len(s.CodeBlocks) > 0 {
		builder.WriteString("#### Code Blocks\n")
		for _, cb := range s.CodeBlocks {
			builder.WriteString(fmt.Sprintf("```%s\n%s\n```\n", cb.Language, cb.Content))
		}
		builder.WriteString("\n")
	}

	// SubSections
	if len(s.SubSections) > 0 {
		builder.WriteString("#### SubSections\n")
		for _, sub := range s.SubSections {
			builder.WriteString(sub.Markup())
			builder.WriteString("\n")
		}
	}
	return builder.String()
}

func (s *Section) String() string {
	var builder strings.Builder

	if s.Metadata != nil {
		builder.WriteString(fmt.Sprintf("Title: %s\n", s.Metadata.Title))
		if s.Metadata.Description != "" {
			builder.WriteString(fmt.Sprintf("Description: %s\n", s.Metadata.Description))
		}
		if len(s.Metadata.Tags) > 0 {
			builder.WriteString(fmt.Sprintf("Tags: %v\n", s.Metadata.Tags))
		}
	}

	if s.Content != nil {
		builder.WriteString("Result:\n")
		for _, para := range s.Content.Paragraphs {
			builder.WriteString(fmt.Sprintf("  - %s\n", para.Text))
		}
	}

	if len(s.CodeBlocks) > 0 {
		builder.WriteString("CodeBlocks:\n")
		for _, cb := range s.CodeBlocks {
			builder.WriteString(fmt.Sprintf("  - [%s] %s\n", cb.Language, cb.Content))
		}
	}

	if len(s.SubSections) > 0 {
		builder.WriteString("SubSections:\n")
		for _, sub := range s.SubSections {
			builder.WriteString(sub.String())
			builder.WriteString("\n")
		}
	}

	return builder.String()
}

func (s *Section) Match(text string) string {
	lcText := strings.ReplaceAll(strings.ToLower(text), " ", "")
	if s.Content != nil {
		for _, para := range s.Content.Paragraphs {
			if strings.Contains(strings.ReplaceAll(strings.ToLower(para.Text), " ", ""), lcText) {
				var builder strings.Builder
				for _, list := range s.Content.Lists {
					builder.WriteString(list.Markup())
				}
				for _, block := range s.Content.CodeBlocks {
					builder.WriteString(block.Content)
				}
				for _, subNode := range para.SubNodes {
					builder.WriteString(subNode.Markup())
				}
				return builder.String()
			}
		}
	}
	for _, section := range s.SubSections {
		if match := section.Match(text); match != "" {
			return match
		}
	}
	return ""
}

// CollectCodeBlocks is a method on Section that recursively traverses itself and nested sections to collect all CodeBlocks.
func (s Section) CollectCodeBlocks() []CodeBlock {
	var blocks []CodeBlock

	// Append code blocks defined directly in this Section.
	blocks = append(blocks, s.CodeBlocks...)

	// If the section has Result, append its CodeBlocks.
	if s.Content != nil {
		blocks = append(blocks, s.Content.CodeBlocks...)

		// Traverse paragraphs to collect code blocks from nested sections.
		for _, para := range s.Content.Paragraphs {
			for _, subSection := range para.SubNodes {
				blocks = append(blocks, subSection.CollectCodeBlocks()...)
			}
		}
	}

	// Recursively traverse nested sub-sections.
	for _, subSection := range s.SubSections {
		blocks = append(blocks, subSection.CollectCodeBlocks()...)
	}

	return blocks
}

type Metadata struct {
	Title       string            `json:"title"`
	Description string            `json:"description,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	Attributes  map[string]string `json:"attributes,omitempty"`
	CreatedAt   time.Time         `json:"createdAt,omitempty"`
}

type Content struct {
	Paragraphs []Paragraph `json:"paragraphs,omitempty"`
	Lists      []List      `json:"lists,omitempty"`
	Tables     []Table     `json:"tables,omitempty"`
	CodeBlocks []CodeBlock `json:"codeBlocks,omitempty"`
	Media      []Media     `json:"media,omitempty"`
}

// Paragraph now contains a slice of Links so we don't lose `[text](Paths)`.
type Paragraph struct {
	Text     string    `json:"text"`
	Links    []Link    `json:"links,omitempty"`
	SubNodes []Section `json:"subNodes,omitempty"`
}

// Link holds the linked text and the URL.
type Link struct {
	Text string `json:"text"`
	URL  string `json:"url"`
}

type List struct {
	Type  ListType   `json:"type"`
	Items []ListItem `json:"items"`
}

func (l *List) bulletListMarkup() string {
	builder := strings.Builder{}
	for _, item := range l.Items {
		builder.WriteString("- " + item.Text + "\n")
	}
	return builder.String()
}

func (l *List) orderedListMarkup() string {
	builder := strings.Builder{}
	for i, item := range l.Items {
		builder.WriteString(fmt.Sprintf("%d. %s\n", i+1, item.Text))
	}
	return builder.String()
}

func (l *List) checklistListMarkup() string {
	builder := strings.Builder{}
	for _, item := range l.Items {
		checkbox := "[ ]"
		if len(item.Metadata) > 0 && item.Metadata["checked"] == "true" {
			checkbox = "[x]"
		}
		builder.WriteString(fmt.Sprintf("%s %s\n", checkbox, item.Text))
	}
	return builder.String()
}

func (l *List) Markup() string {
	switch l.Type {
	case BulletList:
		return l.bulletListMarkup()
	case OrderedList:
		return l.orderedListMarkup()
	case ChecklistList:
		return l.checklistListMarkup()
	default:
		return ""
	}
}

type ListType string

const (
	BulletList    ListType = "bullet"
	OrderedList   ListType = "ordered"
	ChecklistList ListType = "checklist"
)

type ListItem struct {
	Text       string            `json:"text"`
	SubItems   []ListItem        `json:"subItems,omitempty"`
	References []string          `json:"references,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

type Table struct {
	Headers []string `json:"headers"`
	Rows    []Row    `json:"rows"`
}

type Row struct {
	Cells    []string          `json:"cells"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type CodeBlock struct {
	Language string            `json:"language"`
	Location string            `json:"location,omitempty"`
	Project  string            `json:"project,omitempty"`
	Content  string            `json:"content"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type Media struct {
	Type     MediaType         `json:"type"`
	URL      string            `json:"url"`
	AltText  string            `json:"altText,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type MediaType string

const (
	ImageMedia MediaType = "image"
	VideoMedia MediaType = "video"
	AudioMedia MediaType = "audio"
)
