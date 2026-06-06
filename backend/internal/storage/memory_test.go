package storage

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMemoryStorage(t *testing.T) {
	s := NewMemoryStorage(1024)
	assert.NotNil(t, s)
	ms := s.(*memoryStorage)
	assert.Equal(t, int64(1024), ms.GetCapacity())
	assert.Equal(t, int64(0), ms.GetUsed())
}

func TestNewMemoryStorage_ZeroCapacity(t *testing.T) {
	s := NewMemoryStorage(0)
	assert.NotNil(t, s)
	ms := s.(*memoryStorage)
	assert.Equal(t, int64(0), ms.GetCapacity())
}

func TestMemoryStorage_GetUsed(t *testing.T) {
	s := NewMemoryStorage(1024 * 1024)
	ms := s.(*memoryStorage)
	assert.Equal(t, int64(0), ms.GetUsed())
}

func TestMemoryStorage_GetCapacity(t *testing.T) {
	s := NewMemoryStorage(512)
	ms := s.(*memoryStorage)
	assert.Equal(t, int64(512), ms.GetCapacity())
}

func TestMemoryStorage_Close(t *testing.T) {
	s := NewMemoryStorage(1024)
	ms := s.(*memoryStorage)
	err := ms.Close()
	assert.NoError(t, err)
	assert.Equal(t, int64(0), ms.GetUsed())
}

func TestMemoryPieceImpl_ReadAt(t *testing.T) {
	p := &memoryPieceImpl{
		data:   []byte("hello world"),
		length: int64(len("hello world")),
	}

	buf := make([]byte, 5)
	n, err := p.ReadAt(buf, 0)
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, "hello", string(buf))
}

func TestMemoryPieceImpl_ReadAt_Offset(t *testing.T) {
	p := &memoryPieceImpl{
		data:   []byte("hello world"),
		length: int64(len("hello world")),
	}

	buf := make([]byte, 5)
	n, err := p.ReadAt(buf, 6)
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, "world", string(buf))
}

func TestMemoryPieceImpl_ReadAt_BeyondEnd(t *testing.T) {
	p := &memoryPieceImpl{
		data:   []byte("hi"),
		length: 2,
	}

	buf := make([]byte, 10)
	n, err := p.ReadAt(buf, 0)
	require.NoError(t, err)
	assert.Equal(t, 2, n)
}

func TestMemoryPieceImpl_WriteAt(t *testing.T) {
	p := &memoryPieceImpl{
		data:   nil,
		length: 10,
	}

	n, err := p.WriteAt([]byte("hello"), 0)
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, int64(5), p.bytesAllocated)
}

func TestMemoryPieceImpl_WriteAt_ExistingData(t *testing.T) {
	p := &memoryPieceImpl{
		data:   make([]byte, 10),
		length: 10,
	}

	n, err := p.WriteAt([]byte("hello"), 0)
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, "hello", string(p.data[:5]))
}

func TestMemoryPieceImpl_MarkComplete(t *testing.T) {
	p := &memoryPieceImpl{data: make([]byte, 10), length: 10}
	assert.False(t, p.Completion().Complete)

	err := p.MarkComplete()
	require.NoError(t, err)
	assert.True(t, p.Completion().Complete)

	err = p.MarkNotComplete()
	require.NoError(t, err)
	assert.False(t, p.Completion().Complete)
}
