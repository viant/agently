package templating

import (
	"github.com/viant/velty"
)

// Expand renders the provided template string using the velty engine and the
// supplied variables. All keys in vars are defined as variables during
// compilation and then populated into the execution state. The returned string
// is the rendered output buffer.
func Expand(tmpl string, vars map[string]interface{}) (string, error) {
	planner := velty.New()
	// Define variables for compilation
	for k, v := range vars {
		if err := planner.DefineVariable(k, v); err != nil {
			return "", err
		}
	}
	exec, newState, err := planner.Compile([]byte(tmpl))
	if err != nil {
		return "", err
	}
	state := newState()
	// Populate values for execution
	for k, v := range vars {
		if err := state.SetValue(k, v); err != nil {
			return "", err
		}
	}
	if err := exec.Exec(state); err != nil {
		return "", err
	}
	return string(state.Buffer.Bytes()), nil
}
