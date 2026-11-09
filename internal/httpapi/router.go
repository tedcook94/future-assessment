package httpapi

import (
	"log/slog"
	"net/http"

	"future-assessment/internal/appointment"
)

func NewRouter(logger *slog.Logger, svc *appointment.Service) http.Handler {
	a := &api{svc: svc, logger: logger}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handleHealth)
	mux.HandleFunc("GET /trainers/{trainer_id}/availability", a.handleAvailability)
	mux.HandleFunc("GET /trainers/{trainer_id}/appointments", a.handleListAppointments)
	mux.HandleFunc("POST /appointments", a.handleCreateAppointment)

	return logRequests(logger, mux)
}
