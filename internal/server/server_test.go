package server_test

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/spejder/chat/assets"
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
				`data-hx-post="/greet"`,
				`data-hx-target="#greeting"`,
				`id="greeting"`,
				"/assets/js/htmx.min.js",
				"/assets/dist/styles.css",
				`integrity="sha256-`,
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

// TestAssetCacheHeaders covers the two answers for a static file. An address
// with the current hash can never point at other content, so the browser may
// keep it for a year. An address without the hash gets a short time.
func TestAssetCacheHeaders(t *testing.T) {
	t.Parallel()

	const path = "js/htmx.min.js"

	hash, known := assets.Fingerprint(path)
	if !known {
		t.Fatalf("the binary holds no %s", path)
	}

	tests := []struct {
		name   string
		target string
		want   string
	}{
		{
			name:   "the address without a hash",
			target: "/assets/" + path,
			want:   "public, max-age=60",
		},
		{
			name:   "the address with the current hash",
			target: "/assets/" + path + "?v=" + hash,
			want:   "public, max-age=31536000, immutable",
		},
		{
			name:   "the address with an old hash",
			target: "/assets/" + path + "?v=000000000000",
			want:   "public, max-age=60",
		},
	}

	handler := server.New()

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, test.target, nil))

			if got := recorder.Header().Get("Cache-Control"); got != test.want {
				t.Errorf("Cache-Control = %q, want %q", got, test.want)
			}

			if got := recorder.Header().Get("ETag"); got != strconv.Quote(hash) {
				t.Errorf("ETag = %q, want %q", got, strconv.Quote(hash))
			}
		})
	}
}

// TestAKnownAssetIsNotSentTwice makes sure that a browser with the file
// already in hand gets a short answer.
func TestAKnownAssetIsNotSentTwice(t *testing.T) {
	t.Parallel()

	hash, known := assets.Fingerprint("js/htmx.min.js")
	if !known {
		t.Fatal("the binary holds no js/htmx.min.js")
	}

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/assets/js/htmx.min.js", nil)
	request.Header.Set("If-None-Match", strconv.Quote(hash))

	recorder := httptest.NewRecorder()
	server.New().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotModified {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusNotModified)
	}

	if recorder.Body.Len() != 0 {
		t.Errorf("the answer carries %d bytes, want none", recorder.Body.Len())
	}
}

// TestSecurityHeaders makes sure that every answer limits what the page may
// do and where it may load from.
func TestSecurityHeaders(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	server.New().ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))

	want := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
		"Permissions-Policy":     "camera=(), microphone=(), geolocation=()",
	}

	for header, value := range want {
		if got := recorder.Header().Get(header); got != value {
			t.Errorf("%s = %q, want %q", header, got, value)
		}
	}

	policy := recorder.Header().Get("Content-Security-Policy")
	for _, directive := range []string{
		"default-src 'self'",
		"script-src 'self'",
		"frame-ancestors 'none'",
		"object-src 'none'",
	} {
		if !strings.Contains(policy, directive) {
			t.Errorf("the policy misses %q\npolicy: %s", directive, policy)
		}
	}
}

// TestCompression makes sure that a browser that accepts gzip receives the
// page packed, and that a browser without that header receives it plain.
func TestCompression(t *testing.T) {
	t.Parallel()

	t.Run("the page goes out packed", func(t *testing.T) {
		t.Parallel()

		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
		request.Header.Set("Accept-Encoding", "gzip")

		recorder := httptest.NewRecorder()
		server.New().ServeHTTP(recorder, request)

		if got := recorder.Header().Get("Content-Encoding"); got != "gzip" {
			t.Fatalf("Content-Encoding = %q, want %q", got, "gzip")
		}

		if got := recorder.Header().Get("Vary"); !strings.Contains(got, "Accept-Encoding") {
			t.Errorf("Vary = %q, want it to hold Accept-Encoding", got)
		}

		reader, err := gzip.NewReader(recorder.Body)
		if err != nil {
			t.Fatalf("read the packed answer: %v", err)
		}

		body, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("unpack the answer: %v", err)
		}

		if !strings.Contains(string(body), "Hello world") {
			t.Errorf("the unpacked answer misses the page: %s", body)
		}
	})

	t.Run("a browser without the header gets plain bytes", func(t *testing.T) {
		t.Parallel()

		recorder := httptest.NewRecorder()
		server.New().ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))

		if got := recorder.Header().Get("Content-Encoding"); got != "" {
			t.Errorf("Content-Encoding = %q, want none", got)
		}

		if !strings.Contains(recorder.Body.String(), "Hello world") {
			t.Error("the answer misses the page")
		}
	})
}
