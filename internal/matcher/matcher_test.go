package matcher

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/viant/agently/genai/llm"
)

func TestMatcher_Best_PrefersMiniWithBalancedPriorities(t *testing.T) {
	// openai_o4-mini vs openai_gpt-5.1, mirroring default config values.
	// o4-mini:  intelligence 0.85, speed 0.95, cost 0.004
	// gpt-5.1:  intelligence 0.96, speed 0.90, cost 0.01125
	// With equal priorities (0.7 each), the normalized cost penalty should
	// make o4-mini the better choice.
	cands := []Candidate{
		{ID: "openai_o4-mini", Intelligence: 0.85, Speed: 0.95, Cost: 0.004},
		{ID: "openai_gpt-5.1", Intelligence: 0.96, Speed: 0.90, Cost: 0.01125},
	}
	m := New(cands)
	prefs := &llm.ModelPreferences{
		IntelligencePriority: 0.7,
		SpeedPriority:        0.7,
		CostPriority:         0.7,
	}

	best := m.Best(prefs)
	assert.Equal(t, "openai_o4-mini", best, "balanced priorities should select o4-mini over gpt-5.1")
}

func TestMatcher_Best_HonoursHintsFirst(t *testing.T) {
	cands := []Candidate{
		{ID: "openai_o4-mini", Intelligence: 0.5, Speed: 0.5, Cost: 0.01},
		{ID: "openai_gpt-5.1", Intelligence: 0.9, Speed: 0.9, Cost: 0.02},
	}
	m := New(cands)
	// Even though gpt-5.1 would score higher, a hint for "o4-mini"
	// must take precedence.
	prefs := &llm.ModelPreferences{
		IntelligencePriority: 0.7,
		SpeedPriority:        0.7,
		CostPriority:         0.3,
		Hints:                []string{"o4-mini"},
	}

	best := m.Best(prefs)
	assert.Equal(t, "openai_o4-mini", best, "hint should force selection of matching candidate")
}

func TestMatcher_Best_HintDoesNotCollideWithGemini(t *testing.T) {
	cands := []Candidate{
		{ID: "openai_o4-mini", Intelligence: 0.5, Speed: 0.5, Cost: 0.01},
		{ID: "gemini_gemini-2.5-flash", Intelligence: 0.8, Speed: 0.9, Cost: 0.01},
	}
	m := New(cands)
	prefs := &llm.ModelPreferences{Hints: []string{"mini"}}

	best := m.Best(prefs)
	assert.Equal(t, "openai_o4-mini", best, "'mini' should match token '-mini' but not substring in 'gemini'")
}

func TestMatcher_Best_ProviderHintMatchesPrefix(t *testing.T) {
	cands := []Candidate{
		{ID: "openai_o4-mini", Intelligence: 0.5, Speed: 0.5, Cost: 0.01},
		{ID: "gemini_gemini-2.5-flash", Intelligence: 0.8, Speed: 0.9, Cost: 0.01},
	}
	m := New(cands)
	prefs := &llm.ModelPreferences{Hints: []string{"gemini"}}

	best := m.Best(prefs)
	assert.Equal(t, "gemini_gemini-2.5-flash", best, "provider hint should match provider prefix")
}

func TestMatcher_Best_ProviderReductionThenModelHint(t *testing.T) {
	cands := []Candidate{
		{ID: "gemini_gemini-mini", Intelligence: 0.9, Speed: 0.9, Cost: 0.01},
		{ID: "openai_o4-mini", Intelligence: 0.8, Speed: 0.95, Cost: 0.004},
	}
	m := New(cands)
	// Model hint first, provider hint second: provider must reduce candidates, then model hint applies.
	prefs := &llm.ModelPreferences{Hints: []string{"mini", "openai"}}

	best := m.Best(prefs)
	assert.Equal(t, "openai_o4-mini", best, "provider hint should restrict candidates before applying model hint")
}
