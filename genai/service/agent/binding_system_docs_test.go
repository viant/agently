package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/prompt"
	executil "github.com/viant/agently/genai/service/shared/executil"
	agconv "github.com/viant/agently/pkg/agently/conversation"
)

func TestTranscriptSystemDocuments(t *testing.T) {
	content := "file: workspace://localhost/doc.md\n```md\ncontent\n````"
	msg := &apiconv.Message{
		Id:             "msg-1",
		Role:           "system",
		Content:        strPtr(content),
		Tags:           strPtr(executil.SystemDocumentTag),
		ContextSummary: strPtr("workspace://localhost/doc.md"),
	}
	dup := &apiconv.Message{
		Id:      "msg-dup",
		Role:    "system",
		Content: strPtr(content),
	}
	turnDup := &apiconv.Turn{Id: "turn-1", Message: []*agconv.MessageView{(*agconv.MessageView)(msg), (*agconv.MessageView)(dup)}}
	docs := transcriptSystemDocuments(apiconv.Transcript{turnDup})

	require.Len(t, docs, 1)
	assert.Equal(t, "workspace://localhost/doc.md", docs[0].SourceURI)
	assert.Equal(t, "doc.md", docs[0].Title)
	assert.Equal(t, "msg-1", docs[0].Metadata["messageId"])
	assert.Equal(t, "turn-1", docs[0].Metadata["turnId"])

	// Ensure appendTranscriptSystemDocs dedupes on source URI
	b := &prompt.Binding{}
	svc := &Service{}
	svc.appendTranscriptSystemDocs(apiconv.Transcript{turnDup}, b)
	svc.appendTranscriptSystemDocs(apiconv.Transcript{turnDup}, b)
	require.Len(t, b.SystemDocuments.Items, 1)
}

func TestExtractSystemDocSourceFallback(t *testing.T) {
	content := "FILE: workspace://fallback/doc.txt\nbody"
	msg := &apiconv.Message{
		Id:      "msg-2",
		Role:    "system",
		Content: strPtr(content),
		Tags:    strPtr(executil.SystemDocumentTag),
	}
	turn := &apiconv.Turn{Id: "turn-2", Message: []*agconv.MessageView{(*agconv.MessageView)(msg)}}
	docs := transcriptSystemDocuments(apiconv.Transcript{turn})
	require.Len(t, docs, 1)
	assert.Equal(t, "workspace://fallback/doc.txt", docs[0].SourceURI)
	assert.Equal(t, "doc.txt", docs[0].Title)
}

func strPtr(val string) *string {
	return &val
}
