package apperr

import (
	"errors"
	"net/http"
	"strings"
)

type Code string

const (
	CodeInvalidArgument    Code = "INVALID_ARGUMENT"
	CodeNotFound           Code = "NOT_FOUND"
	CodeConflict           Code = "CONFLICT"
	CodeFailedPrecondition Code = "FAILED_PRECONDITION"
	CodeResourceExhausted  Code = "RESOURCE_EXHAUSTED"
	CodeUnsupportedMedia   Code = "UNSUPPORTED_MEDIA_TYPE"
	CodeUnavailable        Code = "UNAVAILABLE"
	CodeInternal           Code = "INTERNAL"
)

type Error struct {
	Code    Code
	Message string
	Err     error
}

func (e *Error) Error() string {
	if message := strings.TrimSpace(e.Message); message != "" {
		return message
	}
	if e.Err != nil {
		return strings.TrimSpace(e.Err.Error())
	}
	return string(e.Code)
}

func (e *Error) Unwrap() error {
	return e.Err
}

func New(code Code, message string) error {
	return &Error{
		Code:    code,
		Message: strings.TrimSpace(message),
	}
}

func Wrap(code Code, message string, err error) error {
	if err == nil {
		return nil
	}

	var appErr *Error
	if errors.As(err, &appErr) {
		if strings.TrimSpace(message) == "" || appErr.Message == strings.TrimSpace(message) {
			return err
		}
	}

	return &Error{
		Code:    code,
		Message: firstNonEmpty(message, err.Error()),
		Err:     err,
	}
}

func CodeOf(err error) Code {
	var appErr *Error
	if errors.As(err, &appErr) {
		return appErr.Code
	}
	if err == nil {
		return ""
	}
	return CodeInternal
}

func Message(err error) string {
	var appErr *Error
	if errors.As(err, &appErr) {
		return appErr.Error()
	}
	if err == nil {
		return ""
	}
	return strings.TrimSpace(err.Error())
}

func HTTPStatus(err error) int {
	switch CodeOf(err) {
	case CodeInvalidArgument:
		return http.StatusBadRequest
	case CodeNotFound:
		return http.StatusNotFound
	case CodeConflict:
		return http.StatusConflict
	case CodeFailedPrecondition:
		return http.StatusPreconditionFailed
	case CodeResourceExhausted:
		return http.StatusRequestEntityTooLarge
	case CodeUnsupportedMedia:
		return http.StatusUnsupportedMediaType
	case CodeUnavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

func IsCode(err error, code Code) bool {
	return CodeOf(err) == code
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
