package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/viant/agently/genai/llm"
	mcbuf "github.com/viant/agently/genai/modelcallctx"
)

type streamState struct {
	lastUsage      *llm.Usage
	lastModel      string
	lastLR         *llm.GenerateResponse
	lastProvider   []byte
	ended          bool
	publishedUsage bool
}

type streamProcessor struct {
	client   *Client
	ctx      context.Context
	observer mcbuf.Observer
	events   chan<- llm.StreamEvent
	agg      *streamAggregator
	state    *streamState
	respBody []byte
}

func (p *streamProcessor) handleData(data string) bool {
	// Persist full raw SSE line for complete stream fidelity (JSON chunk as-is).
	if p.observer != nil && strings.TrimSpace(data) != "" && data != "[DONE]" {
		// Append newline to maintain readable separation between chunks
		p.observer.OnStreamDelta(p.ctx, []byte(data+"\n"))
	}
	// 1) Aggregated delta chunk
	var sresp StreamResponse
	if err := json.Unmarshal([]byte(data), &sresp); err == nil && len(sresp.Choices) > 0 {
		if sresp.Model != "" {
			p.state.lastModel = sresp.Model
		}
		finalized := make([]llm.Choice, 0)
		for _, ch := range sresp.Choices {
			p.agg.updateDelta(ch)
			// Emit text stream delta to observer when content arrives
			if ch.Delta.Content != nil {
				if txt := strings.TrimSpace(*ch.Delta.Content); txt != "" && p.observer != nil {
					p.observer.OnStreamDelta(p.ctx, []byte(txt))
				}
			}
			if ch.FinishReason != nil {
				finalized = append(finalized, p.agg.finalizeChoice(ch.Index, *ch.FinishReason))
			}
		}
		if len(finalized) > 0 {
			lr := &llm.GenerateResponse{Choices: finalized, Model: p.state.lastModel}
			if p.state.lastUsage != nil && p.state.lastUsage.TotalTokens > 0 {
				lr.Usage = p.state.lastUsage
			}
			p.client.publishUsageOnce(p.state.lastModel, p.state.lastUsage, &p.state.publishedUsage)
			emitResponse(p.events, lr)
			p.state.lastLR = lr
		}
		return true
	}

	// 2) Final response object
	var apiResp Response
	if err := json.Unmarshal([]byte(data), &apiResp); err != nil {
		// Tolerate non-standard or intermediary payloads that are not valid
		// JSON responses (e.g., provider diagnostics). Ignore and continue
		// scanning rather than failing the whole stream.
		return true
	}
	lr := ToLLMSResponse(&apiResp)
	// Keep a snapshot of the last provider-level response object for persistence
	if b, err := json.Marshal(apiResp); err == nil {
		p.state.lastProvider = b
	}

	// If this is a usage-only final chunk (OpenAI streams often end with
	// an object whose choices == [] but usage is populated), do NOT emit an
	// empty-choices response. Capture usage and model, but leave final message
	// emission to previously finalized choices or to finalize().
	if lr != nil && len(lr.Choices) == 0 {
		if lr.Usage != nil && lr.Usage.TotalTokens > 0 {
			if p.state.lastModel == "" && lr.Model != "" {
				p.state.lastModel = lr.Model
			}
			p.state.lastUsage = lr.Usage
			p.client.publishUsageOnce(p.state.lastModel, p.state.lastUsage, &p.state.publishedUsage)
			// Re-emit the last aggregated response with usage attached, if available
			if p.state.lastLR != nil {
				// clone shallow and attach usage
				updated := *p.state.lastLR
				updated.Usage = lr.Usage
				updated.Model = p.state.lastModel
				emitResponse(p.events, &updated)
				p.state.lastLR = &updated
			}
		}
		// Do not end observer here; finalize() will notify with accumulated text
		return true
	}

	if lr != nil && lr.Usage != nil && lr.Usage.TotalTokens > 0 {
		if p.state.lastModel == "" && lr.Model != "" {
			p.state.lastModel = lr.Model
		}
		p.state.lastUsage = lr.Usage
	}
	p.client.publishUsageOnce(p.state.lastModel, p.state.lastUsage, &p.state.publishedUsage)
	if err := endObserverOnce(p.observer, p.ctx, p.state.lastModel, lr, p.state.lastUsage, &p.state.ended); err != nil {
		p.events <- llm.StreamEvent{Err: fmt.Errorf("observer OnCallEnd failed: %w", err)}
	}
	emitResponse(p.events, lr)
	p.state.lastLR = lr
	return true
}

func (p *streamProcessor) finalize(scannerErr error) {
	if scannerErr != nil {
		p.events <- llm.StreamEvent{Err: fmt.Errorf("stream read error: %w", scannerErr)}
	}

	if p.observer != nil && !p.state.ended {
		var usage *llm.Usage
		if p.state.lastUsage != nil {
			usage = p.state.lastUsage
		}
		var respJSON []byte
		var finishReason string
		if p.state.lastLR != nil {
			// Prefer provider final object snapshot when available; fallback to SSE body
			if len(p.state.lastProvider) > 0 {
				respJSON = p.state.lastProvider
			} else {
				respJSON = p.respBody
			}
			if len(p.state.lastLR.Choices) > 0 {
				finishReason = p.state.lastLR.Choices[0].FinishReason
			}
		} else {
			respJSON = p.respBody
		}
		// Extract plain text content (vanilla stream) for persistence convenience
		var streamTxt string
		if p.state.lastLR != nil {
			for _, ch := range p.state.lastLR.Choices {
				if strings.TrimSpace(ch.Message.Content) != "" {
					streamTxt = strings.TrimSpace(ch.Message.Content)
					break
				}
			}
		}
		if err := p.observer.OnCallEnd(p.ctx, mcbuf.Info{Provider: "openai", Model: p.state.lastModel, ModelKind: "chat", ResponseJSON: respJSON, CompletedAt: time.Now(), Usage: usage, FinishReason: finishReason, LLMResponse: p.state.lastLR, StreamText: streamTxt}); err != nil {
			p.events <- llm.StreamEvent{Err: fmt.Errorf("observer OnCallEnd failed: %w", err)}
		}
	}
}
