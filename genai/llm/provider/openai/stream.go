package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/viant/agently/genai/llm"
	mcbuf "github.com/viant/agently/genai/modelcallctx"
	"strings"
	"time"
)

type streamState struct {
	lastUsage      *llm.Usage
	lastModel      string
	lastLR         *llm.GenerateResponse
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
		p.events <- llm.StreamEvent{Err: fmt.Errorf("failed to unmarshal stream response: %w", err)}
		return false
	}
	lr := ToLLMSResponse(&apiResp)

	if lr != nil && lr.Usage != nil && lr.Usage.TotalTokens > 0 {
		if p.state.lastModel == "" && lr.Model != "" {
			p.state.lastModel = lr.Model
		}
		p.state.lastUsage = lr.Usage
	}
	p.client.publishUsageOnce(p.state.lastModel, p.state.lastUsage, &p.state.publishedUsage)
	endObserverOnce(p.observer, p.ctx, p.state.lastModel, lr, p.state.lastUsage, &p.state.ended)
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
			if b, err := json.Marshal(p.state.lastLR); err == nil {
				respJSON = b
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
		p.observer.OnCallEnd(p.ctx, mcbuf.Info{Provider: "openai", Model: p.state.lastModel, ModelKind: "chat", ResponseJSON: respJSON, CompletedAt: time.Now(), Usage: usage, FinishReason: finishReason, LLMResponse: p.state.lastLR, StreamText: streamTxt})
	}
}
