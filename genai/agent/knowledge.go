package agent

import "github.com/viant/embedius/matching/option"

type // Knowledge represents a knowledge base
Knowledge struct {
	Description   string          `yaml:"description,omitempty" json:"description,omitempty"`
	Match         *option.Options `json:"match,omitempty"` // Optional matching options
	URL           string          `yaml:"url,omitempty" json:"url,omitempty"`
	InclusionMode string          `yaml:"inclusionMode,omitempty" json:"inclusionMode,omitempty"` // Inclusion mode for the knowledge base
	MaxFiles      int             `yaml:"maxFiles,omitempty" json:"maxFiles,omitempty"`           // Max matched assets per knowledge (default 5)
}

// EffectiveMaxFiles returns the max files constraint with a default of 5 when unset.
func (k *Knowledge) EffectiveMaxFiles() int {
	if k == nil || k.MaxFiles <= 0 {
		return 5
	}
	return k.MaxFiles
}
