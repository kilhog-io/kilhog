package kilhog

import (
	"fmt"
	"net/http"
	"strings"
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

// gatewayErrorMessage builds a client error when the body is not a kilhog API
// envelope. A 403 of that shape usually means a load balancer or WAF (for
// example Cloud Armor) rejected the request before it reached kilhog.
func gatewayErrorMessage(statusCode int, raw []byte) string {
	message := strings.TrimSpace(string(raw))
	if message == "" {
		message = http.StatusText(statusCode)
	}
	if statusCode != http.StatusForbidden {
		return message
	}
	return message + "; request was blocked before reaching the kilhog API " +
		"(load balancer or WAF). If Cloud Armor is enabled, set jsonParsing=STANDARD, " +
		"SQLi sensitivity 1, and exclude IPAM JSON fields (name, description, address, " +
		"prefix, type, tags) from sqli-v33-stable — hyphenated names match CRS 942432"
}

func statusFromCode(code int) int {
	if code >= http.StatusContinue && code <= http.StatusNetworkAuthenticationRequired {
		return code
	}
	return http.StatusInternalServerError
}
