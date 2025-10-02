package conv

import (
	"fmt"
	"reflect"
)

// MergeSlices concatenates two slice values i and j using reflection.
// It returns a new slice (same concrete type as the first non-nil arg) as interface{}.
func MergeSlices(slice1, slice2 interface{}) (interface{}, error) {
	vi := reflect.ValueOf(slice1)
	vj := reflect.ValueOf(slice2)

	// Allow pointers to slices
	for vi.IsValid() && vi.Kind() == reflect.Ptr {
		vi = vi.Elem()
	}
	for vj.IsValid() && vj.Kind() == reflect.Ptr {
		vj = vj.Elem()
	}

	if !vi.IsValid() || vi.Kind() != reflect.Slice {
		return nil, fmt.Errorf("first argument is not a slice")
	}
	if !vj.IsValid() || vj.Kind() != reflect.Slice {
		return nil, fmt.Errorf("second argument is not a slice")
	}

	ti := vi.Type() // e.g. []int
	tj := vj.Type()

	// Element types must be identical or assignable to the result element type.
	// We choose the type of slice1 as the result type for simplicity.
	eli := ti.Elem()
	elj := tj.Elem()

	if !(elj.AssignableTo(eli) && tj.ConvertibleTo(ti) || ti == tj) {
		// Fast path: same slice type
		if ti == tj {
			out := reflect.MakeSlice(ti, 0, vi.Len()+vj.Len())
			out = reflect.AppendSlice(out, vi)
			out = reflect.AppendSlice(out, vj)
			return out.Interface(), nil
		}
		// General check: all elements of slice2 must be assignable to elements of slice1
		if !elj.AssignableTo(eli) {
			return nil, fmt.Errorf("incompatible element types: %s vs %s", eli, elj)
		}
	}

	// Build result using the type of slice1
	out := reflect.MakeSlice(ti, 0, vi.Len()+vj.Len())
	out = reflect.AppendSlice(out, vi)

	// If the slice types differ (but elements are assignable), append slice2 element-by-element
	if ti == tj {
		out = reflect.AppendSlice(out, vj)
		return out.Interface(), nil
	}
	for k := 0; k < vj.Len(); k++ {
		elem := vj.Index(k)
		if !elem.Type().AssignableTo(eli) {
			return nil, fmt.Errorf("element %d of slice2 (%s) not assignable to %s", k, elem.Type(), eli)
		}
		out = reflect.Append(out, elem)
	}
	return out.Interface(), nil
}
