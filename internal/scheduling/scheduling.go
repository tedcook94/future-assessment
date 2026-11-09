package scheduling

import (
	"errors"
	"time"

	// Embedded so the Pacific zone loads even on images without system tzdata.
	_ "time/tzdata"
)

const SlotDuration = 30 * time.Minute

const (
	openHour  = 8
	closeHour = 17
)

var pacific = mustLoadLocation("America/Los_Angeles")

var (
	ErrInvalidDuration      = errors.New("appointment must be exactly 30 minutes")
	ErrMisaligned           = errors.New("appointment must start at :00 or :30")
	ErrOutsideBusinessHours = errors.New("appointment must be Mon–Fri, 8am–5pm Pacific")
)

type Slot struct {
	StartsAt time.Time
	EndsAt   time.Time
}

// ValidateSlot reports whether [start, end) is a bookable slot. It does not
// check whether the slot is in the past or already taken.
func ValidateSlot(start, end time.Time) error {
	if end.Sub(start) != SlotDuration {
		return ErrInvalidDuration
	}

	local := start.In(pacific)
	if local.Minute()%30 != 0 || local.Second() != 0 || local.Nanosecond() != 0 {
		return ErrMisaligned
	}

	if !isWeekday(local.Weekday()) || local.Hour() < openHour || local.Hour() >= closeHour {
		return ErrOutsideBusinessHours
	}

	return nil
}

// Slots returns every business-hours slot that fits entirely within the
// half-open range [from, to), ordered by start time.
func Slots(from, to time.Time) []Slot {
	if !from.Before(to) {
		return nil
	}

	fy, fm, fd := from.In(pacific).Date()
	last := to.In(pacific)

	var slots []Slot
	// Build each slot from its calendar date rather than adding durations, so
	// DST transitions can't shift slots off the wall-clock grid.
	for day := time.Date(fy, fm, fd, 0, 0, 0, 0, pacific); !day.After(last); day = day.AddDate(0, 0, 1) {
		if !isWeekday(day.Weekday()) {
			continue
		}

		y, m, d := day.Date()
		for hour := openHour; hour < closeHour; hour++ {
			for _, minute := range []int{0, 30} {
				start := time.Date(y, m, d, hour, minute, 0, 0, pacific)
				end := start.Add(SlotDuration)
				if start.Before(from) || end.After(to) {
					continue
				}
				slots = append(slots, Slot{StartsAt: start.UTC(), EndsAt: end.UTC()})
			}
		}
	}

	return slots
}

func isWeekday(d time.Weekday) bool {
	return d != time.Saturday && d != time.Sunday
}

func mustLoadLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}
