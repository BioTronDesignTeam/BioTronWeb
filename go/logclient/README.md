# Go logging client

Import `github.com/BioTronDesignTeam/biotron/go/logclient` from BioTron Go services.
The client reads:

- `LOGGER_URL` — defaults to `http://logger-api:8080`
- `LOGGER_INGEST_TOKEN` — shared ingestion secret
- `LOGGER_SERVICE` — canonical component id from Logger's catalog
- `LOG_LEVEL` — `debug`, `info`, `warning`, or `error`; defaults to `info`

`LOG_LEVEL` is enforced before an HTTP request is made:

```go
client, err := logger.NewFromEnv()
if err != nil {
    log.Fatal(err)
}

_ = client.LogInfo(ctx, "telemetry batch stored", map[string]any{
    "samples": len(samples),
})
```

Keep calls to all four methods in the application. Change `LOG_LEVEL` per
container instead of removing debug calls from source.
