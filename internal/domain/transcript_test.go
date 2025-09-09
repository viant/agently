package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	plan "github.com/viant/agently/genai/agent/plan"
	msgread "github.com/viant/agently/internal/dao/message/read"
)

func iPtr(i int) *int { return &i }

func TestTranscript_FilterHelpers_DataDriven(t *testing.T) {
	// Seed transcript with mixed roles, types and interim flags
	t1 := &msgread.MessageView{Id: "u1", Role: "user", Type: "text", Content: "hi"}
	t2 := &msgread.MessageView{Id: "a1", Role: "assistant", Type: "text", Content: "hello"}
	t3 := &msgread.MessageView{Id: "i1", Role: "assistant", Type: "text", Content: "stream", Interim: iPtr(1)}
	t4 := &msgread.MessageView{Id: "c1", Role: "assistant", Type: "control", Content: "sys"}
	t5 := &msgread.MessageView{Id: "tl1", Role: "tool", Type: "text", Content: "tool output"}

	// attach executions to assistant to test outcomes/steps helpers
	t2.Executions = []*plan.Outcome{
		{ID: "op1", Steps: []*plan.StepOutcome{{ID: "s1", Name: "step-1"}}},
		{ID: "op2", Steps: []*plan.StepOutcome{{ID: "s2", Name: "step-2"}}},
	}

	tr := Transcript{t1, t2, t3, t4, t5}

	type tc struct {
		name      string
		gotAny    interface{}
		expectAny interface{}
	}

	cases := []tc{
		{
			name:      "WithoutInterim excludes interim",
			gotAny:    tr.WithoutInterim().IDs(),
			expectAny: []string{"u1", "a1", "c1", "tl1"},
		},
		{
			name:      "History keeps user/assistant non-interim and excludes control",
			gotAny:    tr.History().IDs(),
			expectAny: []string{"u1", "a1"},
		},
		{
			name:      "Users returns only user non-interim",
			gotAny:    tr.Users().IDs(),
			expectAny: []string{"u1"},
		},
		{
			name:      "AssistantsNonInterim returns only assistant non-interim",
			gotAny:    tr.AssistantsNonInterim().IDs(),
			expectAny: []string{"a1"},
		},
		{
			name:      "Outcomes flatten",
			gotAny:    tr.OutcomesIDs(),
			expectAny: []string{"op1", "op2"},
		},
		{
			name:      "StepOutcomes flatten",
			gotAny:    tr.StepIDs(),
			expectAny: []string{"s1", "s2"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.EqualValues(t, c.expectAny, c.gotAny)
		})
	}
}

// Helpers for testing equality in a compact way
func (t Transcript) IDs() []string {
	out := make([]string, 0, len(t))
	for _, v := range t {
		if v != nil {
			out = append(out, v.Id)
		}
	}
	return out
}

func (t Transcript) OutcomesIDs() []string {
	var out []string
	for _, oc := range t.Outcomes() {
		if oc != nil {
			out = append(out, oc.ID)
		}
	}
	return out
}

func (t Transcript) StepIDs() []string {
	var out []string
	for _, s := range t.StepOutcomes() {
		if s != nil {
			out = append(out, s.ID)
		}
	}
	return out
}
