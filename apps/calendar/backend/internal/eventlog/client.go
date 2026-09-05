package eventlog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
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

type Client struct {
	endpoint   string
	token      string
	service    string
	minimum    int
	httpClient *http.Client
	slots      chan struct{}
}

func NewFromEnv(service string) *Client {
	minimum, ok := severity(Level(strings.ToLower(getenv("LOG_LEVEL", string(Info)))))
	if !ok {
		log.Printf("warning: invalid LOG_LEVEL; defaulting to info")
		minimum, _ = severity(Info)
	}
	return &Client{
		endpoint:   strings.TrimRight(getenv("LOGGER_URL", "http://logger-api:8080"), "/") + "/v1/logs",
		token:      os.Getenv("LOGGER_INGEST_TOKEN"),
		service:    service,
		minimum:    minimum,
		httpClient: &http.Client{Timeout: 2 * time.Second},
		slots:      make(chan struct{}, 32),
	}
}

func (c *Client) Enabled() bool {
	return c != nil && c.token != ""
}

func (c *Client) Log(ctx context.Context, level Level, message string, payload any) error {
	if !c.Enabled() {
		return nil
	}
	current, ok := severity(level)
	if !ok {
		return fmt.Errorf("invalid log level %q", level)
	}
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

func (c *Client) LogAsync(level Level, message string, payload any) {
	if !c.Enabled() {
		return
	}
	select {
	case c.slots <- struct{}{}:
	default:
		log.Print("structured log delivery busy; dropping event")
		return
	}
	go func() {
		defer func() { <-c.slots }()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := c.Log(ctx, level, message, payload); err != nil {
			log.Printf("structured log delivery failed: %v", err)
		}
	}()
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
