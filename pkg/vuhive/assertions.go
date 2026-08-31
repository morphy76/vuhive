package vuhive

import (
	"cmp"
	"fmt"
	"strings"
)

// Equal returns a CheckFunc that validates actual == expected.
func Equal[T comparable](actual, expected T) CheckFunc {
	return func() string {
		if actual != expected {
			return fmt.Sprintf("expected %v, got %v", expected, actual)
		}
		return ""
	}
}

// True returns a CheckFunc that validates condition is true.
// An optional failureReason can be provided to customize the failure message.
func True(condition bool, failureReason ...string) CheckFunc {
	return func() string {
		if !condition {
			if len(failureReason) > 0 {
				return failureReason[0]
			}
			return "expected condition to be true"
		}
		return ""
	}
}

// NoError returns a CheckFunc that validates err is nil.
func NoError(err error) CheckFunc {
	return func() string {
		if err != nil {
			return fmt.Sprintf("unexpected error: %v", err)
		}
		return ""
	}
}

// Contains returns a CheckFunc that validates string s contains substr.
func Contains(s, substr string) CheckFunc {
	return func() string {
		if !strings.Contains(s, substr) {
			return fmt.Sprintf("expected %q to contain %q", s, substr)
		}
		return ""
	}
}

// InRange returns a CheckFunc that validates val is within [min, max] inclusive.
func InRange[T cmp.Ordered](val, min, max T) CheckFunc {
	return func() string {
		if val < min || val > max {
			return fmt.Sprintf("expected %v in range [%v, %v]", val, min, max)
		}
		return ""
	}
}
