package store

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var schema string

const exclusionViolation = "23P01"

var ErrOverlap = errors.New("appointment overlaps an existing appointment")

type Appointment struct {
	ID        int64
	TrainerID int64
	UserID    int64
	StartsAt  time.Time
	EndsAt    time.Time
}

type Store struct {
	pool *pgxpool.Pool
}

func New(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() {
	s.pool.Close()
}

// Migrate applies the schema. It is safe to run on every startup.
func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.pool.Exec(ctx, schema); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}

// ListByTrainer returns all appointments for a trainer, ordered by start time.
func (s *Store) ListByTrainer(ctx context.Context, trainerID int64) ([]Appointment, error) {
	const q = `
		SELECT id, trainer_id, user_id, starts_at, ends_at
		FROM appointments
		WHERE trainer_id = $1
		ORDER BY starts_at`

	return s.query(ctx, q, trainerID)
}

// ListInRange returns a trainer's appointments that overlap [from, to),
// ordered by start time.
func (s *Store) ListInRange(ctx context.Context, trainerID int64, from, to time.Time) ([]Appointment, error) {
	const q = `
		SELECT id, trainer_id, user_id, starts_at, ends_at
		FROM appointments
		WHERE trainer_id = $1 AND starts_at < $3 AND ends_at > $2
		ORDER BY starts_at`

	return s.query(ctx, q, trainerID, from, to)
}

// Create inserts an appointment and returns it with its generated ID. It
// returns ErrOverlap if the trainer already has an overlapping appointment.
func (s *Store) Create(ctx context.Context, a Appointment) (Appointment, error) {
	const q = `
		INSERT INTO appointments (trainer_id, user_id, starts_at, ends_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id, trainer_id, user_id, starts_at, ends_at`

	created, err := scanAppointment(s.pool.QueryRow(ctx, q, a.TrainerID, a.UserID, a.StartsAt, a.EndsAt))
	if err != nil {
		if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == exclusionViolation {
			return Appointment{}, ErrOverlap
		}
		return Appointment{}, fmt.Errorf("insert appointment: %w", err)
	}
	return created, nil
}

func (s *Store) query(ctx context.Context, q string, args ...any) ([]Appointment, error) {
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query appointments: %w", err)
	}

	appts, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Appointment, error) {
		return scanAppointment(row)
	})
	if err != nil {
		return nil, fmt.Errorf("scan appointments: %w", err)
	}
	return appts, nil
}

// pgx returns timestamptz values in time.Local regardless of the session time
// zone, so normalize to UTC here.
func scanAppointment(row pgx.Row) (Appointment, error) {
	var a Appointment
	if err := row.Scan(&a.ID, &a.TrainerID, &a.UserID, &a.StartsAt, &a.EndsAt); err != nil {
		return Appointment{}, err
	}
	a.StartsAt = a.StartsAt.UTC()
	a.EndsAt = a.EndsAt.UTC()
	return a, nil
}
