package bundle

import (
	"fmt"
	"strings"
)

// Bundle groups tool patterns into a selectable unit.
// Bundles are global (workspace) and may be referenced by ID from agents or runtime inputs.
type Bundle struct {
	ID          string `yaml:"id" json:"id"`
	Title       string `yaml:"title,omitempty" json:"title,omitempty"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`

	// IconRef refers to a built-in icon identifier (UI-owned mapping).
	IconRef string `yaml:"iconRef,omitempty" json:"iconRef,omitempty"`
	// IconURI references a workspace-local image (workspace://... only).
	IconURI string `yaml:"iconURI,omitempty" json:"iconURI,omitempty"`

	Priority int `yaml:"priority,omitempty" json:"priority,omitempty"`

	Match []MatchRule `yaml:"match,omitempty" json:"match,omitempty"`
}

type MatchRule struct {
	// Name is a tool match pattern (same semantics as tool pattern matching).
	Name string `yaml:"name" json:"name"`
	// Exclude subtracts patterns from the rule match set.
	Exclude []string `yaml:"exclude,omitempty" json:"exclude,omitempty"`
}

func (b *Bundle) Validate() error {
	if b == nil {
		return fmt.Errorf("bundle was nil")
	}
	if strings.TrimSpace(b.ID) == "" {
		return fmt.Errorf("bundle.id was empty")
	}
	if strings.TrimSpace(b.IconURI) != "" && !strings.HasPrefix(strings.TrimSpace(b.IconURI), "workspace://") {
		return fmt.Errorf("bundle.iconURI must start with workspace://")
	}
	if len(b.Match) == 0 {
		return fmt.Errorf("bundle.match is required")
	}
	for i, r := range b.Match {
		if strings.TrimSpace(r.Name) == "" {
			return fmt.Errorf("bundle.match[%d].name was empty", i)
		}
	}
	return nil
}
