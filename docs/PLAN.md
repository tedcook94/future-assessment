# Implementation Plan

Appointment scheduling API in Go: clients book 30-minute video calls with trainers.

## Requirements

- Clients pick from available times; a trainer's appointments can't overlap.
- Every appointment is 30 minutes and starts at :00 or :30.
- Bookable only Monday–Friday, 8am–5pm Pacific.
- Endpoints:
  1. GET available times for a trainer between two dates (`trainer_id`, `starts_at`, `ends_at`).
  2. POST a new appointment (`trainer_id`, `user_id`, `starts_at`, `ends_at`).
  3. GET scheduled appointments for a trainer (`trainer_id`).
- Load existing appointments from `appointments.json`.
- Postgres storage, Docker setup for the reviewer.

## Architecture

- Layering: `handler → service → store (interface)`.
- Service takes an injected `now func() time.Time` for testable clock.
- `slog` for logging; stdlib `net/http` with Go 1.22+ `ServeMux` patterns.
- `pgx/v5` with hand-written SQL (no sqlc).
- Migrations embedded in the binary; run on startup.
- `docker compose up` starts `postgres` and `app`; app migrates, seeds, then serves.

### Routes

| Method | Path | Notes |
| --- | --- | --- |
| GET | `/healthz` | Liveness |
| GET | `/trainers/{trainer_id}/availability?starts_at=&ends_at=` | Open slots |
| POST | `/appointments` | Book a slot |
| GET | `/trainers/{trainer_id}/appointments` | Scheduled appointments |

## Decisions

1. **Time storage:** UTC everywhere. `timestamptz` columns, pool session `timezone=UTC`.
2. **API time format:** RFC 3339. Input accepts any offset and is normalized to UTC; output is UTC (`Z`).
3. **Business rules:** evaluated by converting to `America/Los_Angeles` (DST-aware). Import `time/tzdata` so minimal images work. Last valid slot is 4:30–5:00pm Pacific.
4. **Overlap prevention:** Postgres exclusion constraint (`btree_gist`, `EXCLUDE USING gist (trainer_id WITH =, tstzrange(starts_at, ends_at) WITH &&)`). Violation maps to 409.
5. **Past times:** hidden from availability; booking in the past is rejected.
6. **Availability range:** capped at 30 days (400 if exceeded). Range is half-open `[starts_at, ends_at)`.
7. **POST `ends_at`:** required; must equal `starts_at + 30m` (422 otherwise).
8. **Trainers/users:** plain IDs, no tables, unknown IDs allowed.
9. **Seeding:** idempotent (`ON CONFLICT DO NOTHING`); invalid rows logged and skipped.

## Commits

Mark each `[x]` when complete.

### Phase A: core

- [ ] 1. **Scaffold:** `go mod init`, `main.go` with `slog`, `http.Server`, graceful shutdown (SIGINT/SIGTERM), `GET /healthz`, `.gitignore`.
- [ ] 2. **Scheduling rules:** Pure functions: slot alignment, business-hours check, slot generation over a range. Table tests incl. DST transitions, weekends, 5pm edge.
- [ ] 3. **Postgres store:** Embedded migration, `pgx` pool, store (`ListByTrainer`, `ListInRange`, `Create`), compose file with Postgres only.
- [ ] 4. **Service:** Availability = generated slots minus booked; booking validates then stores. Tests with a fake store.

### Phase B: API and delivery

- [ ] 5. **HTTP handlers:** JSON encode/decode, consistent error shape, 400/409/422 mapping, `httptest` tests.
- [ ] 6. **Seed:** load `appointments.json` on startup. Blocked until file arrives.
- [ ] 7. **Docker:** Multi-stage build to distroless, app waits on Postgres healthcheck.
- [ ] 8. **README:** Run instructions, `curl` examples, assumptions, future work.
