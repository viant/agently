package agent

import "github.com/viant/embedius/matching/option"

type // Knowledge represents a knowledge base
Knowledge struct {
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	// Filter defines optional pre-filtering rules (inclusions/exclusions, max file size)
	// applied when selecting knowledge documents. It replaces the older "match" block
	// in YAML while remaining backwards compatible via the loader.
	Filter        *option.Options `yaml:"filter,omitempty" json:"filter,omitempty"`
	URL           string          `yaml:"url,omitempty" json:"url,omitempty"`
	InclusionMode string          `yaml:"inclusionMode,omitempty" json:"inclusionMode,omitempty"` // Inclusion mode for the knowledge base
	MaxFiles      int             `yaml:"maxFiles,omitempty" json:"maxFiles,omitempty"`           // Max matched assets per knowledge (default 5)
	MinScore      *float64        `yaml:"minScore,omitempty" json:"minScore,omitempty"`           // Force match mode when set; optional score threshold
}

// EffectiveMaxFiles returns the max files constraint with a default of 5 when unset.
func (k *Knowledge) EffectiveMaxFiles() int {
	if k == nil || k.MaxFiles <= 0 {
		return 5
	}
	return k.MaxFiles
}
