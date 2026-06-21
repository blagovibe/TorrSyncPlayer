// SPDX-License-Identifier: MIT

package errors

import (
	"errors"
	"fmt"
)

type ErrorType string

const (
	ErrNotFound      ErrorType = "NOT_FOUND"
	ErrAlreadyExists ErrorType = "ALREADY_EXISTS"
	ErrInvalidInput  ErrorType = "INVALID_INPUT"
	ErrUnauthorized  ErrorType = "UNAUTHORIZED"
	ErrForbidden     ErrorType = "FORBIDDEN"
	ErrInternal      ErrorType = "INTERNAL"
	ErrTimeout       ErrorType = "TIMEOUT"
	ErrUnavailable   ErrorType = "UNAVAILABLE"
)

type AppError struct {
	Type    ErrorType
	Message string
	Err     error
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Type, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s] %s", e.Type, e.Message)
}

func (e *AppError) Unwrap() error {
	return e.Err
}

func New(errType ErrorType, message string) *AppError {
	return &AppError{Type: errType, Message: message}
}

func Wrap(errType ErrorType, message string, err error) *AppError {
	return &AppError{Type: errType, Message: message, Err: err}
}

func NotFound(resource string, id string) *AppError {
	return &AppError{
		Type:    ErrNotFound,
		Message: fmt.Sprintf("%s not found: %s", resource, id),
	}
}

func AlreadyExists(resource string, id string) *AppError {
	return &AppError{
		Type:    ErrAlreadyExists,
		Message: fmt.Sprintf("%s already exists: %s", resource, id),
	}
}

func InvalidInput(details string) *AppError {
	return &AppError{
		Type:    ErrInvalidInput,
		Message: fmt.Sprintf("invalid input: %s", details),
	}
}

func Unauthorized(details string) *AppError {
	return &AppError{
		Type:    ErrUnauthorized,
		Message: fmt.Sprintf("unauthorized: %s", details),
	}
}

func Forbidden(details string) *AppError {
	return &AppError{
		Type:    ErrForbidden,
		Message: fmt.Sprintf("forbidden: %s", details),
	}
}

func Internal(details string, err error) *AppError {
	return &AppError{
		Type:    ErrInternal,
		Message: fmt.Sprintf("internal error: %s", details),
		Err:     err,
	}
}

func Timeout(operation string) *AppError {
	return &AppError{
		Type:    ErrTimeout,
		Message: fmt.Sprintf("operation timed out: %s", operation),
	}
}

func Unavailable(service string) *AppError {
	return &AppError{
		Type:    ErrUnavailable,
		Message: fmt.Sprintf("service unavailable: %s", service),
	}
}

func IsNotFound(err error) bool {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Type == ErrNotFound
	}
	return false
}

func IsAlreadyExists(err error) bool {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Type == ErrAlreadyExists
	}
	return false
}

func IsInvalidInput(err error) bool {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Type == ErrInvalidInput
	}
	return false
}

func IsUnauthorized(err error) bool {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Type == ErrUnauthorized
	}
	return false
}

func IsTimeout(err error) bool {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Type == ErrTimeout
	}
	return false
}

func IsForbidden(err error) bool {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Type == ErrForbidden
	}
	return false
}

func IsInternal(err error) bool {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Type == ErrInternal
	}
	return false
}

func IsUnavailable(err error) bool {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Type == ErrUnavailable
	}
	return false
}
