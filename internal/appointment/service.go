package appointment

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"future-assessment/internal/scheduling"
	"future-assessment/internal/store"
)

const MaxRange = 30 * 24 * time.Hour

var (
	ErrInvalidRange = errors.New("starts_at must be before ends_at")
	ErrRangeTooLong = errors.New("range must not exceed 30 days")
	ErrInPast       = errors.New("appointment must start in the future")
	ErrInvalidID    = errors.New("trainer_id and user_id must be positive integers")
)

type Store interface {
	ListByTrainer(ctx context.Context, trainerID int64) ([]store.Appointment, error)
	ListInRange(ctx context.Context, trainerID int64, from, to time.Time) ([]store.Appointment, error)
	Create(ctx context.Context, a store.Appointment) (store.Appointment, error)
}

type NewAppointment struct {
	TrainerID int64
	UserID    int64
	StartsAt  time.Time
	EndsAt    time.Time
}

type Service struct {
	store Store
	now   func() time.Time
}

func NewService(s Store, now func() time.Time) *Service {
	return &Service{store: s, now: now}
}

// Availability returns the trainer's open slots within [from, to), excluding
// slots that have already started. The range may not exceed MaxRange.
func (s *Service) Availability(ctx context.Context, trainerID int64, from, to time.Time) ([]scheduling.Slot, error) {
	if !from.Before(to) {
		return nil, ErrInvalidRange
	}
	if to.Sub(from) > MaxRange {
		return nil, ErrRangeTooLong
	}

	if now := s.now(); from.Before(now) {
		from = now
	}
	if !from.Before(to) {
		return []scheduling.Slot{}, nil
	}

	booked, err := s.store.ListInRange(ctx, trainerID, from, to)
	if err != nil {
		return nil, fmt.Errorf("list booked appointments: %w", err)
	}

	return subtractBooked(scheduling.Slots(from, to), booked), nil
}

// subtractBooked returns the slots that overlap no booking.
func subtractBooked(slots []scheduling.Slot, booked []store.Appointment) []scheduling.Slot {
	available := make([]scheduling.Slot, 0, len(slots))
	for _, slot := range slots {
		taken := slices.ContainsFunc(booked, func(a store.Appointment) bool {
			return a.StartsAt.Before(slot.EndsAt) && a.EndsAt.After(slot.StartsAt)
		})
		if !taken {
			available = append(available, slot)
		}
	}
	return available
}

// Book validates and stores a new appointment. It returns store.ErrOverlap if
// the trainer is already booked at that time.
func (s *Service) Book(ctx context.Context, req NewAppointment) (store.Appointment, error) {
	if req.TrainerID <= 0 || req.UserID <= 0 {
		return store.Appointment{}, ErrInvalidID
	}
	if err := scheduling.ValidateSlot(req.StartsAt, req.EndsAt); err != nil {
		return store.Appointment{}, err
	}
	if req.StartsAt.Before(s.now()) {
		return store.Appointment{}, ErrInPast
	}

	return s.store.Create(ctx, store.Appointment{
		TrainerID: req.TrainerID,
		UserID:    req.UserID,
		StartsAt:  req.StartsAt.UTC(),
		EndsAt:    req.EndsAt.UTC(),
	})
}

// ListAppointments returns all of a trainer's appointments, ordered by start
// time.
func (s *Service) ListAppointments(ctx context.Context, trainerID int64) ([]store.Appointment, error) {
	appts, err := s.store.ListByTrainer(ctx, trainerID)
	if err != nil {
		return nil, fmt.Errorf("list appointments: %w", err)
	}
	return appts, nil
}
