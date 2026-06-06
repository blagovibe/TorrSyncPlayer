package errors

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAppError_Error(t *testing.T) {
	err := &AppError{Type: ErrNotFound, Message: "resource not found"}
	assert.Equal(t, "[NOT_FOUND] resource not found", err.Error())
}

func TestAppError_ErrorWithWrap(t *testing.T) {
	inner := fmt.Errorf("inner error")
	err := &AppError{Type: ErrInternal, Message: "wrapped", Err: inner}
	assert.Contains(t, err.Error(), "INTERNAL")
	assert.Contains(t, err.Error(), "wrapped")
	assert.Contains(t, err.Error(), "inner error")
}

func TestAppError_Unwrap(t *testing.T) {
	inner := fmt.Errorf("inner")
	err := &AppError{Type: ErrInternal, Message: "test", Err: inner}
	assert.True(t, errors.Is(err, inner))
}

func TestNew(t *testing.T) {
	err := New(ErrNotFound, "resource not found")
	assert.Equal(t, ErrNotFound, err.Type)
	assert.Equal(t, "resource not found", err.Message)
	assert.Nil(t, err.Err)
}

func TestWrap(t *testing.T) {
	inner := fmt.Errorf("inner")
	err := Wrap(ErrInternal, "context", inner)
	assert.Equal(t, ErrInternal, err.Type)
	assert.Equal(t, "context", err.Message)
	assert.Equal(t, inner, err.Err)
}

func TestNotFound(t *testing.T) {
	err := NotFound("user", "123")
	assert.Equal(t, ErrNotFound, err.Type)
	assert.Contains(t, err.Message, "user")
	assert.Contains(t, err.Message, "123")
}

func TestAlreadyExists(t *testing.T) {
	err := AlreadyExists("user", "123")
	assert.Equal(t, ErrAlreadyExists, err.Type)
}

func TestInvalidInput(t *testing.T) {
	err := InvalidInput("bad data")
	assert.Equal(t, ErrInvalidInput, err.Type)
}

func TestUnauthorized(t *testing.T) {
	err := Unauthorized("no token")
	assert.Equal(t, ErrUnauthorized, err.Type)
}

func TestForbidden(t *testing.T) {
	err := Forbidden("admin only")
	assert.Equal(t, ErrForbidden, err.Type)
}

func TestInternal(t *testing.T) {
	inner := fmt.Errorf("db error")
	err := Internal("database", inner)
	assert.Equal(t, ErrInternal, err.Type)
}

func TestTimeout(t *testing.T) {
	err := Timeout("connect")
	assert.Equal(t, ErrTimeout, err.Type)
}

func TestUnavailable(t *testing.T) {
	err := Unavailable("database")
	assert.Equal(t, ErrUnavailable, err.Type)
}

func TestIsNotFound(t *testing.T) {
	assert.True(t, IsNotFound(NotFound("x", "y")))
	assert.False(t, IsNotFound(errors.New("other")))
	assert.False(t, IsNotFound(nil))
}

func TestIsNotFound_Wrapped(t *testing.T) {
	inner := NotFound("x", "y")
	wrapped := fmt.Errorf("wrap: %w", inner)
	assert.True(t, IsNotFound(wrapped))
}

func TestIsAlreadyExists(t *testing.T) {
	assert.True(t, IsAlreadyExists(AlreadyExists("x", "y")))
	assert.False(t, IsAlreadyExists(errors.New("other")))
}

func TestIsInvalidInput(t *testing.T) {
	assert.True(t, IsInvalidInput(InvalidInput("bad")))
	assert.False(t, IsInvalidInput(errors.New("other")))
}

func TestIsUnauthorized(t *testing.T) {
	assert.True(t, IsUnauthorized(Unauthorized("no")))
	assert.False(t, IsUnauthorized(errors.New("other")))
}

func TestIsTimeout(t *testing.T) {
	assert.True(t, IsTimeout(Timeout("op")))
	assert.False(t, IsTimeout(errors.New("other")))
}

func TestIsForbidden(t *testing.T) {
	assert.True(t, IsForbidden(Forbidden("admin")))
	assert.False(t, IsForbidden(errors.New("other")))
}

func TestIsInternal(t *testing.T) {
	assert.True(t, IsInternal(Internal("db", fmt.Errorf("err"))))
	assert.False(t, IsInternal(errors.New("other")))
}

func TestIsUnavailable(t *testing.T) {
	assert.True(t, IsUnavailable(Unavailable("svc")))
	assert.False(t, IsUnavailable(errors.New("other")))
}
