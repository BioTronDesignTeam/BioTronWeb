# logclient

The Go client BioTron services use to send events to Grafana Cloud and
PostHog. Import
`github.com/BioTronDesignTeam/BioTronWeb/go/logclient`. Each backend's `go.mod`
replaces that path with `../../../go/logclient`, and `go.work` lists it, so
one copy serves every service and a change needs no version bump.

The service name is passed in code so that the binary and its Grafana
`service_name` label cannot drift apart:

```go
events := logclient.NewFromEnv("exo-api")
events.LogAsync(logclient.Info, "Exo API started", map[string]any{"port": port})
events.LogAsync(logclient.Info, "Exo API stopping", nil)
if err := events.Close(ctx); err != nil {
	log.Printf("log export flush failed: %v", err)
}
```

`LogAsync` returns at once: it hands the event to the background queues.
There is no waiting variant, because no destination answers per event.
Events below `LOG_LEVEL` are dropped before they are queued. Keep debug calls
in the code and lower `LOG_LEVEL` per container instead.

Grafana and PostHog are never waited on. Each has one
background queue of 1024 events, sent in batches of up to 100 every second
with a five-second timeout. A full queue drops the event; a failed batch is
dropped, not retried, and the failure is said once on the standard log until
the destination recovers. A destination that is down can never fail or stall
a request. Call `Close` once at shutdown, after the last event, so the queues
are sent before the process exits.

## Variables

Each destination is switched on by its own variables and is a no-op without
them. `Enabled` reports whether Grafana or PostHog is on; each service warns
at startup when neither is.

| Variable | Destination | Meaning |
|---|---|---|
| `LOG_LEVEL` | Grafana | `debug`, `info`, `warning`, or `error`; default `info`. An invalid value logs a warning and means `info`. |
| `GRAFANA_OTLP_ENDPOINT` | Grafana | The stack's OTLP gateway, such as `https://otlp-gateway-prod-us-east-0.grafana.net/otlp`. `/v1/logs` is appended. |
| `GRAFANA_OTLP_USER` | Grafana | The stack's instance id (a number), from the OTLP page of the Grafana Cloud portal. |
| `GRAFANA_OTLP_TOKEN` | Grafana | A Cloud Access Policy token with the `logs:write` scope. Secret. |
| `POSTHOG_API_KEY` | PostHog | The project API key (`phc_...`). Empty disables PostHog. Keep it out of logs like any key. |
| `POSTHOG_HOST` | PostHog | Default `https://us.i.posthog.com`; an EU project sets `https://eu.i.posthog.com`. |
| `BIOTRON_ENV` | Grafana, PostHog | Optional: `staging` or `production`. Tags each event with the environment that sent it, since both environments share one Grafana stack and PostHog project. |

Grafana needs all three `GRAFANA_OTLP_*` variables. With only some set it
stays off and warns, naming the missing variables but never a value.

## What goes where

- **Grafana** gets every event at or above `LOG_LEVEL`, as OTLP log records
  through the OTLP gateway into Loki. `service.name` becomes the `service_name`
  label; the message is the line, the level its severity, and each top-level
  payload field a structured-metadata attribute (nested values as JSON). Find
  them in Explore with `{service_name="exo-api"}`. The JSON body is written
  by hand rather than with the OpenTelemetry SDK so that every service image
  does not pull in protobuf and the otel modules for one POST; `grafana.go`
  says more.
- **PostHog** gets product and audit events, not logs:
  - every `Info` event whose payload is a `map[string]any` with a non-empty
    `actor_login` (admin and audit changes, such as Auth's `audit` and
    Sprinter's `adminEvent`) or `operator_login` (Auth's sign-in and
    sign-out). The login is the distinct id, the message the event name, and
    the payload the properties, plus `service` and `environment`. These go
    whatever `LOG_LEVEL` says: they are a record of who did what.
  - anything sent with `Capture(distinctID, event, properties)`, which goes
    to PostHog only. An empty distinct id means the service itself acted, and
    PostHog makes no person profile for it.
  - never warnings or errors, `httplog` request lines, or events
    with no person behind them.

  The payload goes to PostHog as it is, so never put a session, cookie,
  token, or key in a payload.

## Request middleware

`httplog` is the `net/http` middleware that sends one event per request:
method, path, status, and duration, plus `actor` and `error` when the
handler sets them. Mount it outermost; it also recovers panics as a logged
500.

```go
handler = httplog.New(events, httplog.Options{Quiet: []string{"/api/session"}})(mux)
// in a handler:
httplog.SetActor(r, operator.Login)
httplog.SetError(r, err)
```

## Compose wiring

Each Go service that calls `NewFromEnv` must pass these through, beside its
`LOG_LEVEL` entry, all optional:

```yaml
      GRAFANA_OTLP_ENDPOINT: ${GRAFANA_OTLP_ENDPOINT:-}
      GRAFANA_OTLP_USER: ${GRAFANA_OTLP_USER:-}
      GRAFANA_OTLP_TOKEN: ${GRAFANA_OTLP_TOKEN:-}
      POSTHOG_HOST: ${POSTHOG_HOST:-}
      POSTHOG_API_KEY: ${POSTHOG_API_KEY:-}
      BIOTRON_ENV: ${BIOTRON_ENV:-}
```

| Compose file | Service |
|---|---|
| `apps/auth/docker-compose.yml` | `auth-api` |
| `apps/calendar/docker-compose.yml` | `calendar-api` |
| `apps/exo/docker-compose.yml` | `exo-api` |
| `apps/logger/docker-compose.yml` | `logger-api` |
| `apps/sprinter/docker-compose.yml` | `sprinter-bot` |

`apps/site` has no Go service. Add the same names,
empty, to each of those apps' `.env.example`.

## Tests

`logclienttest.New(t, service)` returns a real client and a channel of the
events it exports. It points the client at a stand-in OTLP gateway through
`t.Setenv`, so a test reads back what Grafana would receive instead of
reaching into the client.
