package base

import "github.com/viant/agently/genai/llm"

// UsageListener is a callback used by provider clients to report token usage
// for each successful request.  It is declared as a function type so users can
// pass simple lambdas (e.g. `WithUsageListener(func(model string, usage *llm.Usage){…})`).
//
// To remain compatible with existing provider code that expects the value to
// expose an `OnUsage` method, we attach such method directly on the function
// type.  Therefore:
//   listener := func(model string, usage *llm.Usage) { … }
//   // satisfies usageListener because the method below adapts it.
//   var _ UsageListener = UsageListener(listener)
//
// A struct can also implement its own `OnUsage` method and be converted to
// UsageListener by using its method value, e.g. `myAggregator.OnUsage`.
type UsageListener func(model string, usage *llm.Usage)

// OnUsage makes the function compatible with the method-based invocation used
// across provider implementations.
func (f UsageListener) OnUsage(model string, usage *llm.Usage) {
    if f == nil {
        return
    }
    f(model, usage)
}
