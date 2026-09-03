package kilhog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultBaseURL   = "http://localhost:8080"
	defaultUserAgent = "kilhog-go-sdk"
	defaultTimeout   = 30 * time.Second
	envBaseURL       = "KILHOG_BASE_URL"
	envAPIKey        = "KILHOG_API_KEY"

	httpMethodGet    = "GET"
	httpMethodPost   = "POST"
	httpMethodPut    = "PUT"
	httpMethodDelete = "DELETE"
)

// ClientConfig holds connection settings for the kilhog API.
type ClientConfig struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
	UserAgent  string
}

// Client is an HTTP client for the kilhog REST API.
// It is intended for reuse by pogig, the Terraform provider, and other Go consumers.
type Client struct {
	baseURL    *url.URL
	apiKey     string
	httpClient *http.Client
	userAgent  string
}

// NewClient builds a Client from cfg. Missing fields receive sensible defaults.
func NewClient(cfg ClientConfig) (*Client, error) {
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse base URL failed: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("base URL must include scheme and host")
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultTimeout}
	}

	userAgent := strings.TrimSpace(cfg.UserAgent)
	if userAgent == "" {
		userAgent = defaultUserAgent
	}

	return &Client{
		baseURL:    parsed,
		apiKey:     cfg.APIKey,
		httpClient: httpClient,
		userAgent:  userAgent,
	}, nil
}

// NewClientFromEnv builds a Client using KILHOG_BASE_URL and KILHOG_API_KEY.
func NewClientFromEnv() (*Client, error) {
	return NewClient(ClientConfig{
		BaseURL: envOrDefault(envBaseURL, defaultBaseURL),
		APIKey:  strings.TrimSpace(getenv(envAPIKey)),
	})
}

type successEnvelope struct {
	Status string          `json:"status"`
	Data   json.RawMessage `json:"data"`
}

type errorEnvelope struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Code    int    `json:"code"`
}

func (c *Client) do(ctx context.Context, method, path string, reqBody any, respData any) error {
	var body io.Reader
	if reqBody != nil {
		payload, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("marshal request body failed: %w", err)
		}
		body = bytes.NewReader(payload)
	}

	reqURL := c.baseURL.ResolveReference(&url.URL{Path: strings.TrimPrefix(path, "/")})
	req, err := http.NewRequestWithContext(ctx, method, reqURL.String(), body)
	if err != nil {
		return fmt.Errorf("create request failed: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body failed: %w", err)
	}

	if resp.StatusCode >= http.StatusBadRequest {
		var envelope errorEnvelope
		if err := json.Unmarshal(raw, &envelope); err == nil && envelope.Message != "" {
			code := envelope.Code
			if code == 0 {
				code = resp.StatusCode
			}
			return newAPIError(statusFromCode(code), envelope.Message)
		}
		return newAPIError(resp.StatusCode, gatewayErrorMessage(resp.StatusCode, raw))
	}

	if respData == nil {
		return nil
	}

	var envelope successEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("decode success envelope failed: %w", err)
	}
	if envelope.Status != "success" {
		return fmt.Errorf("unexpected response status %q", envelope.Status)
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return nil
	}
	if err := json.Unmarshal(envelope.Data, respData); err != nil {
		return fmt.Errorf("decode response data failed: %w", err)
	}

	return nil
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(getenv(key)); value != "" {
		return value
	}
	return fallback
}

func getenv(key string) string {
	value, ok := lookupEnv(key)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}
