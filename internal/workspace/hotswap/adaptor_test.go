package hotswap

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestAdaptor verifies that generic adaptor invokes callbacks in the expected
// way for each Action.
func TestAdaptor(t *testing.T) {
	ctx := context.Background()

	// Stub storage -----------------------------------------------
	container := map[string]int{}

	loadCalled := 0
	loader := func(ctx context.Context, name string) (int, error) {
		loadCalled++
		if name == "fail" {
			return 0, errors.New("loader error")
		}
		return len(name), nil // synthetic value
	}

	set := func(name string, v int) { container[name] = v }
	remove := func(name string) { delete(container, name) }

	r := NewAdaptor[int](loader, set, remove)

	// --- add/update ---
	err := r.Reload(ctx, "alpha", AddOrUpdate)
	assert.NoError(t, err)
	assert.EqualValues(t, 1, loadCalled)
	assert.EqualValues(t, map[string]int{"alpha": 5}, container)

	// --- delete ---
	err = r.Reload(ctx, "alpha", Delete)
	assert.NoError(t, err)
	assert.Len(t, container, 0)

	// --- loader error propagates ---
	err = r.Reload(ctx, "fail", AddOrUpdate)
	assert.Error(t, err)
	assert.EqualValues(t, 2, loadCalled) // called again
}
