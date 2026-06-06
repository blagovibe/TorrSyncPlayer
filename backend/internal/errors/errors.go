// Package errors предоставляет единые типы ошибок для серверной части.
// Обеспечивает консистентную обработку ошибок во всех сервисах.
package errors

import (
	"errors"
	"fmt"
)

// ErrorType тип ошибки для категоризации
type ErrorType string

const (
	// ErrNotFound ресурс не найден
	ErrNotFound ErrorType = "NOT_FOUND"
	// ErrAlreadyExists ресурс уже существует
	ErrAlreadyExists ErrorType = "ALREADY_EXISTS"
	// ErrInvalidInput некорректные входные данные
	ErrInvalidInput ErrorType = "INVALID_INPUT"
	// ErrUnauthorized не авторизован
	ErrUnauthorized ErrorType = "UNAUTHORIZED"
	// ErrForbidden доступ запрещён
	ErrForbidden ErrorType = "FORBIDDEN"
	// ErrInternal внутренняя ошибка
	ErrInternal ErrorType = "INTERNAL"
	// ErrTimeout таймаут операции
	ErrTimeout ErrorType = "TIMEOUT"
	// ErrUnavailable сервис недоступен
	ErrUnavailable ErrorType = "UNAVAILABLE"
)

// AppError структурированная ошибка приложения
type AppError struct {
	Type    ErrorType
	Message string
	Err     error
}

// Error реализует интерфейс error
func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Type, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s] %s", e.Type, e.Message)
}

// Unwrap возвращает обёрнутую ошибку для поддержки errors.Is/errors.As
func (e *AppError) Unwrap() error {
	return e.Err
}

// New создаёт новую ошибку приложения
func New(errType ErrorType, message string) *AppError {
	return &AppError{
		Type:    errType,
		Message: message,
	}
}

// Wrap оборачивает существующую ошибку с контекстом
func Wrap(errType ErrorType, message string, err error) *AppError {
	return &AppError{
		Type:    errType,
		Message: message,
		Err:     err,
	}
}

// NotFound создаёт ошибку "не найдено"
func NotFound(resource string, id string) *AppError {
	return &AppError{
		Type:    ErrNotFound,
		Message: fmt.Sprintf("%s не найден: %s", resource, id),
	}
}

// AlreadyExists создаёт ошибку "уже существует"
func AlreadyExists(resource string, id string) *AppError {
	return &AppError{
		Type:    ErrAlreadyExists,
		Message: fmt.Sprintf("%s уже существует: %s", resource, id),
	}
}

// InvalidInput создаёт ошибку некорректных данных
func InvalidInput(details string) *AppError {
	return &AppError{
		Type:    ErrInvalidInput,
		Message: fmt.Sprintf("некорректные данные: %s", details),
	}
}

// Unauthorized создаёт ошибку неавторизованного доступа
func Unauthorized(details string) *AppError {
	return &AppError{
		Type:    ErrUnauthorized,
		Message: fmt.Sprintf("не авторизован: %s", details),
	}
}

// Forbidden создаёт ошибку запрещённого доступа
func Forbidden(details string) *AppError {
	return &AppError{
		Type:    ErrForbidden,
		Message: fmt.Sprintf("доступ запрещён: %s", details),
	}
}

// Internal создаёт ошибку внутренней ошибки сервера
func Internal(details string, err error) *AppError {
	return &AppError{
		Type:    ErrInternal,
		Message: fmt.Sprintf("внутренняя ошибка: %s", details),
		Err:     err,
	}
}

// Timeout создаёт ошибку таймаута
func Timeout(operation string) *AppError {
	return &AppError{
		Type:    ErrTimeout,
		Message: fmt.Sprintf("таймаут операции: %s", operation),
	}
}

// Unavailable создаёт ошибку недоступности сервиса
func Unavailable(service string) *AppError {
	return &AppError{
		Type:    ErrUnavailable,
		Message: fmt.Sprintf("сервис недоступен: %s", service),
	}
}

// IsNotFound проверяет, является ли ошибка типом NOT_FOUND
func IsNotFound(err error) bool {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Type == ErrNotFound
	}
	return false
}

// IsAlreadyExists проверяет, является ли ошибка типом ALREADY_EXISTS
func IsAlreadyExists(err error) bool {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Type == ErrAlreadyExists
	}
	return false
}

// IsInvalidInput проверяет, является ли ошибка типом INVALID_INPUT
func IsInvalidInput(err error) bool {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Type == ErrInvalidInput
	}
	return false
}

// IsUnauthorized проверяет, является ли ошибка типом UNAUTHORIZED
func IsUnauthorized(err error) bool {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Type == ErrUnauthorized
	}
	return false
}

// IsTimeout проверяет, является ли ошибка типом TIMEOUT
func IsTimeout(err error) bool {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Type == ErrTimeout
	}
	return false
}

// IsForbidden проверяет, является ли ошибка типом FORBIDDEN
func IsForbidden(err error) bool {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Type == ErrForbidden
	}
	return false
}

// IsInternal проверяет, является ли ошибка типом INTERNAL
func IsInternal(err error) bool {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Type == ErrInternal
	}
	return false
}

// IsUnavailable проверяет, является ли ошибка типом UNAVAILABLE
func IsUnavailable(err error) bool {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Type == ErrUnavailable
	}
	return false
}
