package vuhive_test

import (
	"errors"
	"testing"

	"github.com/morphy76/vuhive/pkg/vuhive"
	"github.com/stretchr/testify/assert"
)

func TestAssertions_Equal_PassAndFail(t *testing.T) {
	t.Run("integers equal", func(t *testing.T) {
		fn := vuhive.Equal(200, 200)
		assert.Empty(t, fn())
	})

	t.Run("integers not equal", func(t *testing.T) {
		fn := vuhive.Equal(500, 200)
		assert.Equal(t, "expected 200, got 500", fn())
	})

	t.Run("strings equal", func(t *testing.T) {
		fn := vuhive.Equal("ok", "ok")
		assert.Empty(t, fn())
	})

	t.Run("strings not equal", func(t *testing.T) {
		fn := vuhive.Equal("fail", "ok")
		assert.Equal(t, "expected ok, got fail", fn())
	})
}

func TestAssertions_True_PassAndFail(t *testing.T) {
	t.Run("condition true", func(t *testing.T) {
		fn := vuhive.True(true)
		assert.Empty(t, fn())
	})

	t.Run("condition false default message", func(t *testing.T) {
		fn := vuhive.True(false)
		assert.Equal(t, "expected condition to be true", fn())
	})

	t.Run("condition false custom message", func(t *testing.T) {
		fn := vuhive.True(false, "custom failure message")
		assert.Equal(t, "custom failure message", fn())
	})
}

func TestAssertions_NoError_PassAndFail(t *testing.T) {
	t.Run("no error (nil)", func(t *testing.T) {
		fn := vuhive.NoError(nil)
		assert.Empty(t, fn())
	})

	t.Run("with error", func(t *testing.T) {
		err := errors.New("connection reset by peer")
		fn := vuhive.NoError(err)
		assert.Equal(t, "unexpected error: connection reset by peer", fn())
	})
}

func TestAssertions_Contains_PassAndFail(t *testing.T) {
	t.Run("contains substring", func(t *testing.T) {
		fn := vuhive.Contains("application/json; charset=utf-8", "application/json")
		assert.Empty(t, fn())
	})

	t.Run("does not contain substring", func(t *testing.T) {
		fn := vuhive.Contains("text/html", "application/json")
		assert.Equal(t, `expected "text/html" to contain "application/json"`, fn())
	})
}

func TestAssertions_InRange_PassAndFail(t *testing.T) {
	t.Run("in range integer inclusive min", func(t *testing.T) {
		fn := vuhive.InRange(10, 10, 20)
		assert.Empty(t, fn())
	})

	t.Run("in range integer inclusive max", func(t *testing.T) {
		fn := vuhive.InRange(20, 10, 20)
		assert.Empty(t, fn())
	})

	t.Run("in range integer middle", func(t *testing.T) {
		fn := vuhive.InRange(15, 10, 20)
		assert.Empty(t, fn())
	})

	t.Run("out of range integer below", func(t *testing.T) {
		fn := vuhive.InRange(9, 10, 20)
		assert.Equal(t, "expected 9 in range [10, 20]", fn())
	})

	t.Run("out of range integer above", func(t *testing.T) {
		fn := vuhive.InRange(21, 10, 20)
		assert.Equal(t, "expected 21 in range [10, 20]", fn())
	})

	t.Run("in range float", func(t *testing.T) {
		fn := vuhive.InRange(3.14, 0.0, 5.0)
		assert.Empty(t, fn())
	})

	t.Run("out of range float", func(t *testing.T) {
		fn := vuhive.InRange(6.28, 0.0, 5.0)
		assert.Equal(t, "expected 6.28 in range [0, 5]", fn())
	})
}
