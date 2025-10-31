package run

import (
	"reflect"
)

// inputKey is used to retrieve RunInput from request context for predicate handlers.
var inputKey = reflect.TypeOf(&RunInput{})
