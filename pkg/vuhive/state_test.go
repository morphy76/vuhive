package vuhive_test

import (
	"net/http"
	"testing"

	"github.com/morphy76/vuhive/pkg/vuhive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mapStateProvider struct {
	state map[string]any
}

func (m *mapStateProvider) GlobalState(key string) any {
	if m.state == nil {
		return nil
	}
	return m.state[key]
}

type customTestState struct {
	Name  string
	Count int
}

func TestState_ValidKeyAndType_ReturnsValueAndTrue(t *testing.T) {
	client := &http.Client{}
	custom := &customTestState{Name: "vuhive", Count: 42}
	sp := &mapStateProvider{
		state: map[string]any{
			"url":       "http://localhost:8080",
			"retries":   5,
			"client":    client,
			"custom":    custom,
			"is_active": true,
		},
	}

	// String
	strVal, ok := vuhive.State[string](sp, "url")
	assert.True(t, ok)
	assert.Equal(t, "http://localhost:8080", strVal)

	// Int
	intVal, ok := vuhive.State[int](sp, "retries")
	assert.True(t, ok)
	assert.Equal(t, 5, intVal)

	// Pointer to struct
	clientVal, ok := vuhive.State[*http.Client](sp, "client")
	assert.True(t, ok)
	assert.Same(t, client, clientVal)

	// Custom struct pointer
	customVal, ok := vuhive.State[*customTestState](sp, "custom")
	assert.True(t, ok)
	assert.Same(t, custom, customVal)

	// Bool
	boolVal, ok := vuhive.State[bool](sp, "is_active")
	assert.True(t, ok)
	assert.True(t, boolVal)
}

func TestState_MissingKey_ReturnsZeroAndFalse(t *testing.T) {
	sp := &mapStateProvider{
		state: map[string]any{
			"existing": "value",
		},
	}

	strVal, ok := vuhive.State[string](sp, "missing")
	assert.False(t, ok)
	assert.Equal(t, "", strVal)

	intVal, ok := vuhive.State[int](sp, "missing")
	assert.False(t, ok)
	assert.Equal(t, 0, intVal)

	ptrVal, ok := vuhive.State[*http.Client](sp, "missing")
	assert.False(t, ok)
	assert.Nil(t, ptrVal)
}

func TestState_TypeMismatch_ReturnsZeroAndFalse(t *testing.T) {
	sp := &mapStateProvider{
		state: map[string]any{
			"key_str": "not_an_int",
			"key_int": 123,
			"key_ptr": &http.Client{},
		},
	}

	// Requesting int for string value
	intVal, ok := vuhive.State[int](sp, "key_str")
	assert.False(t, ok)
	assert.Equal(t, 0, intVal)

	// Requesting string for int value
	strVal, ok := vuhive.State[string](sp, "key_int")
	assert.False(t, ok)
	assert.Equal(t, "", strVal)

	// Requesting incompatible struct pointer for *http.Client
	customVal, ok := vuhive.State[*customTestState](sp, "key_ptr")
	assert.False(t, ok)
	assert.Nil(t, customVal)
}

func TestState_NilStateProvider_ReturnsZeroAndFalse(t *testing.T) {
	var sp vuhive.StateProvider // nil interface

	strVal, ok := vuhive.State[string](sp, "any_key")
	assert.False(t, ok)
	assert.Equal(t, "", strVal)

	intVal, ok := vuhive.State[int](sp, "any_key")
	assert.False(t, ok)
	assert.Equal(t, 0, intVal)

	ptrVal, ok := vuhive.State[*http.Client](sp, "any_key")
	assert.False(t, ok)
	assert.Nil(t, ptrVal)
}

func TestMustState_Valid_ReturnsValue(t *testing.T) {
	client := &http.Client{}
	sp := &mapStateProvider{
		state: map[string]any{
			"url":    "http://localhost:8080",
			"client": client,
		},
	}

	assert.NotPanics(t, func() {
		strVal := vuhive.MustState[string](sp, "url")
		assert.Equal(t, "http://localhost:8080", strVal)

		clientVal := vuhive.MustState[*http.Client](sp, "client")
		assert.Same(t, client, clientVal)
	})
}

func TestMustState_MissingOrInvalid_PanicsWithDescriptiveMessage(t *testing.T) {
	sp := &mapStateProvider{
		state: map[string]any{
			"port": 8080,
		},
	}

	// Missing key
	assert.PanicsWithValue(t, "vuhive: global state key missing_key not found or invalid type", func() {
		_ = vuhive.MustState[string](sp, "missing_key")
	})

	// Invalid type
	assert.PanicsWithValue(t, "vuhive: global state key port not found or invalid type", func() {
		_ = vuhive.MustState[string](sp, "port")
	})

	// Nil StateProvider
	var nilSp vuhive.StateProvider
	assert.PanicsWithValue(t, "vuhive: global state key port not found or invalid type", func() {
		_ = vuhive.MustState[int](nilSp, "port")
	})
}

func TestStateOrDefault_FallbackWhenMissing(t *testing.T) {
	sp := &mapStateProvider{
		state: map[string]any{
			"url":  "http://localhost:8080",
			"port": 8080,
		},
	}

	// Existing matching key -> returns existing value
	assert.Equal(t, "http://localhost:8080", vuhive.StateOrDefault(sp, "url", "http://fallback:8080"))
	assert.Equal(t, 8080, vuhive.StateOrDefault(sp, "port", 9000))

	// Missing key -> returns default
	assert.Equal(t, "http://fallback:8080", vuhive.StateOrDefault(sp, "missing_url", "http://fallback:8080"))
	assert.Equal(t, 9000, vuhive.StateOrDefault(sp, "missing_port", 9000))

	// Incompatible type -> returns default
	assert.Equal(t, "default_str", vuhive.StateOrDefault[string](sp, "port", "default_str"))

	// Nil StateProvider -> returns default
	var nilSp vuhive.StateProvider
	assert.Equal(t, "fallback", vuhive.StateOrDefault[string](nilSp, "url", "fallback"))
}

func TestAlloc_StateAccessors(t *testing.T) {
	client := &http.Client{}
	sp := &mapStateProvider{
		state: map[string]any{
			"client": client,
			"count":  100,
		},
	}

	// Pre-warm / sanity check
	val, ok := vuhive.State[*http.Client](sp, "client")
	require.True(t, ok)
	require.Same(t, client, val)

	allocsStatePtr := testing.AllocsPerRun(1000, func() {
		_, _ = vuhive.State[*http.Client](sp, "client")
	})
	assert.Equal(t, float64(0), allocsStatePtr, "State[*http.Client] must produce 0 allocations")

	allocsMustStatePtr := testing.AllocsPerRun(1000, func() {
		_ = vuhive.MustState[*http.Client](sp, "client")
	})
	assert.Equal(t, float64(0), allocsMustStatePtr, "MustState[*http.Client] must produce 0 allocations")

	allocsStateOrDefaultPtr := testing.AllocsPerRun(1000, func() {
		_ = vuhive.StateOrDefault[*http.Client](sp, "client", nil)
	})
	assert.Equal(t, float64(0), allocsStateOrDefaultPtr, "StateOrDefault[*http.Client] must produce 0 allocations")

	allocsStateInt := testing.AllocsPerRun(1000, func() {
		_, _ = vuhive.State[int](sp, "count")
	})
	assert.Equal(t, float64(0), allocsStateInt, "State[int] must produce 0 allocations")
}
