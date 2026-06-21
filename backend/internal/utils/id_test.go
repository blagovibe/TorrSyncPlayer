package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateID(t *testing.T) {
	id, err := GenerateID(16)
	require.NoError(t, err)
	assert.Len(t, id, 32)
}

func TestGenerateID_DifferentLengths(t *testing.T) {
	for _, length := range []int{1, 8, 16, 32} {
		id, err := GenerateID(length)
		require.NoError(t, err)
		assert.Len(t, id, length*2)
	}
}

func TestGenerateID_Uniqueness(t *testing.T) {
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id, err := GenerateID(16)
		require.NoError(t, err)
		assert.False(t, ids[id], "duplicate ID generated")
		ids[id] = true
	}
}
