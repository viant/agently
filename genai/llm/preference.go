package llm

import "github.com/viant/mcp-protocol/schema"

type ModelSelection struct {
	Model       string            `yaml:"model,omitempty" json:"model,omitempty"`
	Preferences *ModelPreferences `yaml:"modelPreferences,omitempty" json:"modelPreferences,omitempty"`
	Options     *Options          `yaml:"options,omitempty" json:"options,omitempty"`
	// Internal-only filters for model selection gatekeeping. These are set by
	// agent runtime from agent configuration and are not part of public JSON.
	AllowedProviders []string `yaml:"-" json:"-"`
	AllowedModels    []string `yaml:"-" json:"-"`
}

// ModelPreferences expresses caller priorities (0..1) + optional name hints.
type ModelPreferences struct {
	IntelligencePriority float64  `yaml:"intelligencePriority,omitempty" json:"intelligencePriority,omitempty"`
	SpeedPriority        float64  `yaml:"speedPriority,omitempty" json:"speedPriority,omitempty"`
	CostPriority         float64  `yaml:"costPriority,omitempty" json:"costPriority,omitempty"`
	Hints                []string `yaml:"hints,omitempty" json:"hints,omitempty" description:"model name"`
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
