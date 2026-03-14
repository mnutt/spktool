package domain

import "fmt"

type ErrorCode string

const (
	ErrInvalidArgument ErrorCode = "invalid_argument"
	ErrNotFound        ErrorCode = "not_found"
	ErrConflict        ErrorCode = "conflict"
	ErrExternal        ErrorCode = "external"
	ErrWorkflow        ErrorCode = "workflow"
	ErrUnsupported     ErrorCode = "unsupported"
)

type Error struct {
	Code      ErrorCode `json:"code"`
	Op        string    `json:"op,omitempty"`
	Message   string    `json:"message"`
	Retryable bool      `json:"retryable,omitempty"`
	Cause     error     `json:"-"`
}

func (e *Error) Error() string {
	if e.Op == "" {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.Op, e.Message)
}

func (e *Error) Unwrap() error { return e.Cause }

func Wrap(code ErrorCode, op, msg string, cause error) *Error {
	return &Error{Code: code, Op: op, Message: msg, Cause: cause}
}
