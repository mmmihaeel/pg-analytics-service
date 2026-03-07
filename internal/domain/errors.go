package domain

import "errors"

type ErrorCode string

const (
	ErrCodeInvalid      ErrorCode = "invalid_request"
	ErrCodeNotFound     ErrorCode = "not_found"
	ErrCodeUnauthorized ErrorCode = "unauthorized"
	ErrCodeConflict     ErrorCode = "conflict"
	ErrCodeUnavailable  ErrorCode = "service_unavailable"
	ErrCodeInternal     ErrorCode = "internal_error"
)

type AppError struct {
	Code    ErrorCode
	Message string
	Err     error
}

func (e *AppError) Error() string {
	if e.Err == nil {
		return e.Message
	}

	return e.Message + ": " + e.Err.Error()
}

func (e *AppError) Unwrap() error {
	return e.Err
}

func NewAppError(code ErrorCode, message string) *AppError {
	return &AppError{Code: code, Message: message}
}

func WrapAppError(code ErrorCode, message string, err error) *AppError {
	return &AppError{Code: code, Message: message, Err: err}
}

func IsAppErrorCode(err error, code ErrorCode) bool {
	var appErr *AppError
	if !errors.As(err, &appErr) {
		return false
	}

	return appErr.Code == code
}
