package templating

import (
	"reflect"

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
		if err := planner.DefineVariable(k, unwrapPtr(v)); err != nil {
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
		if err := state.SetValue(k, unwrapPtr(v)); err != nil {
			return "", err
		}
	}
	if err := exec.Exec(state); err != nil {
		return "", err
	}
	return string(state.Buffer.Bytes()), nil
}

// unwrapPtr returns the value pointed to by v when v is a non-nil pointer;
// otherwise it returns v unchanged. This helps align velty variable definitions
// and assignments with concrete struct types instead of pointers.
func unwrapPtr(v interface{}) interface{} {
	rv := reflect.ValueOf(v)
	if rv.IsValid() && rv.Kind() == reflect.Ptr && !rv.IsNil() {
		return rv.Elem().Interface()
	}
	return v
}
