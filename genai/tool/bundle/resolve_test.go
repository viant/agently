package bundle

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/viant/agently/genai/llm"
)

func TestResolveDefinitions(t *testing.T) {
	def := func(name string) *llm.ToolDefinition { return &llm.ToolDefinition{Name: name} }

	testCases := []struct {
		name     string
		bundle   *Bundle
		matches  map[string][]*llm.ToolDefinition
		expected []llm.ToolDefinition
	}{
		{
			name: "include_only",
			bundle: &Bundle{
				ID: "resources",
				Match: []MatchRule{
					{Name: "resources/*"},
				},
			},
			matches: map[string][]*llm.ToolDefinition{
				"resources/*": {def("resources:read"), def("resources:list")},
			},
			expected: []llm.ToolDefinition{
				{Name: "resources:list"},
				{Name: "resources:read"},
			},
		},
		{
			name: "exclude_subtracts_from_rule",
			bundle: &Bundle{
				ID: "resources",
				Match: []MatchRule{
					{Name: "resources/*", Exclude: []string{"resources:read"}},
				},
			},
			matches: map[string][]*llm.ToolDefinition{
				"resources/*":    {def("resources:read"), def("resources:list")},
				"resources:read": {def("resources:read")},
			},
			expected: []llm.ToolDefinition{
				{Name: "resources:list"},
			},
		},
		{
			name: "dedupe_across_rules",
			bundle: &Bundle{
				ID: "mixed",
				Match: []MatchRule{
					{Name: "resources/*"},
					{Name: "resources:read"},
				},
			},
			matches: map[string][]*llm.ToolDefinition{
				"resources/*":    {def("resources:read"), def("resources:list")},
				"resources:read": {def("resources:read")},
			},
			expected: []llm.ToolDefinition{
				{Name: "resources:list"},
				{Name: "resources:read"},
			},
		},
		{
			name: "supports_raw_and_canonical_variants_for_include",
			bundle: &Bundle{
				ID: "steward",
				Match: []MatchRule{
					{Name: "steward:AdHierarchy"},
					{Name: "steward-SaveRecommendation"},
				},
			},
			matches: map[string][]*llm.ToolDefinition{
				"steward:AdHierarchy":        {def("steward-AdHierarchy")},
				"steward-AdHierarchy":        {def("steward-AdHierarchy")},
				"steward-SaveRecommendation": {def("steward-SaveRecommendation")},
			},
			expected: []llm.ToolDefinition{
				{Name: "steward-AdHierarchy"},
				{Name: "steward-SaveRecommendation"},
			},
		},
		{
			name: "supports_canonical_bundle_names_against_raw_registry_names",
			bundle: &Bundle{
				ID: "steward",
				Match: []MatchRule{
					{Name: "steward-AdHierarchy"},
					{Name: "steward-SaveRecommendation"},
				},
			},
			matches: map[string][]*llm.ToolDefinition{
				"steward:AdHierarchy":        {def("steward:AdHierarchy")},
				"steward:SaveRecommendation": {def("steward:SaveRecommendation")},
			},
			expected: []llm.ToolDefinition{
				{Name: "steward:AdHierarchy"},
				{Name: "steward:SaveRecommendation"},
			},
		},
		{
			name: "supports_raw_and_canonical_variants_for_exclude",
			bundle: &Bundle{
				ID: "steward",
				Match: []MatchRule{
					{Name: "steward:*", Exclude: []string{"steward-SaveRecommendation"}},
				},
			},
			matches: map[string][]*llm.ToolDefinition{
				"steward:*":                  {def("steward-AdHierarchy"), def("steward-SaveRecommendation")},
				"steward-SaveRecommendation": {def("steward-SaveRecommendation")},
			},
			expected: []llm.ToolDefinition{
				{Name: "steward-AdHierarchy"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			matchFn := func(pattern string) []*llm.ToolDefinition {
				return tc.matches[pattern]
			}
			actual := ResolveDefinitions(tc.bundle, matchFn)
			assert.EqualValues(t, tc.expected, actual)
		})
	}
}
