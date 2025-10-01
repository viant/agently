package tool

import "github.com/viant/forge/backend/types"

// Feed describes a single matched rule and its extracted data.
// It is used as the payload for tool feeds rendered by the UI.
type Feed struct {
	ID    string `json:"id" yaml:"id"`
	Title string `json:"title,omitempty" yaml:"title,omitempty"`

	// Source identifies the originating tool call (service/method).
	Source Source `json:"source" yaml:"source"`

	// Data holds resolved data blocks keyed by data-source or container name.
	Data []DataItem `json:"data" yaml:"data"`

	// UI carries a renderable container definition for Forge-based UIs.
	UI *types.Container `json:"ui" yaml:"ui"`
}

// Source identifies the tool that produced the output observed by the rule.
type Source struct {
	Service string `json:"service" yaml:"service"`
	Method  string `json:"method" yaml:"method"`
}

// DataItem carries a resolved value and the raw selector used to obtain it.
// The type of Data is intentionally interface{} to support both object and
// collection values (e.g., a map with explanation/steps or a []map rows list).
type DataItem struct {
	Name        string      `json:"name" yaml:"name"`
	Data        interface{} `json:"data" yaml:"data"`
	RawSelector string      `json:"rawSelector" yaml:"rawSelector"`
}
