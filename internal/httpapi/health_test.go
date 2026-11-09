package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealth(t *testing.T) {
	rec := newTestServer(t).do(t, http.MethodGet, "/healthz", "")

	assertStatus(t, rec, http.StatusOK)
	assertBody(t, rec, `{"status":"ok"}`)
}

func TestHealthRejectsWrongMethod(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestServer(t).router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/healthz", nil))

	assertStatus(t, rec, http.StatusMethodNotAllowed)
}
