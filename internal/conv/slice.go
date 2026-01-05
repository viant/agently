package conv

import (
	"reflect"
)

// MergeSlices concatenates two slice values i and j using reflection.
// It returns a new slice (same concrete type as the first non-nil arg) as interface{}.
func MergeSlices(slice1, slice2 interface{}) (interface{}, error) {
	vi := reflect.ValueOf(slice1)
	vj := reflect.ValueOf(slice2)

	// Allow pointers
	for vi.IsValid() && vi.Kind() == reflect.Ptr {
		vi = vi.Elem()
	}
	for vj.IsValid() && vj.Kind() == reflect.Ptr {
		vj = vj.Elem()
	}

	// nil handling
	if !vi.IsValid() {
		return slice2, nil
	}
	if !vj.IsValid() {
		return slice1, nil
	}

	// If both are slices, try to preserve the slice type.
	if vi.Kind() == reflect.Slice && vj.Kind() == reflect.Slice {
		ti := vi.Type()
		tj := vj.Type()

		// Fast path: identical type
		if ti == tj {
			out := reflect.MakeSlice(ti, 0, vi.Len()+vj.Len())
			out = reflect.AppendSlice(out, vi)
			out = reflect.AppendSlice(out, vj)
			return out.Interface(), nil
		}

		eli := ti.Elem()
		elj := tj.Elem()
		if !elj.AssignableTo(eli) {
			// Fallback to []interface{} for heterogeneous lists.
			return mergeAsInterfaces(slice1, slice2), nil
		}

		out := reflect.MakeSlice(ti, 0, vi.Len()+vj.Len())
		out = reflect.AppendSlice(out, vi)
		for k := 0; k < vj.Len(); k++ {
			elem := vj.Index(k)
			if !elem.Type().AssignableTo(eli) {
				return mergeAsInterfaces(slice1, slice2), nil
			}
			out = reflect.Append(out, elem)
		}
		return out.Interface(), nil
	}

	// One side is a slice: try to append the other as an element.
	if vi.Kind() == reflect.Slice && vj.Kind() != reflect.Slice {
		ti := vi.Type()
		eli := ti.Elem()
		elem := vj
		if elem.Type().AssignableTo(eli) {
			out := reflect.MakeSlice(ti, 0, vi.Len()+1)
			out = reflect.AppendSlice(out, vi)
			out = reflect.Append(out, elem)
			return out.Interface(), nil
		}
		return mergeAsInterfaces(slice1, slice2), nil
	}
	if vi.Kind() != reflect.Slice && vj.Kind() == reflect.Slice {
		tj := vj.Type()
		elj := tj.Elem()
		elem := vi
		if elem.Type().AssignableTo(elj) {
			out := reflect.MakeSlice(tj, 0, vj.Len()+1)
			out = reflect.Append(out, elem)
			out = reflect.AppendSlice(out, vj)
			return out.Interface(), nil
		}
		return mergeAsInterfaces(slice1, slice2), nil
	}

	// Neither is a slice → represent as a 2-element list.
	return mergeAsInterfaces(slice1, slice2), nil
}

func mergeAsInterfaces(v1, v2 interface{}) []interface{} {
	out := make([]interface{}, 0, 2)
	out = append(out, toInterfaces(v1)...)
	out = append(out, toInterfaces(v2)...)
	return out
}

func toInterfaces(v interface{}) []interface{} {
	rv := reflect.ValueOf(v)
	for rv.IsValid() && rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	if !rv.IsValid() {
		return nil
	}
	if rv.Kind() != reflect.Slice {
		return []interface{}{v}
	}
	n := rv.Len()
	out := make([]interface{}, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, rv.Index(i).Interface())
	}
	return out
}
