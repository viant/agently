package resources

import (
	"testing"
)

// TestNormalizeUserRoot verifies user inputs normalize to workspace or mcp.
func TestNormalizeUserRoot(t *testing.T) {
	s := &Service{}
	type tc struct {
		name    string
		loc     string
		expURI  string
		expKind string
	}
	cases := []tc{
		{
			name:    "workspace kind agents/",
			loc:     "agents/polaris/system_knowledge/",
			expURI:  "workspace://localhost/agents/polaris/system_knowledge",
			expKind: "workspace",
		},
		{
			name:    "explicit file url under workspace (unknown root)",
			loc:     "file:///var/ws/agents/polaris/knowledge/",
			expURI:  "file:///var/ws/agents/polaris/knowledge/",
			expKind: "file",
		},
		{
			name:    "explicit file url",
			loc:     "file:///opt/app/docs/",
			expURI:  "file:///opt/app/docs/",
			expKind: "file",
		},
		{
			name:    "mcp uri passthrough",
			loc:     "mcp:server:/docs",
			expURI:  "mcp:server:/docs",
			expKind: "mcp",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotURI, gotKind, err := s.normalizeUserRoot(nil, c.loc)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if c.expKind != gotKind {
				t.Fatalf("kind mismatch: expected %q got %q (uri=%s)", c.expKind, gotKind, gotURI)
			}
			if c.expURI != "" && c.expURI != gotURI {
				t.Fatalf("uri mismatch: expected %q got %q", c.expURI, gotURI)
			}
		})
	}
}
