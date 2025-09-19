package conversation

import (
	"bytes"
	"compress/gzip"
	"context"
)

func (p *ResponsePayloadView) OnFetch(ctx context.Context) error {
	if p.InlineBody == nil {
		return nil
	}
	inline := []byte(*p.InlineBody)
	uncompressIfNeeded(&p.Compression, &inline)
	*p.InlineBody = string(bytes.TrimSpace(inline))
	return nil
}

func uncompressIfNeeded(compression *string, inlineBody *[]byte) {
	if *compression == "gzip" && inlineBody != nil {
		gr, err := gzip.NewReader(bytes.NewReader([]byte(*inlineBody)))
		if err == nil {
			var buf bytes.Buffer
			_, _ = buf.ReadFrom(gr)
			_ = gr.Close()
			b := buf.Bytes()
			*inlineBody = b
			*compression = ""
		}
	}
}

func (p *AttachmentView) OnFetch(ctx context.Context) error {
	uncompressIfNeeded(&p.Compression, p.InlineBody)

	return nil
}
