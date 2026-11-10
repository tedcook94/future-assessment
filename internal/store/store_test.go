package store_test

import (
	"context"
	"errors"
	"math"
	"math/rand/v2"
	"os"
	"testing"
	"time"

	"future-assessment/internal/store"
)

// newTestStore connects to TEST_DATABASE_URL and applies the schema. Tests are
// skipped when the variable is unset so `go test ./...` works without a
// database. Each test isolates its data by using random trainer IDs from
// newID, so no cleanup between tests is needed.
func newTestStore(t *testing.T) *store.Store {
	t.Helper()

	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	ctx := t.Context()
	s, err := store.New(ctx, url)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(s.Close)

	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return s
}

func at(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func newID() int64 {
	return rand.Int64N(math.MaxInt64) + 1
}

func appt(trainerID int64, start string) store.Appointment {
	startsAt := at(start)
	return store.Appointment{
		TrainerID: trainerID,
		UserID:    newID(),
		StartsAt:  startsAt,
		EndsAt:    startsAt.Add(30 * time.Minute),
	}
}

func mustCreate(t *testing.T, ctx context.Context, s *store.Store, a store.Appointment) store.Appointment {
	t.Helper()
	created, err := s.Create(ctx, a)
	if err != nil {
		t.Fatalf("Create(%v): %v", a.StartsAt, err)
	}
	return created
}

func startTimes(appts []store.Appointment) []time.Time {
	out := make([]time.Time, len(appts))
	for i, a := range appts {
		out[i] = a.StartsAt
	}
	return out
}

func TestCreateAndListByTrainer(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	trainer := newID()

	later := mustCreate(t, ctx, s, appt(trainer, "2026-11-09T19:00:00Z"))
	// Non-UTC input must come back as the same instant in UTC.
	earlier := mustCreate(t, ctx, s, appt(trainer, "2026-11-09T08:00:00-08:00"))
	mustCreate(t, ctx, s, appt(newID(), "2026-11-09T17:00:00Z"))

	if earlier.ID == 0 || earlier.ID == later.ID {
		t.Fatalf("IDs not generated: %v, %v", earlier.ID, later.ID)
	}

	got, err := s.ListByTrainer(ctx, trainer)
	if err != nil {
		t.Fatalf("ListByTrainer: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0] != earlier || got[1] != later {
		t.Errorf("got %+v, want [%+v %+v]", got, earlier, later)
	}
	if got[0].StartsAt.Location() != time.UTC {
		t.Errorf("StartsAt location = %v, want UTC", got[0].StartsAt.Location())
	}
	if !got[0].StartsAt.Equal(at("2026-11-09T16:00:00Z")) {
		t.Errorf("StartsAt = %v, want 16:00Z", got[0].StartsAt)
	}
}

func TestCreateRejectsOverlap(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	trainer := newID()

	mustCreate(t, ctx, s, appt(trainer, "2026-11-09T17:00:00Z"))

	tests := []struct {
		name  string
		start string
	}{
		{"same slot", "2026-11-09T17:00:00Z"},
		{"partial overlap", "2026-11-09T17:15:00Z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.Create(ctx, appt(trainer, tt.start))
			if !errors.Is(err, store.ErrOverlap) {
				t.Errorf("err = %v, want ErrOverlap", err)
			}
		})
	}
}

func TestCreateAllowsAdjacentSlots(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	trainer := newID()

	mustCreate(t, ctx, s, appt(trainer, "2026-11-09T17:00:00Z"))
	mustCreate(t, ctx, s, appt(trainer, "2026-11-09T17:30:00Z"))
	mustCreate(t, ctx, s, appt(trainer, "2026-11-09T16:30:00Z"))
}

func TestCreateAllowsSameSlotForDifferentTrainers(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	mustCreate(t, ctx, s, appt(newID(), "2026-11-09T17:00:00Z"))
	mustCreate(t, ctx, s, appt(newID(), "2026-11-09T17:00:00Z"))
}

func TestListInRange(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	trainer := newID()

	for _, start := range []string{
		"2026-11-09T16:00:00Z", // ends at range start: excluded
		"2026-11-09T16:30:00Z", // first slot in range
		"2026-11-09T17:30:00Z", // last slot in range
		"2026-11-09T18:00:00Z", // starts at range end: excluded
	} {
		mustCreate(t, ctx, s, appt(trainer, start))
	}
	mustCreate(t, ctx, s, appt(newID(), "2026-11-09T17:00:00Z"))

	got, err := s.ListInRange(ctx, trainer, at("2026-11-09T16:30:00Z"), at("2026-11-09T18:00:00Z"))
	if err != nil {
		t.Fatalf("ListInRange: %v", err)
	}

	want := []time.Time{at("2026-11-09T16:30:00Z"), at("2026-11-09T17:30:00Z")}
	gotStarts := startTimes(got)
	if len(gotStarts) != len(want) {
		t.Fatalf("starts = %v, want %v", gotStarts, want)
	}
	for i := range want {
		if !gotStarts[i].Equal(want[i]) {
			t.Errorf("starts[%d] = %v, want %v", i, gotStarts[i], want[i])
		}
	}
}

func TestListInRangeIncludesStraddlingAppointment(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	trainer := newID()

	mustCreate(t, ctx, s, store.Appointment{
		TrainerID: trainer,
		UserID:    newID(),
		StartsAt:  at("2026-11-09T16:15:00Z"),
		EndsAt:    at("2026-11-09T16:45:00Z"),
	})

	got, err := s.ListInRange(ctx, trainer, at("2026-11-09T16:30:00Z"), at("2026-11-09T17:00:00Z"))
	if err != nil {
		t.Fatalf("ListInRange: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
}

func TestImport(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	trainer, otherTrainer := newID(), newID()

	// Import takes explicit IDs, so build them from the newest generated ID to
	// avoid colliding with rows from other tests.
	last := mustCreate(t, ctx, s, appt(trainer, "2026-11-09T16:00:00Z")).ID

	first := appt(trainer, "2026-11-10T01:00:00Z")
	first.ID = last + 1
	overlapping := appt(trainer, "2026-11-10T01:15:00Z")
	overlapping.ID = last + 2
	duplicateID := appt(otherTrainer, "2026-11-09T17:00:00Z")
	duplicateID.ID = last
	rows := []store.Appointment{first, overlapping, duplicateID}

	inserted, err := s.Import(ctx, rows)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if inserted != 1 {
		t.Errorf("inserted = %d, want 1", inserted)
	}

	inserted, err = s.Import(ctx, rows)
	if err != nil {
		t.Fatalf("second Import: %v", err)
	}
	if inserted != 0 {
		t.Errorf("second import inserted = %d, want 0", inserted)
	}

	got, err := s.ListByTrainer(ctx, trainer)
	if err != nil {
		t.Fatalf("ListByTrainer: %v", err)
	}
	if len(got) != 2 || got[1] != first {
		t.Errorf("got %+v, want second row %+v", got, first)
	}

	next := mustCreate(t, ctx, s, appt(otherTrainer, "2026-11-10T17:00:00Z"))
	if next.ID != first.ID+1 {
		t.Errorf("next ID = %d, want %d", next.ID, first.ID+1)
	}
}
