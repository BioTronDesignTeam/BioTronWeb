# logclient

The Go client BioTron services use to send events to Logger and, when
configured, to Grafana Cloud and PostHog. Import
`github.com/BioTronDesignTeam/BioTronWeb/go/logclient`. Each backend's `go.mod`
replaces that path with `../../../go/logclient`, and `go.work` lists it, so
one copy serves every service and a change needs no version bump.

The service name is passed in code so that the binary and its Logger
catalog id cannot drift apart:

```go
events := logclient.NewFromEnv("exo-api")
events.LogAsync(logclient.Info, "Exo API started", map[string]any{"port": port})
if err := events.Log(ctx, logclient.Info, "Exo API stopping", nil); err != nil {
	log.Printf("final log event failed: %v", err)
}
if err := events.Close(ctx); err != nil {
	log.Printf("log export flush failed: %v", err)
}
```

`Log` sends one event to Logger and waits. `LogAsync` returns at once: it
sends in a goroutine with a two-second timeout and at most 32 events in
flight, and past that it drops the event and says so on the standard log.
Events below `LOG_LEVEL` are dropped before any request. Keep debug calls in
the code and lower `LOG_LEVEL` per container instead.

Grafana and PostHog are never waited on, not even by `Log`. Each has one
background queue of 1024 events, sent in batches of up to 100 every second
with a five-second timeout. A full queue drops the event; a failed batch is
dropped, not retried, and the failure is said once on the standard log until
the destination recovers. A destination that is down can never fail or stall
a request. Call `Close` once at shutdown, after the last event, so the queues
are sent before the process exits.

## Variables

Each destination is switched on by its own variables and is a no-op without
them. `Enabled` reports whether any destination is on; `LoggerEnabled`
reports Logger alone, for the startup warning about `LOGGER_INGEST_TOKEN`.

| Variable | Destination | Meaning |
|---|---|---|
| `LOGGER_URL` | Logger | Default `http://logger-api:8080`. |
| `LOGGER_INGEST_TOKEN` | Logger | The ingest secret every sender shares. Empty disables Logger. |
| `LOG_LEVEL` | Logger, Grafana | `debug`, `info`, `warning`, or `error`; default `info`. An invalid value logs a warning and means `info`. |
| `GRAFANA_OTLP_ENDPOINT` | Grafana | The stack's OTLP gateway, such as `https://otlp-gateway-prod-us-east-0.grafana.net/otlp`. `/v1/logs` is appended. |
| `GRAFANA_OTLP_USER` | Grafana | The stack's instance id (a number), from the OTLP page of the Grafana Cloud portal. |
| `GRAFANA_OTLP_TOKEN` | Grafana | A Cloud Access Policy token with the `logs:write` scope. Secret. |
| `POSTHOG_API_KEY` | PostHog | The project API key (`phc_...`). Empty disables PostHog. Keep it out of logs like any key. |
| `POSTHOG_HOST` | PostHog | Default `https://us.i.posthog.com`; an EU project sets `https://eu.i.posthog.com`. |
| `BIOTRON_ENV` | Grafana, PostHog | Optional: `staging` or `production`. Tags each event with the environment that sent it, since both environments share one Grafana stack and PostHog project. |

Grafana needs all three `GRAFANA_OTLP_*` variables. With only some set it
stays off and warns, naming the missing variables but never a value.

## What goes where

- **Logger** gets every event at or above `LOG_LEVEL`, unchanged.
- **Grafana** gets the same events as Logger, as OTLP log records through
  the OTLP gateway into Loki. `service.name` becomes the `service_name`
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
  - never warnings or errors, `httplog`/`fiberlog` request lines, or events
    with no person behind them.

  The payload goes to PostHog as it is, so the rule that holds for Logger
  holds here: never put a session, cookie, token, or key in a payload.

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

`fiberlog` is the same middleware for Fiber. It stays until every backend has
moved to `net/http`, then goes.

## Moving off Logger

Logger stays the source of truth until Grafana and PostHog are checked.

1. Create a Grafana Cloud stack and an access policy token with
   `logs:write`, and a PostHog project. Put the values in each app's `.env`
   on staging (and the deploy secrets), with `BIOTRON_ENV=staging`.
2. Pass the variables through every compose file below and redeploy staging.
   Logger keeps receiving everything, unchanged.
3. Check Grafana Explore shows each service's events, and PostHog shows
   sign-ins and admin changes. Then do the same on production.
4. Once both are verified, unset `LOGGER_INGEST_TOKEN` in the services, then
   retire the Logger app. Its status page and monitor need a replacement
   (Grafana alerting or synthetic checks) before it goes; Logger's own events
   are written to its store and never reach Grafana.

## Compose wiring

Each Go service that calls `NewFromEnv` must pass these through, beside its
existing `LOGGER_*` and `LOG_LEVEL` entries, all optional:

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
| `apps/sprinter/docker-compose.yml` | `sprinter-bot` |

`apps/logger` writes its own events through its store, not this client, so
it needs none of them. `apps/site` has no Go service. Add the same names,
empty, to each of those apps' `.env.example`.
