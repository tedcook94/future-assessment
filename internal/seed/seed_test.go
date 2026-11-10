package seed_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"future-assessment/internal/seed"
	"future-assessment/internal/store"
)

var _ seed.Importer = (*store.Store)(nil)

type recordingImporter struct {
	got []store.Appointment
	err error
}

func (r *recordingImporter) Import(_ context.Context, appts []store.Appointment) (int, error) {
	if r.err != nil {
		return 0, r.err
	}
	r.got = appts
	return len(appts), nil
}

func writeFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "appointments.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func ids(appts []store.Appointment) []int64 {
	out := make([]int64, len(appts))
	for i, a := range appts {
		out[i] = a.ID
	}
	return out
}

func TestParse(t *testing.T) {
	appts, err := seed.Parse(strings.NewReader(`[
		{"id": 7, "trainer_id": 1, "user_id": 2, "started_at": "2019-01-24T09:00:00-08:00", "ended_at": "2019-01-24T09:30:00-08:00"}
	]`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	want := store.Appointment{
		ID:        7,
		TrainerID: 1,
		UserID:    2,
		StartsAt:  time.Date(2019, time.January, 24, 17, 0, 0, 0, time.UTC),
		EndsAt:    time.Date(2019, time.January, 24, 17, 30, 0, 0, time.UTC),
	}
	if len(appts) != 1 || appts[0] != want {
		t.Errorf("got %+v, want [%+v]", appts, want)
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name, input string
	}{
		{"malformed JSON", `[{`},
		{"not an array", `{}`},
		{"missing field", `[{"id": 1, "trainer_id": 1, "user_id": 1, "started_at": "2019-01-24T09:00:00-08:00"}]`},
		{"unknown field", `[{"id": 1, "trainer_id": 1, "user_id": 1, "started_at": "2019-01-24T09:00:00-08:00", "ended_at": "2019-01-24T09:30:00-08:00", "notes": ""}]`},
		{"string ID", `[{"id": "1", "trainer_id": 1, "user_id": 1, "started_at": "2019-01-24T09:00:00-08:00", "ended_at": "2019-01-24T09:30:00-08:00"}]`},
		{"invalid time", `[{"id": 1, "trainer_id": 1, "user_id": 1, "started_at": "2019-01-24 09:00", "ended_at": "2019-01-24T09:30:00-08:00"}]`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := seed.Parse(strings.NewReader(tt.input)); err == nil {
				t.Error("err = nil, want an error")
			}
		})
	}
}

func TestRunImportsRepositoryFile(t *testing.T) {
	imp := &recordingImporter{}
	logs := &bytes.Buffer{}

	if err := seed.Run(t.Context(), imp, "../../appointments.json", slog.New(slog.NewTextHandler(logs, nil))); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if want := []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}; !slices.Equal(ids(imp.got), want) {
		t.Errorf("imported IDs = %v, want %v", ids(imp.got), want)
	}
	// Rows 5, 7, 8, 9, and 10 are on a Saturday; row 9 is also an hour long.
	if n := strings.Count(logs.String(), "breaks booking rules"); n != 5 {
		t.Errorf("rule warnings = %d, want 5:\n%s", n, logs)
	}
}

func TestRunSkipsInvalidRows(t *testing.T) {
	path := writeFile(t, `[
		{"id": 1, "trainer_id": 1, "user_id": 1, "started_at": "2019-01-24T09:00:00-08:00", "ended_at": "2019-01-24T09:30:00-08:00"},
		{"id": 2, "trainer_id": 1, "user_id": 1, "started_at": "2019-01-24T10:00:00-08:00", "ended_at": "2019-01-24T10:00:00-08:00"},
		{"id": 3, "trainer_id": 1, "user_id": 1, "started_at": "2019-01-24T11:00:00-08:00", "ended_at": "2019-01-24T10:30:00-08:00"},
		{"id": 0, "trainer_id": 1, "user_id": 1, "started_at": "2019-01-24T12:00:00-08:00", "ended_at": "2019-01-24T12:30:00-08:00"},
		{"id": 5, "trainer_id": -1, "user_id": 1, "started_at": "2019-01-24T13:00:00-08:00", "ended_at": "2019-01-24T13:30:00-08:00"}
	]`)
	imp := &recordingImporter{}

	if err := seed.Run(t.Context(), imp, path, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if want := []int64{1}; !slices.Equal(ids(imp.got), want) {
		t.Errorf("imported IDs = %v, want %v", ids(imp.got), want)
	}
}

func TestRunErrors(t *testing.T) {
	errBoom := errors.New("boom")

	tests := []struct {
		name string
		path string
		imp  *recordingImporter
	}{
		{"missing file", filepath.Join(t.TempDir(), "missing.json"), &recordingImporter{}},
		{"malformed file", writeFile(t, `[{`), &recordingImporter{}},
		{"import failure", writeFile(t, `[]`), &recordingImporter{err: errBoom}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := seed.Run(t.Context(), tt.imp, tt.path, slog.New(slog.DiscardHandler)); err == nil {
				t.Error("err = nil, want an error")
			}
		})
	}
}
