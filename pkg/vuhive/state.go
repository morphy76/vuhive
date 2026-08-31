package vuhive

// State retrieves a typed value from the given StateProvider (VUContext or TeardownContext).
// Returns the typed value and true if the key exists and matches type T.
// Returns the zero value of T and false if the key does not exist or has an incompatible type.
func State[T any](sp StateProvider, key string) (T, bool) {
	if sp == nil {
		var zero T
		return zero, false
	}
	val := sp.GlobalState(key)
	if val == nil {
		var zero T
		return zero, false
	}
	typedVal, ok := val.(T)
	if !ok {
		var zero T
		return zero, false
	}
	return typedVal, true
}

// MustState retrieves a typed value from StateProvider and panics with a descriptive error
// message if the key is not found or cannot be converted to type T.
func MustState[T any](sp StateProvider, key string) T {
	val, ok := State[T](sp, key)
	if !ok {
		panic("vuhive: global state key " + key + " not found or invalid type")
	}
	return val
}

// StateOrDefault retrieves a typed value from StateProvider, returning defaultValue if the
// key is not found or has an incompatible type.
func StateOrDefault[T any](sp StateProvider, key string, defaultValue T) T {
	val, ok := State[T](sp, key)
	if !ok {
		return defaultValue
	}
	return val
}
