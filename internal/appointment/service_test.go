package appointment_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"future-assessment/internal/appointment"
	"future-assessment/internal/scheduling"
	"future-assessment/internal/store"
	"future-assessment/internal/store/storetest"
)

var pacific = func() *time.Location {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		panic(err)
	}
	return loc
}()

func pt(month time.Month, day, hour, minute int) time.Time {
	return time.Date(2026, month, day, hour, minute, 0, 0, pacific)
}

// now is Monday 2026-11-09 10:10 Pacific, mid-way through the 10:00 slot.
var now = pt(time.November, 9, 10, 10)

var errBoom = errors.New("boom")

const (
	trainerID      int64 = 1
	userID         int64 = 2
	otherTrainerID int64 = 3
	otherUserID    int64 = 4
)

var (
	_ appointment.Store = (*store.Store)(nil)
	_ appointment.Store = (*storetest.Fake)(nil)
)

func newService(fs *storetest.Fake) *appointment.Service {
	return appointment.NewService(fs, func() time.Time { return now })
}

func slotStarts(slots []scheduling.Slot) []time.Time {
	out := make([]time.Time, len(slots))
	for i, s := range slots {
		out[i] = s.StartsAt
	}
	return out
}

func containsStart(slots []scheduling.Slot, start time.Time) bool {
	return slices.ContainsFunc(slots, func(s scheduling.Slot) bool { return s.StartsAt.Equal(start) })
}

func TestAvailabilityRangeValidation(t *testing.T) {
	from := pt(time.November, 10, 0, 0)

	tests := []struct {
		name string
		to   time.Time
		want error
	}{
		{"from equals to", from, appointment.ErrInvalidRange},
		{"from after to", from.Add(-time.Minute), appointment.ErrInvalidRange},
		{"exactly max range", from.Add(appointment.MaxRange), nil},
		{"over max range", from.Add(appointment.MaxRange + time.Minute), appointment.ErrRangeTooLong},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newService(&storetest.Fake{}).Availability(t.Context(), trainerID, from, tt.to)
			if !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestAvailabilityExcludesBookedSlots(t *testing.T) {
	fs := &storetest.Fake{}
	fs.Seed(trainerID, pt(time.November, 10, 9, 0), pt(time.November, 10, 9, 30))
	fs.Seed(trainerID, pt(time.November, 10, 9, 30), pt(time.November, 10, 10, 0))
	fs.Seed(otherTrainerID, pt(time.November, 10, 11, 0), pt(time.November, 10, 11, 30))

	slots, err := newService(fs).Availability(t.Context(), trainerID, pt(time.November, 10, 0, 0), pt(time.November, 11, 0, 0))
	if err != nil {
		t.Fatalf("Availability: %v", err)
	}

	if len(slots) != 16 {
		t.Fatalf("len = %d, want 16: %v", len(slots), slotStarts(slots))
	}
	for _, booked := range []time.Time{pt(time.November, 10, 9, 0), pt(time.November, 10, 9, 30)} {
		if containsStart(slots, booked) {
			t.Errorf("booked slot %v listed as available", booked)
		}
	}
	for _, open := range []time.Time{pt(time.November, 10, 8, 30), pt(time.November, 10, 10, 0), pt(time.November, 10, 11, 0)} {
		if !containsStart(slots, open) {
			t.Errorf("open slot %v missing", open)
		}
	}
}

func TestAvailabilityExcludesSlotsTouchedByOffGridBooking(t *testing.T) {
	fs := &storetest.Fake{}
	fs.Seed(trainerID, pt(time.November, 10, 10, 15), pt(time.November, 10, 10, 45))

	slots, err := newService(fs).Availability(t.Context(), trainerID, pt(time.November, 10, 0, 0), pt(time.November, 11, 0, 0))
	if err != nil {
		t.Fatalf("Availability: %v", err)
	}

	if len(slots) != 16 {
		t.Fatalf("len = %d, want 16: %v", len(slots), slotStarts(slots))
	}
	for _, blocked := range []time.Time{pt(time.November, 10, 10, 0), pt(time.November, 10, 10, 30)} {
		if containsStart(slots, blocked) {
			t.Errorf("slot %v overlaps booking but listed as available", blocked)
		}
	}
}

func TestAvailabilityHidesPastAndInProgressSlots(t *testing.T) {
	slots, err := newService(&storetest.Fake{}).Availability(t.Context(), trainerID, pt(time.November, 9, 0, 0), pt(time.November, 10, 0, 0))
	if err != nil {
		t.Fatalf("Availability: %v", err)
	}

	// 10:00 is in progress at 10:10, so the day starts at 10:30: 10:30–16:30 is 13 slots.
	if len(slots) != 13 {
		t.Fatalf("len = %d, want 13: %v", len(slots), slotStarts(slots))
	}
	if !slots[0].StartsAt.Equal(pt(time.November, 9, 10, 30)) {
		t.Errorf("first slot = %v, want 10:30 Pacific", slots[0].StartsAt)
	}
}

func TestAvailabilityEntirelyInPast(t *testing.T) {
	slots, err := newService(&storetest.Fake{}).Availability(t.Context(), trainerID, pt(time.November, 2, 0, 0), pt(time.November, 3, 0, 0))
	if err != nil {
		t.Fatalf("Availability: %v", err)
	}
	if slots == nil || len(slots) != 0 {
		t.Errorf("slots = %#v, want non-nil empty", slots)
	}
}

func TestAvailabilityStoreError(t *testing.T) {
	_, err := newService(&storetest.Fake{Err: errBoom}).Availability(t.Context(), trainerID, pt(time.November, 10, 0, 0), pt(time.November, 11, 0, 0))
	if !errors.Is(err, errBoom) {
		t.Errorf("err = %v, want %v", err, errBoom)
	}
}

func TestBookValidation(t *testing.T) {
	valid := appointment.NewAppointment{
		TrainerID: trainerID,
		UserID:    userID,
		StartsAt:  pt(time.November, 10, 10, 0),
		EndsAt:    pt(time.November, 10, 10, 30),
	}

	tests := []struct {
		name   string
		modify func(*appointment.NewAppointment)
		want   error
	}{
		{"zero trainer", func(r *appointment.NewAppointment) { r.TrainerID = 0 }, appointment.ErrInvalidID},
		{"negative trainer", func(r *appointment.NewAppointment) { r.TrainerID = -1 }, appointment.ErrInvalidID},
		{"zero user", func(r *appointment.NewAppointment) { r.UserID = 0 }, appointment.ErrInvalidID},
		{"wrong duration", func(r *appointment.NewAppointment) { r.EndsAt = pt(time.November, 10, 11, 0) }, scheduling.ErrInvalidDuration},
		{"misaligned", func(r *appointment.NewAppointment) {
			r.StartsAt, r.EndsAt = pt(time.November, 10, 10, 15), pt(time.November, 10, 10, 45)
		}, scheduling.ErrMisaligned},
		{"weekend", func(r *appointment.NewAppointment) {
			r.StartsAt, r.EndsAt = pt(time.November, 14, 10, 0), pt(time.November, 14, 10, 30)
		}, scheduling.ErrOutsideBusinessHours},
		{"past", func(r *appointment.NewAppointment) {
			r.StartsAt, r.EndsAt = pt(time.November, 6, 10, 0), pt(time.November, 6, 10, 30)
		}, appointment.ErrInPast},
		{"in progress", func(r *appointment.NewAppointment) {
			r.StartsAt, r.EndsAt = pt(time.November, 9, 10, 0), pt(time.November, 9, 10, 30)
		}, appointment.ErrInPast},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := valid
			tt.modify(&req)
			fs := &storetest.Fake{}

			_, err := newService(fs).Book(t.Context(), req)
			if !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
			if len(fs.All()) != 0 {
				t.Errorf("invalid request was stored: %+v", fs.All())
			}
		})
	}
}

func TestBookStoresInUTC(t *testing.T) {
	fs := &storetest.Fake{}
	start, err := time.Parse(time.RFC3339, "2026-11-10T13:00:00-05:00") // 10:00 Pacific
	if err != nil {
		t.Fatal(err)
	}
	req := appointment.NewAppointment{
		TrainerID: trainerID,
		UserID:    userID,
		StartsAt:  start,
		EndsAt:    start.Add(scheduling.SlotDuration),
	}

	got, err := newService(fs).Book(t.Context(), req)
	if err != nil {
		t.Fatalf("Book: %v", err)
	}

	if got.ID == 0 {
		t.Error("ID not set")
	}
	if got.TrainerID != req.TrainerID || got.UserID != req.UserID {
		t.Errorf("IDs = %v/%v, want %v/%v", got.TrainerID, got.UserID, req.TrainerID, req.UserID)
	}
	if got.StartsAt.Location() != time.UTC || got.EndsAt.Location() != time.UTC {
		t.Errorf("times not UTC: %v, %v", got.StartsAt, got.EndsAt)
	}
	if !got.StartsAt.Equal(start) {
		t.Errorf("StartsAt = %v, want %v", got.StartsAt, start)
	}
	if len(fs.All()) != 1 {
		t.Errorf("stored %d appointments, want 1", len(fs.All()))
	}
}

func TestBookOverlap(t *testing.T) {
	svc := newService(&storetest.Fake{})
	req := appointment.NewAppointment{
		TrainerID: trainerID,
		UserID:    userID,
		StartsAt:  pt(time.November, 10, 10, 0),
		EndsAt:    pt(time.November, 10, 10, 30),
	}

	if _, err := svc.Book(t.Context(), req); err != nil {
		t.Fatalf("first Book: %v", err)
	}
	req.UserID = otherUserID
	if _, err := svc.Book(t.Context(), req); !errors.Is(err, store.ErrOverlap) {
		t.Errorf("err = %v, want store.ErrOverlap", err)
	}
}

func TestListAppointments(t *testing.T) {
	fs := &storetest.Fake{}
	fs.Seed(trainerID, pt(time.November, 10, 11, 0), pt(time.November, 10, 11, 30))
	fs.Seed(trainerID, pt(time.November, 3, 9, 0), pt(time.November, 3, 9, 30))
	fs.Seed(otherTrainerID, pt(time.November, 10, 9, 0), pt(time.November, 10, 9, 30))

	got, err := newService(fs).ListAppointments(t.Context(), trainerID)
	if err != nil {
		t.Fatalf("ListAppointments: %v", err)
	}

	want := []time.Time{pt(time.November, 3, 9, 0), pt(time.November, 10, 11, 0)}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if !got[i].StartsAt.Equal(want[i]) {
			t.Errorf("got[%d].StartsAt = %v, want %v", i, got[i].StartsAt, want[i])
		}
	}
}

func TestListAppointmentsStoreError(t *testing.T) {
	_, err := newService(&storetest.Fake{Err: errBoom}).ListAppointments(t.Context(), trainerID)
	if !errors.Is(err, errBoom) {
		t.Errorf("err = %v, want %v", err, errBoom)
	}
}
