package server_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spejder/chat/internal/server"
)

// TestRoutes sends one request per route and reads the answer.
func TestRoutes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		method     string
		target     string
		wantStatus int
		wantBody   []string
	}{
		{
			name:       "the page holds the greeting target and the htmx script",
			method:     http.MethodGet,
			target:     "/",
			wantStatus: http.StatusOK,
			wantBody: []string{
				"Hello world",
				`hx-post="/greet"`,
				`hx-target="#greeting"`,
				`id="greeting"`,
				"/assets/js/htmx.min.js",
				"/assets/dist/styles.css",
			},
		},
		{
			name:       "the greeting is a fragment without a page around it",
			method:     http.MethodPost,
			target:     "/greet",
			wantStatus: http.StatusOK,
			wantBody:   []string{"Hello from the server at "},
		},
		{
			name:       "the htmx file is served from the binary",
			method:     http.MethodGet,
			target:     "/assets/js/htmx.min.js",
			wantStatus: http.StatusOK,
			wantBody:   []string{`version="4.0.0`},
		},
		{
			name:       "an unknown path answers with not found",
			method:     http.MethodGet,
			target:     "/does-not-exist",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "the page does not answer a post",
			method:     http.MethodPost,
			target:     "/",
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "the greeting does not answer a get",
			method:     http.MethodGet,
			target:     "/greet",
			wantStatus: http.StatusMethodNotAllowed,
		},
	}

	handler := server.New()

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), test.method, test.target, nil))

			if recorder.Code != test.wantStatus {
				t.Errorf("status = %d, want %d", recorder.Code, test.wantStatus)
			}

			body := recorder.Body.String()
			for _, want := range test.wantBody {
				if !strings.Contains(body, want) {
					t.Errorf("body does not hold %q\nbody: %s", want, body)
				}
			}
		})
	}
}

// TestGreetingIsAFragment makes sure that the htmx answer carries no page
// shell. A full document in the answer would replace the target with a second
// copy of the page.
func TestGreetingIsAFragment(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	server.New().ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/greet", nil))

	body := recorder.Body.String()
	for _, unwanted := range []string{"<html", "<body", "<!doctype"} {
		if strings.Contains(strings.ToLower(body), unwanted) {
			t.Errorf("the fragment holds %q\nbody: %s", unwanted, body)
		}
	}

	if got := recorder.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q, want %q", got, "text/html; charset=utf-8")
	}
}

// TestAssetsCarryACacheHeader makes sure that a browser keeps the static files.
func TestAssetsCarryACacheHeader(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	server.New().ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/assets/js/htmx.min.js", nil))

	if got := recorder.Header().Get("Cache-Control"); got != "public, max-age=3600" {
		t.Errorf("Cache-Control = %q, want %q", got, "public, max-age=3600")
	}
}
