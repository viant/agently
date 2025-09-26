package augmenter

import (
	"github.com/tmc/langchaingo/schema"
	"github.com/viant/linager/inspector/graph"
)

type Documents []schema.Document

type Document schema.Document

func (d Document) Size() int {
	size := len(d.PageContent)
	for k, v := range d.Metadata {
		if text, ok := v.(string); ok { //estimate, meta may not be used in
			size += len(k) + len(text)
		}
	}
	return size
}

func (d Documents) Size() int {
	size := 0
	for _, doc := range d {
		size += Document(doc).Size()
	}
	return size
}
func (d Documents) ProjectDocuments() graph.Documents {
	var results []*graph.Document
	for _, doc := range d {
		document := Document(doc)
		results = append(results, document.ProjectDocument())
	}
	return results
}

func (d Document) ProjectDocument() *graph.Document {
	return &graph.Document{
		Name:      getStringFromMetadata(d.Metadata, "name"),
		Path:      getStringFromMetadata(d.Metadata, "path"),
		Package:   getStringFromMetadata(d.Metadata, "package"),
		Kind:      graph.DocumentKind(getStringFromMetadata(d.Metadata, "kind")),
		Signature: getStringFromMetadata(d.Metadata, "signature"),
		Content:   d.PageContent,
	}
}
