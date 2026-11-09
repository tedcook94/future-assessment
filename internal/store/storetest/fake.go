package storetest

import (
	"context"
	"slices"
	"sync"
	"time"
	"uuid"

	"future-assessment/internal/store"
)

// Fake is an in-memory appointment store that enforces the same no-overlap
// rule as the Postgres exclusion constraint. The zero value is ready to use.
type Fake struct {
	// Err, if set, is returned by every store method.
	Err error

	mu    sync.Mutex
	appts []store.Appointment
}

func (f *Fake) ListByTrainer(_ context.Context, trainerID uuid.UUID) ([]store.Appointment, error) {
	return f.filter(func(a store.Appointment) bool { return a.TrainerID == trainerID })
}

func (f *Fake) ListInRange(_ context.Context, trainerID uuid.UUID, from, to time.Time) ([]store.Appointment, error) {
	return f.filter(func(a store.Appointment) bool {
		return a.TrainerID == trainerID && a.StartsAt.Before(to) && a.EndsAt.After(from)
	})
}

func (f *Fake) Create(_ context.Context, a store.Appointment) (store.Appointment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return store.Appointment{}, f.Err
	}
	for _, b := range f.appts {
		if b.TrainerID == a.TrainerID && a.StartsAt.Before(b.EndsAt) && b.StartsAt.Before(a.EndsAt) {
			return store.Appointment{}, store.ErrOverlap
		}
	}

	a.ID = uuid.NewV7()
	f.appts = append(f.appts, a)
	return a, nil
}

// Seed inserts an appointment directly, skipping the overlap check, the way
// legacy data might arrive. Use it to arrange test state.
func (f *Fake) Seed(trainerID uuid.UUID, start, end time.Time) store.Appointment {
	f.mu.Lock()
	defer f.mu.Unlock()

	a := store.Appointment{
		ID:        uuid.NewV7(),
		TrainerID: trainerID,
		UserID:    uuid.New(),
		StartsAt:  start.UTC(),
		EndsAt:    end.UTC(),
	}
	f.appts = append(f.appts, a)
	return a
}

func (f *Fake) All() []store.Appointment {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.appts)
}

func (f *Fake) filter(keep func(store.Appointment) bool) ([]store.Appointment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.Err != nil {
		return nil, f.Err
	}
	var out []store.Appointment
	for _, a := range f.appts {
		if keep(a) {
			out = append(out, a)
		}
	}
	slices.SortFunc(out, func(a, b store.Appointment) int { return a.StartsAt.Compare(b.StartsAt) })
	return out, nil
}
