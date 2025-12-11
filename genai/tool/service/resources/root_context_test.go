package resources

import "testing"

func TestJoinBaseWithPath_RootSlash(t *testing.T) {
	base := "/tmp/root"
	ws := "workspace://localhost/root"
	got, err := joinBaseWithPath(ws, base, "/", "workspace://localhost/root")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != base {
		t.Fatalf("expected %s, got %s", base, got)
	}
}

func TestToWorkspaceURI_DataDriven(t *testing.T) {
	type testCase struct {
		name     string
		input    string
		expected string
	}
	workspacePrefix := "workspace://localhost"
	cases := []testCase{
		{name: "relative path", input: "knowledge/bidder", expected: "knowledge/bidder"},
		{name: "simple segment", input: "bidder", expected: "bidder"},
		{name: "workspace uri passthrough", input: workspacePrefix + "/knowledge/bidder", expected: workspacePrefix + "/knowledge/bidder"},
		{name: "file uri converted", input: "file://localhost/tmp/doc.md", expected: workspacePrefix + "/tmp/doc.md"},
		{name: "abs path converted", input: "/tmp/doc.md", expected: workspacePrefix + "/tmp/doc.md"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := toWorkspaceURI(tc.input)
			if got != tc.expected {
				t.Fatalf("expected %s, got %s", tc.expected, got)
			}
		})
	}
}
