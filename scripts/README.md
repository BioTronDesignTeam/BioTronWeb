# scripts

Dev/ops utilities. Plain shell — runnable anywhere with `bash` + `curl` (no Node
needed).

## mock_telemetry.sh

The dev stand-in for the ESP32 WiFi coprocessor: POSTs **batched** telemetry JSON
to the Fiber ingest endpoint at a configurable rate. Replaces the old Python dummy
server. Each POST is one batch from one machine in the confirmed wire shape —
`{ machine_id, samples: [ { sampled_at, seq, battery, left, right, mcu_status,
link_status } ] }` (see the message contract in the root `CLAUDE.md`).

```bash
./scripts/mock_telemetry.sh
INGEST_URL=http://localhost:8080/api/telemetry RATE_HZ=5 ./scripts/mock_telemetry.sh
MACHINE_ID=exo-002 RATE_HZ=2 BATCH=20 COUNT=50 ./scripts/mock_telemetry.sh  # 50 batches then stop
```

| Env | Default | Meaning |
|-----|---------|---------|
| `MACHINE_ID` | `exo-001` | the batch's `machine_id` tag |
| `INGEST_URL` | `http://localhost:8080/api/telemetry` | where to POST |
| `RATE_HZ` | `1` | POSTs (batches) per second |
| `BATCH` | `10` | samples per batch (`sampled_at` spaced ~100 Hz) |
| `COUNT` | `0` | stop after N batches (`0` = forever) |

Needs `bash`, `curl`, and `awk` (all standard on the Debian box and in the dev
container); `sampled_at` uses GNU `date +%s%6N` (microsecond epoch). Fine up to
~tens of Hz; for high-rate load testing a compiled sender (Go) scales better.
Point `INGEST_URL` at the real ingest route once the Fiber backend lands it.
