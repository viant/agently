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
	a.Add(model, u.PromptTokens, u.CompletionTokens, embed)
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
func (a *Aggregator) Add(model string, prompt, completion, embed int) {
	stat := a.ensure(model)
	stat.PromptTokens += prompt
	stat.CompletionTokens += completion
	stat.EmbeddingTokens += embed
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
