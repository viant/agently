package llm

import "github.com/viant/mcp-protocol/schema"

// ModelPreferences expresses caller priorities (0..1) + optional name hints.
type ModelPreferences struct {
	IntelligencePriority float64
	SpeedPriority        float64
	CostPriority         float64
	Hints                []string
}

// ModelPreferencesOption // is a functional option for ModelPreferences.
type ModelPreferencesOption func(*ModelPreferences)

func NewModelPreferences(options ...ModelPreferencesOption) *ModelPreferences {
	ret := &ModelPreferences{
		IntelligencePriority: 0.5,
		SpeedPriority:        0.5,
		CostPriority:         0.5,
		Hints:                make([]string, 0),
	}
	for _, opt := range options {
		opt(ret)
	}
	return ret
}

func WithPreferences(preferences *schema.ModelPreferences) ModelPreferencesOption {
	return func(p *ModelPreferences) {
		if preferences.IntelligencePriority != nil {
			p.IntelligencePriority = *preferences.IntelligencePriority
		}
		if preferences.SpeedPriority != nil {
			p.SpeedPriority = *preferences.SpeedPriority
		}
		if preferences.CostPriority != nil {
			p.CostPriority = *preferences.CostPriority
		}
		for _, hint := range preferences.Hints {
			if hint.Name != nil {
				p.Hints = append(p.Hints, *hint.Name)
			}
		}
	}
}
