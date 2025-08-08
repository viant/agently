package agent

import "github.com/viant/embedius/matching/option"

type // Knowledge represents a knowledge base
Knowledge struct {
	Description   string          `yaml:"description,omitempty" json:"description,omitempty"`
	Match         *option.Options `json:"match,omitempty"` // Optional matching options
	URL           string          `yaml:"url,omitempty" json:"url,omitempty"`
	InclusionMode string          `yaml:"inclusionMode,omitempty" json:"inclusionMode,omitempty"` // Inclusion mode for the knowledge base
}
