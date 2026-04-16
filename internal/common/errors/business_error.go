package errors

import "fmt"

type BusinessError struct {
	Code    ErrorCode
	Message string
	Cause   error
}

func NewBusinessError(code ErrorCode, message string, cause error) *BusinessError {
	return &BusinessError{
		Code:    code,
		Message: message,
		Cause:   cause,
	}
}

func (e *BusinessError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (e *BusinessError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func (e *BusinessError) String() string {
	return fmt.Sprintf("%d:%s", e.Code, e.Message)
}
