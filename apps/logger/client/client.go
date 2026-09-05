package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type Level string

const (
	Debug   Level = "debug"
	Info    Level = "info"
	Warning Level = "warning"
	Error   Level = "error"
)

type Config struct {
	URL        string
	Token      string
	Service    string
	Level      Level
	HTTPClient *http.Client
}

type Client struct {
	endpoint   string
	token      string
	service    string
	minimum    int
	httpClient *http.Client
}

func New(config Config) (*Client, error) {
	if config.URL == "" || config.Token == "" || config.Service == "" {
		return nil, errors.New("logger URL, token, and service are required")
	}
	minimum, ok := severity(config.Level)
	if !ok {
		return nil, fmt.Errorf("invalid log level %q", config.Level)
	}
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 2 * time.Second}
	}
	return &Client{
		endpoint:   strings.TrimRight(config.URL, "/") + "/v1/logs",
		token:      config.Token,
		service:    config.Service,
		minimum:    minimum,
		httpClient: httpClient,
	}, nil
}

func NewFromEnv() (*Client, error) {
	return New(Config{
		URL:     getenv("LOGGER_URL", "http://logger-api:8080"),
		Token:   os.Getenv("LOGGER_INGEST_TOKEN"),
		Service: os.Getenv("LOGGER_SERVICE"),
		Level:   Level(strings.ToLower(getenv("LOG_LEVEL", string(Info)))),
	})
}

func (c *Client) LogDebug(ctx context.Context, message string, payload any) error {
	return c.log(ctx, Debug, message, payload)
}

func (c *Client) LogInfo(ctx context.Context, message string, payload any) error {
	return c.log(ctx, Info, message, payload)
}

func (c *Client) LogWarning(ctx context.Context, message string, payload any) error {
	return c.log(ctx, Warning, message, payload)
}

func (c *Client) LogError(ctx context.Context, message string, payload any) error {
	return c.log(ctx, Error, message, payload)
}

func (c *Client) log(ctx context.Context, level Level, message string, payload any) error {
	current, _ := severity(level)
	if current < c.minimum {
		return nil
	}
	body := struct {
		Service string `json:"service"`
		Level   Level  `json:"level"`
		Message string `json:"message"`
		Payload any    `json:"payload,omitempty"`
	}{Service: c.service, Level: level, Message: message, Payload: payload}
	raw, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal log event: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+c.token)
	request.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("send log event: %w", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("logger returned HTTP %d", response.StatusCode)
	}
	return nil
}

func severity(level Level) (int, bool) {
	switch level {
	case Debug:
		return 0, true
	case Info:
		return 1, true
	case Warning:
		return 2, true
	case Error:
		return 3, true
	default:
		return 0, false
	}
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
