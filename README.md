# Appointment Scheduling API

An HTTP JSON API in Go that lets clients book 30-minute video calls with their trainer.

- Appointments are 30 minutes and start at :00 or :30.
- Bookings are allowed Monday–Friday, 8am–5pm Pacific. The last slot is 4:30–5:00pm.
- A trainer's appointments can't overlap.

## Quick start

Requires Docker with Compose.

```sh
docker compose up --build
curl localhost:8080/healthz
# {"status":"ok"}
```

The app waits for Postgres to be healthy, applies the schema, loads `appointments.json` (see [Seeding](#seeding)), then listens on port 8080. Stop with Ctrl-C, or run `docker compose down -v` to also delete the database volume.

If ports 8080 or 5432 are taken, override them:

```sh
APP_PORT=18080 POSTGRES_PORT=55432 docker compose up --build
```

## API

All times are [RFC 3339](https://www.rfc-editor.org/rfc/rfc3339). Input may use any offset; responses are always UTC (`Z`). All IDs are positive integers.

| Method | Path | Success |
| --- | --- | --- |
| GET | `/trainers/{trainer_id}/availability?starts_at=&ends_at=` | 200 |
| POST | `/appointments` | 201 |
| GET | `/trainers/{trainer_id}/appointments` | 200 |
| GET | `/healthz` | 200 |

### Examples

The examples use trainer 1001 and Monday, December 7, 2026. Substitute any future weekday.

**Book an appointment** (9:00am PST is 17:00Z):

```sh
curl -X POST localhost:8080/appointments -d '{
  "trainer_id": 1001,
  "user_id": 2001,
  "starts_at": "2026-12-07T17:00:00Z",
  "ends_at": "2026-12-07T17:30:00Z"
}'
```

```json
{"id":11,"trainer_id":1001,"user_id":2001,"starts_at":"2026-12-07T17:00:00Z","ends_at":"2026-12-07T17:30:00Z"}
```

Sending the same request again returns `409` with `{"error":"appointment overlaps an existing appointment"}`.

**Get available times** between 8:30 and 10:00am PST:

```sh
curl "localhost:8080/trainers/1001/availability?starts_at=2026-12-07T16:30:00Z&ends_at=2026-12-07T18:00:00Z"
```

```json
{"available_times":[{"starts_at":"2026-12-07T16:30:00Z","ends_at":"2026-12-07T17:00:00Z"},{"starts_at":"2026-12-07T17:30:00Z","ends_at":"2026-12-07T18:00:00Z"}]}
```

**List a trainer's appointments:**

```sh
curl localhost:8080/trainers/1001/appointments
```

```json
{"appointments":[{"id":11,"trainer_id":1001,"user_id":2001,"starts_at":"2026-12-07T17:00:00Z","ends_at":"2026-12-07T17:30:00Z"}]}
```

> **Offsets in query strings:** an unencoded `+` becomes a space, so `+05:00` fails to parse. Use `Z`, a negative offset like `-08:00`, or URL-encode `+` as `%2B`.

### Errors

Errors return `{"error": "<message>"}`.

| Status | When |
| --- | --- |
| 400 | Malformed JSON, unknown or duplicate fields, invalid ID or timestamp, missing field, `starts_at` not before `ends_at`, availability range over 30 days |
| 409 | The trainer already has an overlapping appointment |
| 413 | Request body over 1 MB |
| 422 | Not 30 minutes, not on :00/:30, outside business hours, or in the past |
| 500 | Unexpected failure; details are logged, not returned |

### Request collection

The `bruno/` directory has a [Bruno](https://www.usebruno.com/) collection covering the happy path and error cases, with tests. See [docs/BRUNO.md](docs/BRUNO.md). With the stack running:

```sh
cd bruno && npx @usebruno/cli run --env local
```

## Seeding

On startup the app imports the appointments in `appointments.json`: trainer 1 has 5, trainer 2 has 2, and trainer 3 has 3.

```sh
curl localhost:8080/trainers/1/appointments
```

- **How it's wired:** Compose mounts the file read-only at `/seed/appointments.json` and sets `SEED_FILE` to that path. Without `SEED_FILE`, nothing is imported. Edits to the file take effect on the next restart, without a rebuild.
- **Safe to rerun:** each row keeps its ID from the file, and rows that already exist are skipped, so restarts never duplicate data. New bookings get IDs after the highest seeded one (11 onward).
- **Field names:** the file uses `started_at`/`ended_at`; the API uses `starts_at`/`ends_at`.
- **Failures:** a missing or malformed file stops startup, since that's a configuration mistake.

## Design decisions and assumptions

1. **UTC storage, Pacific rules.** Times are stored as `timestamptz` and returned in UTC. They're converted to `America/Los_Angeles` only to check business hours, which keeps daylight saving correct (8am Pacific is 16:00Z in winter and 15:00Z in summer). Time-zone data is embedded in the binary, so it works in a minimal image.
2. **The database prevents overlaps.** A Postgres exclusion constraint on `(trainer_id, tstzrange(starts_at, ends_at))` makes double booking impossible even under concurrent requests. Adjacent slots (10:00–10:30 and 10:30–11:00) don't conflict.
3. **Availability:**
   - The range is half-open, `[starts_at, ends_at)`, and only slots that fit entirely inside it are returned.
   - Slots that have started or passed are hidden.
   - Ranges are capped at 30 days to bound response size.
4. **Booking requires `ends_at`,** and it must equal `starts_at` plus 30 minutes. This keeps the request shape from the prompt while enforcing the fixed duration.
5. **IDs are positive integers,** matching `appointments.json`. There are no trainer or user tables, so unknown IDs are accepted. Appointment IDs are generated by Postgres.
6. **Schema setup is a single idempotent SQL file** embedded in the binary and applied on startup. A migration tool wasn't worth it for one table.
7. **Seeded appointments are imported even when they break booking rules.** Five of the ten rows are on a Saturday, and one of those is an hour long. They're existing appointments, so dropping them would lose real bookings. The rules govern new bookings only.
   - Each violation is logged as a warning at startup.
   - Imported rows still block availability wherever they overlap, and still count toward overlap checks.
   - Rows are skipped only if their times are unusable (start not before end), their IDs aren't positive, or they overlap a row already imported for that trainer.
   - The seed data is from January 2019, so it never affects current availability.

## Project layout

```
main.go                       config, wiring, graceful shutdown
internal/
├── scheduling/               slot rules and slot generation (pure, no I/O)
├── appointment/              service: availability and booking
├── store/                    Postgres store and embedded schema
│   └── storetest/            in-memory fake for tests
├── seed/                     appointments.json import
└── httpapi/                  router, handlers, JSON and error mapping
appointments.json             seed data
bruno/                        Bruno request collection
```

Requests flow `httpapi → appointment → store`. The service depends on a small store interface and an injected clock, so it's tested without a database.

## Development

Requires Go 1.27 or later (uses the standard library `encoding/json/v2` package).

```sh
# Unit tests (no database needed; store tests are skipped)
go test ./...

# Including store integration tests against the compose Postgres
docker compose up -d postgres
TEST_DATABASE_URL='postgres://scheduler:scheduler@localhost:5432/scheduler' go test ./...

# Run the app locally
DATABASE_URL='postgres://scheduler:scheduler@localhost:5432/scheduler?sslmode=disable' go run .
```

| Variable | Default | Purpose |
| --- | --- | --- |
| `DATABASE_URL` | none (required) | Postgres connection string |
| `PORT` | `8080` | HTTP listen port |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, or `error` |
| `SEED_FILE` | none | Path to an appointments JSON file to import on startup |
