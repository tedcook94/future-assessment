package scheduling_test

import (
	"errors"
	"testing"
	"time"

	"future-assessment/internal/scheduling"
)

var pacific = func() *time.Location {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		panic(err)
	}
	return loc
}()

func pt(year int, month time.Month, day, hour, minute int) time.Time {
	return time.Date(year, month, day, hour, minute, 0, 0, pacific)
}

func utc(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestValidateSlot(t *testing.T) {
	tests := []struct {
		name  string
		start time.Time
		end   time.Time
		want  error
	}{
		{"opening slot", pt(2026, 11, 9, 8, 0), pt(2026, 11, 9, 8, 30), nil},
		{"closing slot", pt(2026, 11, 9, 16, 30), pt(2026, 11, 9, 17, 0), nil},
		{"winter 8am is 16:00Z", utc("2026-12-07T16:00:00Z"), utc("2026-12-07T16:30:00Z"), nil},
		{"summer 8am is 15:00Z", utc("2026-07-06T15:00:00Z"), utc("2026-07-06T15:30:00Z"), nil},
		{"non-UTC offset", utc("2026-11-09T11:00:00-05:00"), utc("2026-11-09T11:30:00-05:00"), nil},

		{"starts at close", pt(2026, 11, 9, 17, 0), pt(2026, 11, 9, 17, 30), scheduling.ErrOutsideBusinessHours},
		{"before open", pt(2026, 11, 9, 7, 30), pt(2026, 11, 9, 8, 0), scheduling.ErrOutsideBusinessHours},
		{"winter 15:00Z is 7am", utc("2026-12-07T15:00:00Z"), utc("2026-12-07T15:30:00Z"), scheduling.ErrOutsideBusinessHours},
		{"saturday", pt(2026, 11, 14, 10, 0), pt(2026, 11, 14, 10, 30), scheduling.ErrOutsideBusinessHours},
		{"sunday", pt(2026, 11, 15, 10, 0), pt(2026, 11, 15, 10, 30), scheduling.ErrOutsideBusinessHours},
		{"quarter past", pt(2026, 11, 9, 10, 15), pt(2026, 11, 9, 10, 45), scheduling.ErrMisaligned},
		{"nonzero seconds", pt(2026, 11, 9, 10, 0).Add(time.Second), pt(2026, 11, 9, 10, 30).Add(time.Second), scheduling.ErrMisaligned},
		{"sixty minutes", pt(2026, 11, 9, 10, 0), pt(2026, 11, 9, 11, 0), scheduling.ErrInvalidDuration},
		{"end before start", pt(2026, 11, 9, 10, 30), pt(2026, 11, 9, 10, 0), scheduling.ErrInvalidDuration},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := scheduling.ValidateSlot(tt.start, tt.end); !errors.Is(err, tt.want) {
				t.Errorf("ValidateSlot() = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestSlotsFullWeekday(t *testing.T) {
	slots := scheduling.Slots(pt(2026, 11, 9, 0, 0), pt(2026, 11, 10, 0, 0))

	if len(slots) != 18 {
		t.Fatalf("len = %d, want 18", len(slots))
	}
	assertSlot(t, slots[0], pt(2026, 11, 9, 8, 0))
	assertSlot(t, slots[17], pt(2026, 11, 9, 16, 30))

	for i, s := range slots {
		if err := scheduling.ValidateSlot(s.StartsAt, s.EndsAt); err != nil {
			t.Errorf("slots[%d] invalid: %v", i, err)
		}
		if s.StartsAt.Location() != time.UTC || s.EndsAt.Location() != time.UTC {
			t.Errorf("slots[%d] not UTC: %v", i, s)
		}
	}
}

func TestSlotsAcrossDSTEnd(t *testing.T) {
	// DST ends Sunday 2026-11-01, so 8am Pacific shifts from 15:00Z to 16:00Z.
	slots := scheduling.Slots(pt(2026, 10, 30, 0, 0), pt(2026, 11, 3, 0, 0))

	if len(slots) != 36 {
		t.Fatalf("len = %d, want 36 (Fri + Mon, no weekend)", len(slots))
	}
	if got, want := slots[0].StartsAt, utc("2026-10-30T15:00:00Z"); !got.Equal(want) {
		t.Errorf("Friday first slot = %v, want %v", got, want)
	}
	if got, want := slots[18].StartsAt, utc("2026-11-02T16:00:00Z"); !got.Equal(want) {
		t.Errorf("Monday first slot = %v, want %v", got, want)
	}
}

func TestSlotsFullWeek(t *testing.T) {
	slots := scheduling.Slots(pt(2026, 11, 9, 0, 0), pt(2026, 11, 16, 0, 0))

	if len(slots) != 90 {
		t.Fatalf("len = %d, want 90", len(slots))
	}
}

func TestSlotsPartialRange(t *testing.T) {
	slots := scheduling.Slots(pt(2026, 11, 9, 10, 15), pt(2026, 11, 9, 12, 0))

	want := []time.Time{
		pt(2026, 11, 9, 10, 30),
		pt(2026, 11, 9, 11, 0),
		pt(2026, 11, 9, 11, 30),
	}
	if len(slots) != len(want) {
		t.Fatalf("len = %d, want %d: %v", len(slots), len(want), slots)
	}
	for i, start := range want {
		assertSlot(t, slots[i], start)
	}
}

func TestSlotsExcludesSlotCrossingRangeEnd(t *testing.T) {
	slots := scheduling.Slots(pt(2026, 11, 9, 10, 0), pt(2026, 11, 9, 10, 45))

	if len(slots) != 1 {
		t.Fatalf("len = %d, want 1: %v", len(slots), slots)
	}
	assertSlot(t, slots[0], pt(2026, 11, 9, 10, 0))
}

func TestSlotsEmptyRange(t *testing.T) {
	tests := []struct {
		name     string
		from, to time.Time
	}{
		{"from equals to", pt(2026, 11, 9, 10, 0), pt(2026, 11, 9, 10, 0)},
		{"from after to", pt(2026, 11, 10, 0, 0), pt(2026, 11, 9, 0, 0)},
		{"weekend only", pt(2026, 11, 14, 0, 0), pt(2026, 11, 16, 0, 0)},
		{"after hours only", pt(2026, 11, 9, 17, 0), pt(2026, 11, 10, 8, 0)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if slots := scheduling.Slots(tt.from, tt.to); len(slots) != 0 {
				t.Errorf("len = %d, want 0: %v", len(slots), slots)
			}
		})
	}
}

func assertSlot(t *testing.T, got scheduling.Slot, wantStart time.Time) {
	t.Helper()
	if !got.StartsAt.Equal(wantStart) {
		t.Errorf("StartsAt = %v, want %v", got.StartsAt, wantStart)
	}
	if want := wantStart.Add(scheduling.SlotDuration); !got.EndsAt.Equal(want) {
		t.Errorf("EndsAt = %v, want %v", got.EndsAt, want)
	}
}
