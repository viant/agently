package tool

import "github.com/viant/forge/backend/types"

type FeedSpec struct {
	ID         string           `yaml:"id,omitempty" json:"id,omitempty"`
	Title      string           `yaml:"title,omitempty" json:"title,omitempty"`
	Priority   int              `yaml:"priority,omitempty" json:"priority,omitempty"`
	Match      MatchSpec        `yaml:"match" json:"match"`
	Activation ActivationSpec   `yaml:"activation" json:"activation"`
	DataSource []DataSourceSpec `yaml:"dataSource" json:"dataSource"`
	UI         types.Container  `yaml:"ui" json:"ui"`
}

type MatchSpec struct {
	Service string `yaml:"service,omitempty" json:"service,omitempty"`
	Method  string `yaml:"method,omitempty" json:"method,omitempty"`
}

type ActivationSpec struct {
	// Kind selects the activation mode: "history" or "tool_call".
	Kind string `yaml:"kind,omitempty" json:"kind,omitempty"`
	// Scope controls how many recorded calls are considered when kind==history:
	//  - "last" (default): only the most recent matching call is used
	//  - "all": aggregate data from all matching calls in the turn
	Scope string `yaml:"scope,omitempty" json:"scope,omitempty"`
	// Optional explicit tool to invoke when kind==tool_call. When omitted,
	// match.service/method may be used as a fallback by the consumer.
	Service string                 `yaml:"service,omitempty" json:"service,omitempty"`
	Method  string                 `yaml:"method,omitempty" json:"method,omitempty"`
	Args    map[string]interface{} `yaml:"args,omitempty" json:"args,omitempty"`
}

type DataSourceSpec struct {
	Name     string `yaml:"name" json:"name"`
	Selector string `yaml:"selector" json:"selector"`
}
