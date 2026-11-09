package httpapi

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"net/http"

	"future-assessment/internal/appointment"
	"future-assessment/internal/scheduling"
	"future-assessment/internal/store"
)

const maxBodyBytes = 1 << 20

var errorStatuses = []struct {
	err    error
	status int
}{
	{appointment.ErrInvalidRange, http.StatusBadRequest},
	{appointment.ErrRangeTooLong, http.StatusBadRequest},
	{appointment.ErrInvalidID, http.StatusBadRequest},
	{store.ErrOverlap, http.StatusConflict},
	{scheduling.ErrInvalidDuration, http.StatusUnprocessableEntity},
	{scheduling.ErrMisaligned, http.StatusUnprocessableEntity},
	{scheduling.ErrOutsideBusinessHours, http.StatusUnprocessableEntity},
	{appointment.ErrInPast, http.StatusUnprocessableEntity},
}

type errorResponse struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.MarshalWrite(w, v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}

// writeServiceError maps known domain errors to client errors. Anything else is
// logged and reported as a generic 500 so internal details don't leak.
func (a *api) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	for _, e := range errorStatuses {
		if errors.Is(err, e.err) {
			writeError(w, e.status, e.err.Error())
			return
		}
	}

	a.logger.ErrorContext(r.Context(), "request failed",
		"method", r.Method,
		"path", r.URL.Path,
		"err", err,
	)
	writeError(w, http.StatusInternalServerError, "internal server error")
}

// decodeJSON decodes exactly one JSON value into dst, rejecting unknown
// fields, trailing data, and bodies over maxBodyBytes. On failure it writes the
// error response and returns false.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	err := json.UnmarshalRead(http.MaxBytesReader(w, r.Body, maxBodyBytes), dst, json.RejectUnknownMembers(true))
	if err == nil {
		return true
	}

	if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
		writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("body must not exceed %d bytes", maxBodyBytes))
		return false
	}
	writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
	return false
}
