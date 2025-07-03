package refiner

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/viant/mcp-protocol/schema"
)

func TestApplyPreset(t *testing.T) {
	// ------------------------------------------------------------------
	// Arrange – define a minimal schema with two properties.
	// ------------------------------------------------------------------
	rs := &schema.ElicitRequestParamsRequestedSchema{
		Type: "object",
		Properties: map[string]any{
			"b": map[string]any{"type": "string"},
			"a": map[string]any{"type": "string"},
		},
		Required: []string{"a", "b"},
	}

	// Global preset that wants custom order a→b and sets a custom widget for
	// field a.
	preset := &Preset{
		Fields: []map[string]any{
			{"name": "a", "widget": "textarea"},
			{"name": "b"},
		},
		Match: struct {
			Fields []string "json:\"fields,omitempty\" yaml:\"fields,omitempty\""
		}{Fields: []string{"a", "b"}},
	}

	SetGlobalPreset(preset)

	// ------------------------------------------------------------------
	// Act – call Refine which internally applies the preset.
	// ------------------------------------------------------------------
	Refine(rs)

	// ------------------------------------------------------------------
	// Assert – verify explicit x-ui-order and widget override.
	// ------------------------------------------------------------------
	aProp := rs.Properties["a"].(map[string]any)
	bProp := rs.Properties["b"].(map[string]any)

	assert.EqualValues(t, 10, aProp["x-ui-order"])
	assert.EqualValues(t, 20, bProp["x-ui-order"])
	assert.EqualValues(t, "textarea", aProp["widget"])
}

func TestMultiplePresetLoad(t *testing.T) {
	rs := &schema.ElicitRequestParamsRequestedSchema{
		Type: "object",
		Properties: map[string]any{
			"x": map[string]any{"type": "string"},
			"y": map[string]any{"type": "string"},
		},
	}

	p1 := &Preset{
		Fields: []map[string]any{{"name": "x", "widget": "text"}},
		Match: struct {
			Fields []string "json:\"fields,omitempty\" yaml:\"fields,omitempty\""
		}{Fields: []string{"x", "y"}},
	}

	// Register single preset.
	globalPresets = []*Preset{p1}

	Refine(rs)

	xProp := rs.Properties["x"].(map[string]any)
	assert.EqualValues(t, "text", xProp["widget"])
}
