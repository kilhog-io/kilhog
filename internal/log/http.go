package log

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const maxLoggedBodyBytes = 64 << 10 // 64 KiB

// HTTPMiddleware logs every HTTP request. At info level, only method, path, status,
// and duration are logged. At debug level, headers (API key redacted), request body,
// and response body are included.
func HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !Enabled(LevelInfo) {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		debug := Enabled(LevelDebug)

		var requestBody []byte
		if debug && r.Body != nil && r.Body != http.NoBody {
			requestBody, _ = io.ReadAll(io.LimitReader(r.Body, maxLoggedBodyBytes+1))
			r.Body = io.NopCloser(bytes.NewReader(requestBody))
		}

		capture := &responseCapture{ResponseWriter: w, status: http.StatusOK}
		if debug {
			capture.body = &bytes.Buffer{}
		}

		next.ServeHTTP(capture, r)

		duration := time.Since(start)
		path := r.URL.Path
		if r.URL.RawQuery != "" {
			path += "?" + r.URL.RawQuery
		}

		if debug {
			logHTTPDebug(r, path, capture.status, duration, requestBody, capture.body.Bytes())
			return
		}

		slog.Info("http request",
			"method", r.Method,
			"path", path,
			"status", capture.status,
			"duration", duration.Round(time.Microsecond).String(),
		)
	})
}

func logHTTPDebug(r *http.Request, path string, status int, duration time.Duration, requestBody, responseBody []byte) {
	attrs := []any{
		"method", r.Method,
		"path", path,
		"status", status,
		"duration", duration.Round(time.Microsecond).String(),
		"headers", redactHeaders(r.Header),
	}

	if len(requestBody) > 0 {
		attrs = append(attrs, "request_body", truncateBody(requestBody))
	}

	if len(responseBody) > 0 {
		attrs = append(attrs, "response_body", truncateBody(responseBody))
	}

	slog.Debug("http request", attrs...)
}

func redactHeaders(headers http.Header) map[string]string {
	out := make(map[string]string, len(headers))
	for key, values := range headers {
		lower := strings.ToLower(key)
		if lower == "authorization" || lower == "x-api-key" {
			out[key] = "[REDACTED]"
			continue
		}
		out[key] = strings.Join(values, ", ")
	}
	return out
}

func truncateBody(body []byte) string {
	if len(body) > maxLoggedBodyBytes {
		return string(body[:maxLoggedBodyBytes]) + "…[truncated]"
	}
	return string(body)
}

type responseCapture struct {
	http.ResponseWriter
	status int
	body   *bytes.Buffer
}

func (c *responseCapture) WriteHeader(status int) {
	c.status = status
	c.ResponseWriter.WriteHeader(status)
}

func (c *responseCapture) Write(b []byte) (int, error) {
	if c.body != nil {
		_, _ = c.body.Write(b)
	}
	return c.ResponseWriter.Write(b)
}
