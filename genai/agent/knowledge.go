package agent

import "github.com/viant/embedius/matching/option"

type // Knowledge represents a knowledge base
Knowledge struct {
	Description string          `yaml:"description,omitempty" json:"description,omitempty"`
	Match       *option.Options `json:"match,omitempty"` // Optional matching options
	URL         string          `yaml:"Paths,omitempty" json:"Paths,omitempty"`
}
