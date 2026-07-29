package service

import "fmt"

// UserError wraps a sentinel error with a client-facing message.
type UserError struct {
	Err     error
	Message string
}

func (e *UserError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return e.Err.Error()
}

func (e *UserError) Unwrap() error {
	return e.Err
}

func userError(err error, format string, args ...any) error {
	return &UserError{
		Err:     err,
		Message: fmt.Sprintf(format, args...),
	}
}
