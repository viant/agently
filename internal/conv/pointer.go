package conv

func Pointer[T any](value T) *T {
	return &value
}

func Dereference[T any](ptr *T) T {
	if ptr == nil {
		var zero T
		return zero
	}
	return *ptr
}
