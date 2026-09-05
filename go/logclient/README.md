# logclient

The Go client BioTron services use to send events to Logger. Import
`github.com/BioTronDesignTeam/BioTronWeb/go/logclient`. Each backend's `go.mod`
replaces that path with `../../../go/logclient`, and `go.work` lists it, so
one copy serves every service and a change needs no version bump.

The client reads three variables:

- `LOGGER_URL`, default `http://logger-api:8080`.
- `LOGGER_INGEST_TOKEN`, the ingest secret every sender shares. Empty means
  send nothing, so a service runs unchanged where Logger is absent.
- `LOG_LEVEL`: `debug`, `info`, `warning`, or `error`; default `info`. An
  invalid value logs a warning and means `info`.

The service name is passed in code so that the binary and its Logger
catalog id cannot drift apart:

```go
events := logclient.NewFromEnv("exo-api")
events.LogAsync(logclient.Info, "Exo API started", map[string]any{"port": port})
if err := events.Log(ctx, logclient.Info, "Exo API stopping", nil); err != nil {
	log.Printf("final log event failed: %v", err)
}
```

`Log` sends one event and waits. `LogAsync` returns at once: it sends in a
goroutine with a two-second timeout and at most 32 events in flight, and
past that it drops the event and says so on the standard log. Events below
`LOG_LEVEL` are dropped before any request. Keep debug calls in the code
and lower `LOG_LEVEL` per container instead.
