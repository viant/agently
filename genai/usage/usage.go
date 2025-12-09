package usage

import (
	"context"
	"sort"
	"sync"

	"github.com/viant/agently/genai/llm"
)

// Stat accumulates token numbers for a single model.
type Stat struct {
	PromptTokens     int
	CompletionTokens int
	EmbeddingTokens  int
	CachedTokens     int
}

// Aggregator collects usage grouped by model name.
type Aggregator struct {
	mux      sync.RWMutex
	PerModel map[string]*Stat
}

// OnUsage satisfies provider/base.UsageListener interface allowing Aggregator
// to be passed directly to provider clients. It records the supplied usage
// figures under the given model name.
func (a *Aggregator) OnUsage(model string, u *llm.Usage) {
	if u == nil {
		return
	}
	embed := u.TotalTokens - (u.PromptTokens + u.CompletionTokens)
	if embed < 0 {
		embed = 0
	}
	// Record cached tokens when provider reports them (e.g., OpenAI prompt_cached_tokens)
	a.Add(model, u.PromptTokens, u.CompletionTokens, embed, u.PromptCachedTokens)
}

func (a *Aggregator) ensure(model string) *Stat {
	a.mux.Lock()
	defer a.mux.Unlock()
	if a.PerModel == nil {
		a.PerModel = map[string]*Stat{}
	}
	s, ok := a.PerModel[model]
	if !ok {
		s = &Stat{}
		a.PerModel[model] = s
	}
	return s
}

// Add records token counts for a specific model.
// Add records token counts for a specific model. Cached tokens are optional –
// pass 0 when not applicable.
func (a *Aggregator) Add(model string, prompt, completion, embed, cached int) {
	stat := a.ensure(model)
	stat.PromptTokens += prompt
	stat.CompletionTokens += completion
	stat.EmbeddingTokens += embed
	stat.CachedTokens += cached
}

// Totals returns accumulated prompt, completion, embedding and cached tokens
// across all tracked models. It is primarily intended for tests and reporting.
func (a *Aggregator) Totals() (prompt, completion, embed, cached int) {
	a.mux.RLock()
	defer a.mux.RUnlock()
	for _, stat := range a.PerModel {
		prompt += stat.PromptTokens
		completion += stat.CompletionTokens
		embed += stat.EmbeddingTokens
		cached += stat.CachedTokens
	}
	return prompt, completion, embed, cached
}

// Keys returns sorted list of model names.
func (a *Aggregator) Keys() []string {
	a.mux.RLock()
	defer a.mux.RUnlock()
	keys := make([]string, 0, len(a.PerModel))
	for k := range a.PerModel {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// -- context helpers ---------------------------------------------------------

type keyT struct{}

var key = keyT{}

// WithAggregator injects Aggregator into context.
func WithAggregator(ctx context.Context) (context.Context, *Aggregator) {
	agg := &Aggregator{}
	return context.WithValue(ctx, key, agg), agg
}

func FromContext(ctx context.Context) *Aggregator {
	v := ctx.Value(key)
	if v == nil {
		return nil
	}
	if a, ok := v.(*Aggregator); ok {
		return a
	}
	return nil
}
