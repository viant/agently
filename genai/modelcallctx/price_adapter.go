package modelcallctx

// fixedModelPriceProvider wraps a TokenPriceProvider and always resolves
// prices using a fixed, declared model id/name instead of the provider-reported one.
type fixedModelPriceProvider struct {
	base  TokenPriceProvider
	model string
}

// NewFixedModelPriceProvider creates a provider that ignores the lookup key
// and always returns prices for the declared model.
func NewFixedModelPriceProvider(base TokenPriceProvider, declared string) TokenPriceProvider {
	return fixedModelPriceProvider{base: base, model: declared}
}

func (f fixedModelPriceProvider) TokenPrices(_ string) (float64, float64, float64, bool) {
	if f.base == nil || f.model == "" {
		return 0, 0, 0, false
	}
	return f.base.TokenPrices(f.model)
}
