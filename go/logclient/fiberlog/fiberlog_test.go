package fiberlog

import (
	"errors"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"

	"github.com/BioTronDesignTeam/biotron/go/logclient"
)

type event struct {
	level   logclient.Level
	message string
	payload map[string]any
}

type recorder struct{ events []event }

func (r *recorder) LogAsync(level logclient.Level, message string, payload any) {
	r.events = append(r.events, event{level, message, payload.(map[string]any)})
}

var errNotFound = errors.New("thing not found")

func newApp(sink Sink, options Options) *fiber.App {
	app := fiber.New(fiber.Config{
		ErrorHandler: func(c fiber.Ctx, err error) error {
			status := fiber.StatusInternalServerError
			var fiberError *fiber.Error
			if errors.As(err, &fiberError) {
				status = fiberError.Code
			} else if errors.Is(err, errNotFound) {
				status = fiber.StatusNotFound
			}
			return c.Status(status).SendString("mapped")
		},
	})
	app.Use(New(sink, options))
	app.Get("/health", func(c fiber.Ctx) error { return c.SendString("ok") })
	app.Get("/ok", func(c fiber.Ctx) error {
		SetActor(c, "octocat")
		return c.SendString("ok")
	})
	app.Get("/quiet", func(c fiber.Ctx) error { return c.SendString("ok") })
	app.Get("/quiet-fails", func(c fiber.Ctx) error { return fiber.ErrUnauthorized })
	app.Get("/missing", func(c fiber.Ctx) error { return errNotFound })
	app.Get("/broken", func(c fiber.Ctx) error { return errors.New("database on fire") })
	app.Get("/self-written", func(c fiber.Ctx) error {
		SetError(c, errors.New("upstream said no"))
		return c.Status(fiber.StatusBadGateway).SendString("no")
	})
	app.Options("/ok", func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) })
	return app
}

func get(t *testing.T, app *fiber.App, method, path string) (int, string) {
	t.Helper()
	response, err := app.Test(httptest.NewRequest(method, path, nil))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	return response.StatusCode, string(body)
}

func TestLogsStatusActorAndCause(t *testing.T) {
	sink := &recorder{}
	app := newApp(sink, Options{Quiet: []string{"/quiet", "/quiet-fails"}})

	get(t, app, "GET", "/health")
	get(t, app, "GET", "/quiet")
	get(t, app, "OPTIONS", "/ok")
	if len(sink.events) != 0 {
		t.Fatalf("health, quiet, and preflight must not log; got %+v", sink.events)
	}

	if status, _ := get(t, app, "GET", "/ok"); status != 200 {
		t.Fatalf("status = %d", status)
	}
	if status, _ := get(t, app, "GET", "/quiet-fails"); status != 401 {
		t.Fatalf("status = %d", status)
	}
	if status, body := get(t, app, "GET", "/missing"); status != 404 || body != "mapped" {
		t.Fatalf("status = %d body = %q; the app's error handler must run", status, body)
	}
	if status, _ := get(t, app, "GET", "/broken"); status != 500 {
		t.Fatalf("status = %d", status)
	}
	get(t, app, "GET", "/self-written")

	want := []struct {
		level  logclient.Level
		path   string
		status int
		actor  string
		cause  string
	}{
		{logclient.Info, "/ok", 200, "octocat", ""},
		{logclient.Warning, "/quiet-fails", 401, "", "Unauthorized"},
		{logclient.Warning, "/missing", 404, "", "thing not found"},
		{logclient.Error, "/broken", 500, "", "database on fire"},
		{logclient.Error, "/self-written", 502, "", "upstream said no"},
	}
	if len(sink.events) != len(want) {
		t.Fatalf("got %d events, want %d: %+v", len(sink.events), len(want), sink.events)
	}
	for i, w := range want {
		got := sink.events[i]
		if got.message != "HTTP request completed" || got.level != w.level || got.payload["path"] != w.path || got.payload["status"] != w.status {
			t.Errorf("event %d = %+v, want %+v", i, got, w)
		}
		if actor, _ := got.payload["actor"].(string); actor != w.actor {
			t.Errorf("event %d actor = %q, want %q", i, actor, w.actor)
		}
		if cause, _ := got.payload["error"].(string); cause != w.cause {
			t.Errorf("event %d error = %q, want %q", i, cause, w.cause)
		}
		if _, ok := got.payload["duration_ms"].(int64); !ok {
			t.Errorf("event %d has no duration", i)
		}
	}
}

func TestSkipDropsRefusedRequests(t *testing.T) {
	sink := &recorder{}
	app := newApp(sink, Options{Skip: func(c fiber.Ctx, status int) bool {
		return c.Path() == "/quiet-fails" && status == fiber.StatusUnauthorized
	}})
	get(t, app, "GET", "/quiet-fails")
	get(t, app, "GET", "/ok")
	if len(sink.events) != 1 || sink.events[0].payload["path"] != "/ok" {
		t.Fatalf("skip must drop the refused request only; got %+v", sink.events)
	}
}
