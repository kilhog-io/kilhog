package kilhog

import (
	"fmt"
	"net/http"
)

// APIError is returned when the kilhog API responds with a non-success envelope.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("kilhog API error (HTTP %d): %s", e.StatusCode, e.Message)
}

func newAPIError(statusCode int, message string) *APIError {
	return &APIError{
		StatusCode: statusCode,
		Message:    message,
	}
}

func statusFromCode(code int) int {
	if code >= http.StatusContinue && code <= http.StatusNetworkAuthenticationRequired {
		return code
	}
	return http.StatusInternalServerError
}
