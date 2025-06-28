package memory

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUsageStore_AddAndTotals(t *testing.T) {
	store := NewUsageStore()

	// conv1 – two updates for same model, plus one embedding-only model
	store.Add("conv1", "gpt-3.5", 100, 50, 20, 0)
	store.Add("conv1", "gpt-3.5", 30, 10, 0, 0)
	store.Add("conv1", "ada-002", 0, 0, 100, 0)

	p, c, e, cached := store.Totals("conv1")
	assert.EqualValues(t, 130, p)
	assert.EqualValues(t, 60, c)
	assert.EqualValues(t, 120, e)
	assert.EqualValues(t, 0, cached)

	agg := store.Aggregator("conv1")
	assert.NotNil(t, agg)
	assert.ElementsMatch(t, []string{"ada-002", "gpt-3.5"}, agg.Keys())

	// Unknown conversation – expect zeros.
	p, c, e, cached = store.Totals("unknown")
	assert.EqualValues(t, 0, p)
	assert.EqualValues(t, 0, c)
	assert.EqualValues(t, 0, e)
	assert.EqualValues(t, 0, cached)
}
