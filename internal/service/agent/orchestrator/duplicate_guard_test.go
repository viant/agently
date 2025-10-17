package orchestrator

import (
	plan "github.com/viant/agently/genai/llm"

	"testing"
)

func TestDuplicateGuard_ShouldBlock(t *testing.T) {
	testCases := []struct {
		name         string
		priorResults []plan.ToolCall
		callName     string
		callArgs     map[string]interface{}
		wantBlock    bool
	}{
		{
			name: "block when prior successful result exists",
			priorResults: []plan.ToolCall{{
				Name:      "sqlkit-dbListConnections",
				Arguments: map[string]interface{}{"pattern": "*"},
				Result:    "[{\"name\":\"dev\"}]",
				Error:     "",
			}},
			callName:  "sqlkit-dbListConnections",
			callArgs:  map[string]interface{}{"pattern": "*"},
			wantBlock: true,
		},
		{
			name: "allow retry when previous result had error",
			priorResults: []plan.ToolCall{{
				Name:      "sqlkit-dbListConnections",
				Arguments: map[string]interface{}{"pattern": "*"},
				Error:     "connection timeout",
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

func TestDuplicateGuard_ConsecutiveCalls(t *testing.T) {
	type call struct {
		Name      string
		Args      map[string]interface{}
		Error     string
		wantBlock bool
	}

	testCases := []struct {
		name     string
		sequence []call
	}{
		{
			name: "block second and later consecutive identical call when NO error present",
			sequence: []call{
				{
					Name:      "sqlkit-query",
					Args:      map[string]interface{}{"query": "SELECT * FROM users"},
					Error:     "",
					wantBlock: false,
				},
				{
					Name:      "sqlkit-query",
					Args:      map[string]interface{}{"query": "SELECT * FROM users"},
					Error:     "",
					wantBlock: true, // Second consecutive identical call should be blocked
				},
				{
					Name:      "sqlkit-query",
					Args:      map[string]interface{}{"query": "SELECT * FROM users"},
					Error:     "",
					wantBlock: true, // Third consecutive identical call should be blocked
				},
			},
		},
		{
			name: "block second consecutive identical call, reset, block new second identical call when NO error present",
			sequence: []call{
				{
					Name:      "sqlkit-query",
					Args:      map[string]interface{}{"query": "SELECT * FROM users"},
					Error:     "",
					wantBlock: false,
				},
				{
					Name:      "sqlkit-query",
					Args:      map[string]interface{}{"query": "SELECT * FROM users"},
					Error:     "",
					wantBlock: true, // Second consecutive identical call should be blocked
				},
				{
					Name:      "sqlkit-query",
					Args:      map[string]interface{}{"query": "SELECT * FROM orders"}, // Different query
					Error:     "some error",
					wantBlock: false, // Counter should reset
				},
				{
					Name:      "sqlkit-query",
					Args:      map[string]interface{}{"query": "SELECT * FROM users"},
					Error:     "",
					wantBlock: false, // First call shouldn't be blocked
				},
				{
					Name:      "sqlkit-query",
					Args:      map[string]interface{}{"query": "SELECT * FROM users"},
					Error:     "",
					wantBlock: true, // Second consecutive identical call should be blocked
				},
			},
		},
		{
			name: "block 3 (consecutiveLimit) consecutive identical call when error present",
			sequence: []call{
				{
					Name:      "sqlkit-query",
					Args:      map[string]interface{}{"query": "SELECT * FROM users"},
					Error:     "some error",
					wantBlock: false,
				},
				{
					Name:      "sqlkit-query",
					Args:      map[string]interface{}{"query": "SELECT * FROM user"},
					Error:     "some error",
					wantBlock: false,
				},
				{
					Name:      "sqlkit-query",
					Args:      map[string]interface{}{"query": "SELECT * FROM user"},
					Error:     "some error",
					wantBlock: true, // Third consecutive identical call should be blocked
				},

				{
					Name:      "sqlkit-query",
					Args:      map[string]interface{}{"query": "SELECT * FROM user"},
					Error:     "some error",
					wantBlock: true, // Fourth consecutive identical call should be blocked
				},
			},
		},
		{
			name: "reset consecutive counter on different call",
			sequence: []call{
				{
					Name:      "sqlkit-query",
					Args:      map[string]interface{}{"query": "SELECT * FROM user"},
					Error:     "some error",
					wantBlock: false,
				},
				{
					Name:      "sqlkit-query",
					Args:      map[string]interface{}{"query": "SELECT * FROM user"},
					Error:     "some error",
					wantBlock: false,
				},
				{
					Name:      "sqlkit-query",
					Args:      map[string]interface{}{"query": "SELECT * FROM orders"}, // Different query
					Error:     "some error",
					wantBlock: false, // Counter should reset
				},
				{
					Name:      "sqlkit-query",
					Args:      map[string]interface{}{"query": "SELECT * FROM user"},
					Error:     "some error",
					wantBlock: false, // Counter should reset
				},
			},
		},
		{
			name: "mixed cases block consecutive identical calls and reseting when error present or not",
			sequence: []call{
				{
					Name:      "sqlkit-query",
					Args:      map[string]interface{}{"query": "SELECT * FROM user"},
					Error:     "some error",
					wantBlock: false,
				},
				{
					Name:      "sqlkit-query",
					Args:      map[string]interface{}{"query": "SELECT * FROM user"},
					Error:     "",
					wantBlock: false, // Second consecutive identical shouldn't be blocked when error occurred for same call before
				},
				{
					Name:      "sqlkit-query",
					Args:      map[string]interface{}{"query": "SELECT * FROM user"},
					Error:     "",
					wantBlock: true, // Third consecutive identical call should be blocked
				},
				{
					Name:      "sqlkit-query",
					Args:      map[string]interface{}{"query": "SELECT * FROM orders"}, // Different query
					Error:     "",
					wantBlock: false, // Counter should reset
				},
				{
					Name:      "sqlkit-query",
					Args:      map[string]interface{}{"query": "SELECT * FROM user"},
					Error:     "",
					wantBlock: false, // First call shouldn't be blocked
				},
				{
					Name:      "sqlkit-query",
					Args:      map[string]interface{}{"query": "SELECT * FROM user"},
					Error:     "",
					wantBlock: true, // Second consecutive identical call should be blocked
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			guard := NewDuplicateGuard(nil)

			for i, call := range tc.sequence {
				gotBlock, _ := guard.ShouldBlock(call.Name, call.Args)
				if gotBlock != call.wantBlock {
					t.Errorf("call %d: expected block=%v, got %v", i, call.wantBlock, gotBlock)
				}

				// Register the result if not blocked
				if !gotBlock {
					guard.RegisterResult(call.Name, call.Args, plan.ToolCall{
						Name:      call.Name,
						Arguments: call.Args,
						Result:    "some result",
						Error:     call.Error,
					})
				}
			}
		})
	}
}

func TestDuplicateGuard_WindowFrequency(t *testing.T) {

	type call struct {
		Name      string
		Args      map[string]interface{}
		Error     string
		wantBlock bool
	}

	sequence := []call{
		{Name: "a", Args: map[string]interface{}{"a": ""}, Error: "", wantBlock: false},
		{Name: "c", Args: map[string]interface{}{"c": ""}, Error: "", wantBlock: false},
		{Name: "X", Args: map[string]interface{}{"X": ""}, Error: "", wantBlock: false},
		{Name: "a", Args: map[string]interface{}{"a": ""}, Error: "", wantBlock: false},
		{Name: "X", Args: map[string]interface{}{"X": ""}, Error: "", wantBlock: false},
		{Name: "b", Args: map[string]interface{}{"b": ""}, Error: "", wantBlock: false},
		{Name: "X", Args: map[string]interface{}{"X": ""}, Error: "E", wantBlock: false},
		{Name: "X", Args: map[string]interface{}{"X": ""}, Error: "", wantBlock: false},
		{Name: "d", Args: map[string]interface{}{"d": ""}, Error: "", wantBlock: false},
		{Name: "X", Args: map[string]interface{}{"X": ""}, Error: "", wantBlock: true},  // 5th occurrence in window should be blocked, no matter previous X call was successful or not
		{Name: "1", Args: map[string]interface{}{"1": ""}, Error: "", wantBlock: false}, // 1-3 new entries in sliding window, make place for new X calls
		{Name: "2", Args: map[string]interface{}{"2": ""}, Error: "", wantBlock: false},
		{Name: "3", Args: map[string]interface{}{"3": ""}, Error: "", wantBlock: false},
		{Name: "4", Args: map[string]interface{}{"4": ""}, Error: "", wantBlock: false},
		{Name: "5", Args: map[string]interface{}{"5": ""}, Error: "", wantBlock: false},
		{Name: "X", Args: map[string]interface{}{"X": ""}, Error: "E", wantBlock: false},
		{Name: "X", Args: map[string]interface{}{"X": ""}, Error: "", wantBlock: false},
		{Name: "6", Args: map[string]interface{}{"6": ""}, Error: "", wantBlock: false},
		{Name: "X", Args: map[string]interface{}{"X": ""}, Error: "", wantBlock: false},
		{Name: "7", Args: map[string]interface{}{"7": ""}, Error: "", wantBlock: false},
		{Name: "X", Args: map[string]interface{}{"X": ""}, Error: "", wantBlock: false},
		{Name: "8", Args: map[string]interface{}{"8": ""}, Error: "", wantBlock: false},
		{Name: "X", Args: map[string]interface{}{"X": ""}, Error: "", wantBlock: true},
	}

	guard := NewDuplicateGuard(nil)

	for i, call := range sequence {
		gotBlock, _ := guard.ShouldBlock(call.Name, call.Args)
		if gotBlock != call.wantBlock {
			t.Errorf("call %d: expected block=%v, got %v", i, call.wantBlock, gotBlock)
		}

		// Register the result if not blocked
		if !gotBlock {
			guard.RegisterResult(call.Name, call.Args, plan.ToolCall{
				Name:      call.Name,
				Arguments: call.Args,
				Result:    "some result",
				Error:     call.Error,
			})
		}
	}
}

func TestDuplicateGuard_AlternatingPattern(t *testing.T) {
	type call struct {
		Name      string
		Args      map[string]interface{}
		Error     string
		wantBlock bool
	}

	// Create a sequence with alternating pattern A,B,A,B,A,B,A,B
	sequence := make([]call, 8)

	for i := 0; i < 8; i++ {
		if i%2 == 0 {
			sequence[i] = call{
				Name:      "call-A",
				Args:      map[string]interface{}{"type": "A"},
				wantBlock: false, //i == 6, // The 4th A call (at index 6) should be blocked
			}
		} else {
			sequence[i] = call{
				Name:      "call-B",
				Args:      map[string]interface{}{"type": "B"},
				wantBlock: i == 7, // The 4th B call (at index 7) should be blocked
			}
		}
	}

	guard := NewDuplicateGuard(nil)

	for i, call := range sequence {
		gotBlock, _ := guard.ShouldBlock(call.Name, call.Args)
		if gotBlock != call.wantBlock {
			t.Errorf("call %d: expected block=%v, got %v", i, call.wantBlock, gotBlock)
		}

		// Register the result if not blocked
		if !gotBlock {
			guard.RegisterResult(call.Name, call.Args, plan.ToolCall{
				Name:   call.Name,
				Args:   call.Args,
				Result: "some result",
				Error:  call.Error,
			})
		}
	}
}

func TestDuplicateGuard_Args(t *testing.T) {

	type call struct {
		name      string
		args      map[string]interface{}
		wantBlock bool
	}

	testCases := []struct {
		desc     string
		sequence []call
	}{
		{
			desc: "different argument order should be treated as same",
			sequence: []call{
				{
					name: "query",
					args: map[string]interface{}{
						"param1": "value1",
						"param2": "value2",
					},
					wantBlock: false,
				},
				{
					name: "query",
					args: map[string]interface{}{
						"param2": "value2", // Different order
						"param1": "value1",
					},
					wantBlock: true, // Should be blocked as it's the same call
				},
			},
		},
		{
			desc: "nested arguments should be canonicalized",
			sequence: []call{
				{
					name: "complex-query",
					args: map[string]interface{}{
						"filters": map[string]interface{}{
							"desc": "test",
							"age":  30,
						},
					},
					wantBlock: false,
				},
				{
					name: "complex-query",
					args: map[string]interface{}{
						"filters": map[string]interface{}{
							"age":  30, // Different order
							"desc": "test",
						},
					},
					wantBlock: true, // Should be blocked as it's the same call
				},
			},
		},
		{
			desc: "empty arguments should be handled correctly",
			sequence: []call{
				{
					name:      "no-args-call",
					args:      map[string]interface{}{},
					wantBlock: false,
				},
				{
					name:      "no-args-call",
					args:      nil,
					wantBlock: true, // nil and empty map should be treated as same
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			guard := NewDuplicateGuard(nil)

			for i, call := range tc.sequence {
				gotBlock, _ := guard.ShouldBlock(call.name, call.args)
				if gotBlock != call.wantBlock {
					t.Errorf("call %d: expected block=%v, got %v", i, call.wantBlock, gotBlock)
				}

				// Register the result if not blocked
				if !gotBlock {
					guard.RegisterResult(call.name, call.args, plan.ToolCall{
						Name:      call.name,
						Arguments: call.args,
						Result:    "some result",
					})
				}
			}
		})
	}
}
