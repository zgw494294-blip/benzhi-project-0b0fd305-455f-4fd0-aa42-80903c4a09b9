package application

import "fmt"

type Error struct {
	Code    string
	Message string
	Version uint64
}

func (e *Error) Error() string { return e.Message }
func NewError(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}
