package adapter

import (
	"path"
	"sort"
	"strings"

	"github.com/tmc/langchaingo/schema"
	"github.com/viant/afs/asset"
	"github.com/viant/agently/genai/prompt"
)

// FromSchemaDocs converts search documents into prompt.Document items.
func FromSchemaDocs(docs []schema.Document) []*prompt.Document {
	out := make([]*prompt.Document, 0, len(docs))

	// Ensure deterministic processing order: sort knowledge by URL
	sort.SliceStable(docs, func(i, j int) bool {
		sourceI := extractSource(docs[i].Metadata)
		sourceJ := extractSource(docs[j].Metadata)
		return sourceI < sourceJ
	})

	for _, d := range docs {
		source := extractSource(d.Metadata)
		title := baseName(source)
		out = append(out, &prompt.Document{Title: title, PageContent: d.PageContent, SourceURI: source})
	}
	return out
}

// FromAssets converts file resources into prompt.Document items.
func FromAssets(resources []*asset.Resource) []*prompt.Document {
	out := make([]*prompt.Document, 0, len(resources))
	for _, r := range resources {
		if r == nil {
			continue
		}
		title := baseName(r.Name)
		out = append(out, &prompt.Document{Title: title, PageContent: string(r.Data), SourceURI: r.Name})
	}
	return out
}

func baseName(uri string) string {
	if uri == "" {
		return ""
	}
	if b := path.Base(uri); b != "." && b != "/" {
		return b
	}
	return uri
}

func extractSource(meta map[string]any) string {
	if meta == nil {
		return ""
	}
	if v, ok := meta["path"]; ok {
		if s, _ := v.(string); strings.TrimSpace(s) != "" {
			return s
		}
	}
	if v, ok := meta["docId"]; ok {
		if s, _ := v.(string); strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}
