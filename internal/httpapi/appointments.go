package httpapi

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"future-assessment/internal/appointment"
	"future-assessment/internal/store"
)

type api struct {
	svc    *appointment.Service
	logger *slog.Logger
}

type slotResponse struct {
	StartsAt time.Time `json:"starts_at"`
	EndsAt   time.Time `json:"ends_at"`
}

type availabilityResponse struct {
	AvailableTimes []slotResponse `json:"available_times"`
}

type appointmentResponse struct {
	ID        int64     `json:"id"`
	TrainerID int64     `json:"trainer_id"`
	UserID    int64     `json:"user_id"`
	StartsAt  time.Time `json:"starts_at"`
	EndsAt    time.Time `json:"ends_at"`
}

type appointmentsResponse struct {
	Appointments []appointmentResponse `json:"appointments"`
}

type createAppointmentRequest struct {
	TrainerID int64     `json:"trainer_id"`
	UserID    int64     `json:"user_id"`
	StartsAt  time.Time `json:"starts_at"`
	EndsAt    time.Time `json:"ends_at"`
}

func (a *api) handleAvailability(w http.ResponseWriter, r *http.Request) {
	trainerID, ok := pathTrainerID(w, r)
	if !ok {
		return
	}
	from, ok := queryTime(w, r, "starts_at")
	if !ok {
		return
	}
	to, ok := queryTime(w, r, "ends_at")
	if !ok {
		return
	}

	slots, err := a.svc.Availability(r.Context(), trainerID, from, to)
	if err != nil {
		a.writeServiceError(w, r, err)
		return
	}

	resp := availabilityResponse{AvailableTimes: make([]slotResponse, 0, len(slots))}
	for _, s := range slots {
		resp.AvailableTimes = append(resp.AvailableTimes, slotResponse{StartsAt: s.StartsAt, EndsAt: s.EndsAt})
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *api) handleCreateAppointment(w http.ResponseWriter, r *http.Request) {
	var req createAppointmentRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	// Without this, a missing time decodes to the zero value and surfaces as a
	// confusing "must be exactly 30 minutes" error.
	if req.StartsAt.IsZero() || req.EndsAt.IsZero() {
		writeError(w, http.StatusBadRequest, "starts_at and ends_at are required")
		return
	}

	appt, err := a.svc.Book(r.Context(), appointment.NewAppointment{
		TrainerID: req.TrainerID,
		UserID:    req.UserID,
		StartsAt:  req.StartsAt,
		EndsAt:    req.EndsAt,
	})
	if err != nil {
		a.writeServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, toAppointmentResponse(appt))
}

func (a *api) handleListAppointments(w http.ResponseWriter, r *http.Request) {
	trainerID, ok := pathTrainerID(w, r)
	if !ok {
		return
	}

	appts, err := a.svc.ListAppointments(r.Context(), trainerID)
	if err != nil {
		a.writeServiceError(w, r, err)
		return
	}

	resp := appointmentsResponse{Appointments: make([]appointmentResponse, 0, len(appts))}
	for _, appt := range appts {
		resp.Appointments = append(resp.Appointments, toAppointmentResponse(appt))
	}
	writeJSON(w, http.StatusOK, resp)
}

func toAppointmentResponse(a store.Appointment) appointmentResponse {
	return appointmentResponse{
		ID:        a.ID,
		TrainerID: a.TrainerID,
		UserID:    a.UserID,
		StartsAt:  a.StartsAt,
		EndsAt:    a.EndsAt,
	}
}

func pathTrainerID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("trainer_id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "trainer_id must be a positive integer")
		return 0, false
	}
	return id, true
}

func queryTime(w http.ResponseWriter, r *http.Request, name string) (time.Time, bool) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		writeError(w, http.StatusBadRequest, name+" is required")
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, name+" must be an RFC 3339 timestamp")
		return time.Time{}, false
	}
	return t, true
}
