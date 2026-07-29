package log

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPMiddleware_Info(t *testing.T) {
	SetLevel(LevelInfo)

	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))

	handler := HTTPMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"status":"success"}`))
	}))

	req := httptest.NewRequest(http.MethodPost, "/networks", strings.NewReader(`{"name":"lab"}`))
	req.Header.Set("Authorization", "Bearer secret-key")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	output := buf.String()
	if !strings.Contains(output, "http request") {
		t.Fatalf("expected http request log, got %q", output)
	}
	if !strings.Contains(output, "POST") || !strings.Contains(output, "/networks") {
		t.Fatalf("expected method and path in log, got %q", output)
	}
	if strings.Contains(output, "secret-key") {
		t.Fatalf("API key must not appear in info logs, got %q", output)
	}
	if strings.Contains(output, "request_body") || strings.Contains(output, "response_body") {
		t.Fatalf("info log must not include bodies, got %q", output)
	}

	SetLevel(LevelInfo)
	slog.SetDefault(defaultLogger)
}

func TestHTTPMiddleware_Debug(t *testing.T) {
	SetLevel(LevelDebug)

	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))

	handler := HTTPMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"name":"lab"}` {
			t.Fatalf("request body = %q", body)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"status":"success","data":{}}`))
	}))

	req := httptest.NewRequest(http.MethodPost, "/networks", strings.NewReader(`{"name":"lab"}`))
	req.Header.Set("Authorization", "Bearer secret-key")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	output := buf.String()
	if !strings.Contains(output, "request_body") {
		t.Fatalf("expected request_body in debug log, got %q", output)
	}
	if !strings.Contains(output, "response_body") {
		t.Fatalf("expected response_body in debug log, got %q", output)
	}
	if !strings.Contains(output, "[REDACTED]") {
		t.Fatalf("expected redacted authorization header, got %q", output)
	}
	if strings.Contains(output, "secret-key") {
		t.Fatalf("API key must not appear in debug logs, got %q", output)
	}

	SetLevel(LevelInfo)
	slog.SetDefault(defaultLogger)
}

func TestRedactHeaders(t *testing.T) {
	headers := http.Header{}
	headers.Set("Authorization", "Bearer secret")
	headers.Set("X-API-Key", "also-secret")
	headers.Set("Content-Type", "application/json")

	redacted := redactHeaders(headers)
	if redacted["Authorization"] != "[REDACTED]" {
		t.Fatalf("Authorization = %q", redacted["Authorization"])
	}
	foundRedactedAPIKey := false
	for key, value := range redacted {
		if strings.EqualFold(key, "X-API-Key") {
			if value != "[REDACTED]" {
				t.Fatalf("%s = %q", key, value)
			}
			foundRedactedAPIKey = true
		}
	}
	if !foundRedactedAPIKey {
		t.Fatal("X-API-Key header not found in redacted headers")
	}
	if redacted["Content-Type"] != "application/json" {
		t.Fatalf("Content-Type = %q", redacted["Content-Type"])
	}
}
