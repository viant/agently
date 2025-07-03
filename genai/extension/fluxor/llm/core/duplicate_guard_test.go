package core

import (
	plan "github.com/viant/agently/genai/agent/plan"
	"testing"
)

func TestDuplicateGuard_ShouldBlock(t *testing.T) {
	testCases := []struct {
		name         string
		priorResults []plan.Result
		callName     string
		callArgs     map[string]interface{}
		wantBlock    bool
	}{
		{
			name: "block when prior successful result exists",
			priorResults: []plan.Result{{
				Name:   "sqlkit-dbListConnections",
				Args:   map[string]interface{}{"pattern": "*"},
				Result: "[{\"name\":\"dev\"}]",
				Error:  "",
			}},
			callName:  "sqlkit-dbListConnections",
			callArgs:  map[string]interface{}{"pattern": "*"},
			wantBlock: true,
		},
		{
			name: "allow retry when previous result had error",
			priorResults: []plan.Result{{
				Name:  "sqlkit-dbListConnections",
				Args:  map[string]interface{}{"pattern": "*"},
				Error: "connection timeout",
			}},
			callName:  "sqlkit-dbListConnections",
			callArgs:  map[string]interface{}{"pattern": "*"},
			wantBlock: false,
		},
	}

	for _, tc := range testCases {
		guard := NewDuplicateGuard(tc.priorResults)
		gotBlock, _ := guard.ShouldBlock(tc.callName, tc.callArgs)
		if gotBlock != tc.wantBlock {
			t.Errorf("%s: expected block=%v, got %v", tc.name, tc.wantBlock, gotBlock)
		}
	}
}
