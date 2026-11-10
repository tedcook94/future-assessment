package httpapi

import (
	"bytes"
	"encoding/json/v2"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"future-assessment/internal/appointment"
	"future-assessment/internal/store/storetest"
)

// now is Monday 2026-11-09 10:10 Pacific (18:10Z). The next day's 8am Pacific
// slot starts at 2026-11-10T16:00:00Z.
var now = time.Date(2026, time.November, 9, 18, 10, 0, 0, time.UTC)

const (
	trainerID      int64 = 1
	userID         int64 = 2
	otherTrainerID int64 = 3
	otherUserID    int64 = 4
)

var (
	trainer = strconv.FormatInt(trainerID, 10)
	user    = strconv.FormatInt(userID, 10)
)

type testServer struct {
	router http.Handler
	store  *storetest.Fake
	logs   *bytes.Buffer
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()
	return newTestServerWithStore(t, &storetest.Fake{})
}

func newTestServerWithStore(t *testing.T, fake *storetest.Fake) *testServer {
	t.Helper()
	logs := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(logs, nil))
	svc := appointment.NewService(fake, func() time.Time { return now })
	return &testServer{router: NewRouter(logger, svc), store: fake, logs: logs}
}

func (s *testServer) do(t *testing.T, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)

	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("%s %s: Content-Type = %q, want application/json", method, target, got)
	}
	return rec
}

func mustTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func assertStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, want, rec.Body)
	}
}

func assertBody(t *testing.T, rec *httptest.ResponseRecorder, want string) {
	t.Helper()
	if got := strings.TrimSpace(rec.Body.String()); got != want {
		t.Errorf("body =\n%s\nwant\n%s", got, want)
	}
}

func assertErrorBody(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	var body errorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Error == "" {
		t.Errorf("body = %s, want {\"error\": \"...\"}", rec.Body)
	}
}

func decodeBody[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %s: %v", rec.Body, err)
	}
	return v
}

func availabilityURL(trainerID, from, to string) string {
	return "/trainers/" + trainerID + "/availability?starts_at=" + from + "&ends_at=" + to
}

func TestAvailability(t *testing.T) {
	s := newTestServer(t)
	s.store.Seed(trainerID, mustTime("2026-11-10T16:00:00Z"), mustTime("2026-11-10T16:30:00Z"))

	rec := s.do(t, http.MethodGet, availabilityURL(trainer, "2026-11-10T16:00:00Z", "2026-11-10T17:30:00Z"), "")

	assertStatus(t, rec, http.StatusOK)
	assertBody(t, rec, `{"available_times":[`+
		`{"starts_at":"2026-11-10T16:30:00Z","ends_at":"2026-11-10T17:00:00Z"},`+
		`{"starts_at":"2026-11-10T17:00:00Z","ends_at":"2026-11-10T17:30:00Z"}]}`)
}

func TestAvailabilityAcceptsOffsets(t *testing.T) {
	s := newTestServer(t)

	rec := s.do(t, http.MethodGet, availabilityURL(trainer, "2026-11-10T08:00:00-08:00", "2026-11-10T08:30:00-08:00"), "")

	assertStatus(t, rec, http.StatusOK)
	assertBody(t, rec, `{"available_times":[{"starts_at":"2026-11-10T16:00:00Z","ends_at":"2026-11-10T16:30:00Z"}]}`)
}

func TestAvailabilityEmpty(t *testing.T) {
	s := newTestServer(t)

	// Saturday 00:00 to Monday 00:00 Pacific.
	rec := s.do(t, http.MethodGet, availabilityURL(trainer, "2026-11-14T08:00:00Z", "2026-11-16T08:00:00Z"), "")

	assertStatus(t, rec, http.StatusOK)
	assertBody(t, rec, `{"available_times":[]}`)
}

func TestAvailabilityBadRequest(t *testing.T) {
	tests := []struct {
		name   string
		target string
	}{
		{"non-numeric trainer_id", availabilityURL("abc", "2026-11-10T00:00:00Z", "2026-11-11T00:00:00Z")},
		{"zero trainer_id", availabilityURL("0", "2026-11-10T00:00:00Z", "2026-11-11T00:00:00Z")},
		{"negative trainer_id", availabilityURL("-1", "2026-11-10T00:00:00Z", "2026-11-11T00:00:00Z")},
		{"fractional trainer_id", availabilityURL("1.5", "2026-11-10T00:00:00Z", "2026-11-11T00:00:00Z")},
		{"missing starts_at", "/trainers/" + trainer + "/availability?ends_at=2026-11-11T00:00:00Z"},
		{"missing ends_at", "/trainers/" + trainer + "/availability?starts_at=2026-11-10T00:00:00Z"},
		{"date-only starts_at", availabilityURL(trainer, "2026-11-10", "2026-11-11T00:00:00Z")},
		{"inverted range", availabilityURL(trainer, "2026-11-11T00:00:00Z", "2026-11-10T00:00:00Z")},
		{"range over 30 days", availabilityURL(trainer, "2026-11-10T00:00:00Z", "2026-12-10T00:01:00Z")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := newTestServer(t).do(t, http.MethodGet, tt.target, "")
			assertStatus(t, rec, http.StatusBadRequest)
			assertErrorBody(t, rec)
		})
	}
}

func TestCreateAppointment(t *testing.T) {
	s := newTestServer(t)
	body := `{"trainer_id":` + trainer + `,"user_id":` + user + `,` +
		`"starts_at":"2026-11-10T08:00:00-08:00","ends_at":"2026-11-10T08:30:00-08:00"}`

	rec := s.do(t, http.MethodPost, "/appointments", body)

	assertStatus(t, rec, http.StatusCreated)
	got := decodeBody[appointmentResponse](t, rec)
	if got.ID == 0 {
		t.Error("id not set")
	}
	if got.TrainerID != trainerID || got.UserID != userID {
		t.Errorf("ids = %v/%v, want %v/%v", got.TrainerID, got.UserID, trainerID, userID)
	}
	if !strings.Contains(rec.Body.String(), `"starts_at":"2026-11-10T16:00:00Z","ends_at":"2026-11-10T16:30:00Z"`) {
		t.Errorf("times not normalized to UTC: %s", rec.Body)
	}

	rec = s.do(t, http.MethodGet, availabilityURL(trainer, "2026-11-10T16:00:00Z", "2026-11-10T16:30:00Z"), "")
	assertStatus(t, rec, http.StatusOK)
	assertBody(t, rec, `{"available_times":[]}`)
}

func TestCreateAppointmentConflict(t *testing.T) {
	s := newTestServer(t)
	body := func(userID int64) string {
		return `{"trainer_id":` + trainer + `,"user_id":` + strconv.FormatInt(userID, 10) + `,` +
			`"starts_at":"2026-11-10T16:00:00Z","ends_at":"2026-11-10T16:30:00Z"}`
	}

	assertStatus(t, s.do(t, http.MethodPost, "/appointments", body(userID)), http.StatusCreated)

	rec := s.do(t, http.MethodPost, "/appointments", body(otherUserID))
	assertStatus(t, rec, http.StatusConflict)
	assertErrorBody(t, rec)
}

func TestCreateAppointmentErrors(t *testing.T) {
	withIDs := func(trainerID, userID string) string {
		return `{"trainer_id":` + trainerID + `,"user_id":` + userID + `,"starts_at":"2026-11-10T16:00:00Z","ends_at":"2026-11-10T16:30:00Z"}`
	}
	withTimes := func(start, end string) string {
		return `{"trainer_id":` + trainer + `,"user_id":` + user + `,"starts_at":"` + start + `","ends_at":"` + end + `"}`
	}

	tests := []struct {
		name   string
		body   string
		status int
	}{
		{"malformed JSON", `{`, http.StatusBadRequest},
		{"unknown field", `{"trainer_id":` + trainer + `,"extra":1}`, http.StatusBadRequest},
		{"trailing data", withTimes("2026-11-10T16:00:00Z", "2026-11-10T16:30:00Z") + `{}`, http.StatusBadRequest},
		{"duplicate key", `{"trainer_id":` + trainer + `,"trainer_id":` + trainer + `,"user_id":` + user + `,"starts_at":"2026-11-10T16:00:00Z","ends_at":"2026-11-10T16:30:00Z"}`, http.StatusBadRequest},
		{"wrong key case", `{"Trainer_ID":` + trainer + `,"user_id":` + user + `,"starts_at":"2026-11-10T16:00:00Z","ends_at":"2026-11-10T16:30:00Z"}`, http.StatusBadRequest},
		{"string trainer_id", withIDs(`"1"`, user), http.StatusBadRequest},
		{"fractional trainer_id", withIDs("1.5", user), http.StatusBadRequest},
		{"zero trainer_id", withIDs("0", user), http.StatusBadRequest},
		{"negative user_id", withIDs(trainer, "-1"), http.StatusBadRequest},
		{"missing trainer_id", `{"user_id":` + user + `,"starts_at":"2026-11-10T16:00:00Z","ends_at":"2026-11-10T16:30:00Z"}`, http.StatusBadRequest},
		{"missing ends_at", `{"trainer_id":` + trainer + `,"user_id":` + user + `,"starts_at":"2026-11-10T16:00:00Z"}`, http.StatusBadRequest},
		{"non-RFC 3339 time", withTimes("2026-11-10 16:00:00", "2026-11-10T16:30:00Z"), http.StatusBadRequest},
		{"body too large", `{"trainer_id":` + strings.Repeat("1", maxBodyBytes) + `}`, http.StatusRequestEntityTooLarge},
		{"wrong duration", withTimes("2026-11-10T16:00:00Z", "2026-11-10T17:00:00Z"), http.StatusUnprocessableEntity},
		{"misaligned", withTimes("2026-11-10T16:15:00Z", "2026-11-10T16:45:00Z"), http.StatusUnprocessableEntity},
		{"weekend", withTimes("2026-11-14T18:00:00Z", "2026-11-14T18:30:00Z"), http.StatusUnprocessableEntity},
		{"past", withTimes("2026-11-06T18:00:00Z", "2026-11-06T18:30:00Z"), http.StatusUnprocessableEntity},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestServer(t)
			rec := s.do(t, http.MethodPost, "/appointments", tt.body)

			assertStatus(t, rec, tt.status)
			assertErrorBody(t, rec)
			if n := len(s.store.All()); n != 0 {
				t.Errorf("stored %d appointments, want 0", n)
			}
		})
	}
}

func TestListAppointments(t *testing.T) {
	s := newTestServer(t)
	later := s.store.Seed(trainerID, mustTime("2026-11-10T18:00:00Z"), mustTime("2026-11-10T18:30:00Z"))
	earlier := s.store.Seed(trainerID, mustTime("2026-11-03T17:00:00Z"), mustTime("2026-11-03T17:30:00Z"))
	s.store.Seed(otherTrainerID, mustTime("2026-11-10T17:00:00Z"), mustTime("2026-11-10T17:30:00Z"))

	rec := s.do(t, http.MethodGet, "/trainers/"+trainer+"/appointments", "")

	assertStatus(t, rec, http.StatusOK)
	got := decodeBody[appointmentsResponse](t, rec).Appointments
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2: %s", len(got), rec.Body)
	}
	if got[0].ID != earlier.ID || got[1].ID != later.ID {
		t.Errorf("order = [%v %v], want [%v %v]", got[0].ID, got[1].ID, earlier.ID, later.ID)
	}
}

func TestListAppointmentsEmpty(t *testing.T) {
	rec := newTestServer(t).do(t, http.MethodGet, "/trainers/"+trainer+"/appointments", "")

	assertStatus(t, rec, http.StatusOK)
	assertBody(t, rec, `{"appointments":[]}`)
}

func TestListAppointmentsInvalidTrainerID(t *testing.T) {
	for _, id := range []string{"abc", "0", "-1", "9223372036854775808"} {
		t.Run(id, func(t *testing.T) {
			rec := newTestServer(t).do(t, http.MethodGet, "/trainers/"+id+"/appointments", "")

			assertStatus(t, rec, http.StatusBadRequest)
			assertErrorBody(t, rec)
		})
	}
}

func TestStoreFailureIsInternalError(t *testing.T) {
	s := newTestServerWithStore(t, &storetest.Fake{Err: errors.New("connection refused")})

	tests := []struct {
		name, method, target, body string
	}{
		{"availability", http.MethodGet, availabilityURL(trainer, "2026-11-10T00:00:00Z", "2026-11-11T00:00:00Z"), ""},
		{"list", http.MethodGet, "/trainers/" + trainer + "/appointments", ""},
		{"create", http.MethodPost, "/appointments", `{"trainer_id":` + trainer + `,"user_id":` + user + `,` +
			`"starts_at":"2026-11-10T16:00:00Z","ends_at":"2026-11-10T16:30:00Z"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s.logs.Reset()
			rec := s.do(t, tt.method, tt.target, tt.body)

			assertStatus(t, rec, http.StatusInternalServerError)
			assertBody(t, rec, `{"error":"internal server error"}`)
			if !strings.Contains(s.logs.String(), "connection refused") {
				t.Errorf("underlying error not logged: %s", s.logs)
			}
		})
	}
}
