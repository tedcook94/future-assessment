package seed

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"future-assessment/internal/scheduling"
	"future-assessment/internal/store"
)

type Importer interface {
	Import(ctx context.Context, appts []store.Appointment) (int, error)
}

type record struct {
	ID        *int64     `json:"id"`
	TrainerID *int64     `json:"trainer_id"`
	UserID    *int64     `json:"user_id"`
	StartedAt *time.Time `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at"`
}

// Parse decodes a seed file. It fails on malformed JSON, unknown fields, and
// missing fields, but does not check booking rules.
func Parse(r io.Reader) ([]store.Appointment, error) {
	var records []record
	if err := json.UnmarshalRead(r, &records, json.RejectUnknownMembers(true)); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	appts := make([]store.Appointment, 0, len(records))
	for i, rec := range records {
		if rec.ID == nil || rec.TrainerID == nil || rec.UserID == nil || rec.StartedAt == nil || rec.EndedAt == nil {
			return nil, fmt.Errorf("row %d: id, trainer_id, user_id, started_at, and ended_at are required", i)
		}
		appts = append(appts, store.Appointment{
			ID:        *rec.ID,
			TrainerID: *rec.TrainerID,
			UserID:    *rec.UserID,
			StartsAt:  rec.StartedAt.UTC(),
			EndsAt:    rec.EndedAt.UTC(),
		})
	}
	return appts, nil
}

// Run imports the appointments in the file at path. Rows that break booking
// rules are imported with a warning, because they are existing appointments
// and the rules only govern new bookings. Rows with non-positive IDs or an
// empty time range are skipped with a warning.
func Run(ctx context.Context, imp Importer, path string, logger *slog.Logger) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open seed file: %w", err)
	}
	defer f.Close()

	appts, err := Parse(f)
	if err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}

	valid := make([]store.Appointment, 0, len(appts))
	for _, a := range appts {
		if a.ID <= 0 || a.TrainerID <= 0 || a.UserID <= 0 {
			logger.Warn("skipping seed row: IDs must be positive", "id", a.ID)
			continue
		}
		if !a.StartsAt.Before(a.EndsAt) {
			logger.Warn("skipping seed row: started_at must be before ended_at", "id", a.ID)
			continue
		}
		if err := scheduling.ValidateSlot(a.StartsAt, a.EndsAt); err != nil {
			logger.Warn("importing seed row that breaks booking rules", "id", a.ID, "err", err)
		}
		valid = append(valid, a)
	}

	inserted, err := imp.Import(ctx, valid)
	if err != nil {
		return err
	}

	logger.Info("seed complete", "file", path, "rows", len(appts), "inserted", inserted, "skipped", len(appts)-inserted)
	return nil
}
