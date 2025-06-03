package core

import "reflect"

// MapEqual reports whether a and b contain the same keys
// and all corresponding values are deeply equal.
// It treats nil and empty maps as equivalent.
func MapEqual(a, b map[string]interface{}) bool {
	// Fast path: identical pointers.
	if &a == &b {
		return true
	}

	// len works on nil maps (returns 0), so this also covers
	// the nil-vs-empty case.
	if len(a) != len(b) {
		return false
	}

	for k, av := range a {
		bv, ok := b[k]
		if !ok { // key missing in b
			return false
		}
		if reflect.TypeOf(av) != reflect.TypeOf(bv) {
			continue
		}
		if !reflect.DeepEqual(av, bv) {
			return false // values differ
		}
	}
	return true
}
