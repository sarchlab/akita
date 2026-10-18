package naming

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNaming(t *testing.T) {
	t.Run("should parse name", func(t *testing.T) {
		name := parseName("GPU[0].Core[0]")
		require.Equal(t, "GPU", name.Tokens[0].ElemName)
		require.Equal(t, []int{0}, name.Tokens[0].Index)
		require.Equal(t, "Core", name.Tokens[1].ElemName)
		require.Equal(t, []int{0}, name.Tokens[1].Index)
	})

	t.Run("should parse multi-dimensional index", func(t *testing.T) {
		name := parseName("GPU[0][1].Core[0][1]")
		require.Equal(t, "GPU", name.Tokens[0].ElemName)
		require.Equal(t, []int{0, 1}, name.Tokens[0].Index)
		require.Equal(t, "Core", name.Tokens[1].ElemName)
		require.Equal(t, []int{0, 1}, name.Tokens[1].Index)
	})

	t.Run("should panic if the name is empty", func(t *testing.T) {
		require.Panics(t, func() { MustBeValid("") })
	})

	t.Run("should panic if name include underscore", func(t *testing.T) {
		require.Panics(t, func() { MustBeValid("GPU_0") })
	})

	t.Run("should panic if name include dash", func(t *testing.T) {
		require.Panics(t, func() { MustBeValid("GPU-0") })
	})

	t.Run("should panic if name is not capitalized CamelCase", func(t *testing.T) {
		require.Panics(t, func() { MustBeValid("gpu0") })
	})

	t.Run("should have paired square brackets", func(t *testing.T) {
		require.Panics(t, func() { MustBeValid("GPU[0") })
	})

	t.Run("should have paired square brackets", func(t *testing.T) {
		require.Panics(t, func() { MustBeValid("GPU0]") })
	})

	t.Run("should be panic if element name is empty", func(t *testing.T) {
		require.Panics(t, func() { MustBeValid("GPU..0") })
	})

	t.Run("should build name", func(t *testing.T) {
		require.Equal(t, "GPU", BuildName("", "GPU"))
		require.Equal(t, "GPU.Core", BuildName("GPU", "Core"))
	})

	t.Run("should build name with index", func(t *testing.T) {
		require.Equal(t, "GPU[0]", BuildNameWithIndex("", "GPU", 0))
		require.Equal(t, "GPU.Core[0]", BuildNameWithIndex("GPU", "Core", 0))
	})

	t.Run("should build name with multi-dimensional index", func(t *testing.T) {
		require.Equal(t, "GPU[0]", BuildNameWithMultiDimensionalIndex("", "GPU", []int{0}))
		require.Equal(t, "GPU.Core[0][1]", BuildNameWithMultiDimensionalIndex("GPU", "Core", []int{0, 1}))
	})
}
