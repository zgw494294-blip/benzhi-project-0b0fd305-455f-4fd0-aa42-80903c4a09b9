package domain

import "fmt"

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Message }

func NewError(code, format string, args ...any) error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

func ErrorCode(err error) string {
	if e, ok := err.(*Error); ok {
		return e.Code
	}
	return "internal_error"
}
