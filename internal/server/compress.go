package server

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"sync"
)

// compressible names the media types that become much smaller with gzip.
// Images and fonts are already compressed, so a second pass only costs time.
var compressible = []string{
	"text/html",
	"text/css",
	"text/plain",
	"text/javascript",
	"application/javascript",
	"application/json",
	"application/xml",
	"image/svg+xml",
}

// writers keeps the gzip writers, because each one holds a large buffer.
var writers = sync.Pool{
	New: func() any {
		return gzip.NewWriter(io.Discard)
	},
}

// compress packs the answer when the browser accepts gzip and the answer is
// text. A request for a byte range passes through, because a range of the
// packed bytes means nothing to the browser.
func compress(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Accept-Encoding")

		accepted := strings.Contains(r.Header.Get("Accept-Encoding"), "gzip")
		if !accepted || r.Header.Get("Range") != "" {
			next.ServeHTTP(w, r)

			return
		}

		zip, ok := writers.Get().(*gzip.Writer)
		if !ok {
			next.ServeHTTP(w, r)

			return
		}

		defer writers.Put(zip)

		zip.Reset(w)

		writer := &gzipWriter{ResponseWriter: w, zip: zip}
		defer writer.finish()

		next.ServeHTTP(writer, r)
	})
}

// gzipWriter decides at the moment of the first byte whether the answer goes
// out packed, because only then does the content type stand.
type gzipWriter struct {
	http.ResponseWriter

	zip         *gzip.Writer
	wroteHeader bool
	packing     bool
}

func (w *gzipWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}

	w.wroteHeader = true

	if w.shouldPack(status) {
		w.packing = true

		w.Header().Del("Content-Length")
		w.Header().Set("Content-Encoding", "gzip")
	}

	w.ResponseWriter.WriteHeader(status)
}

func (w *gzipWriter) Write(content []byte) (int, error) {
	if !w.wroteHeader {
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", http.DetectContentType(content))
		}

		w.WriteHeader(http.StatusOK)
	}

	if w.packing {
		return w.zip.Write(content)
	}

	return w.ResponseWriter.Write(content)
}

// Flush sends what the gzip writer holds, so a long answer reaches the
// browser in parts.
func (w *gzipWriter) Flush() {
	if w.packing {
		_ = w.zip.Flush()
	}

	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// Unwrap gives the original writer to the standard library, which needs it
// for ResponseController.
func (w *gzipWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// shouldPack answers whether this status and content type deserve gzip.
func (w *gzipWriter) shouldPack(status int) bool {
	if status == http.StatusNoContent || status == http.StatusNotModified {
		return false
	}

	if w.Header().Get("Content-Encoding") != "" {
		return false
	}

	contentType := w.Header().Get("Content-Type")
	for _, packable := range compressible {
		if strings.HasPrefix(contentType, packable) {
			return true
		}
	}

	return false
}

// finish closes the gzip stream, which writes the last bytes.
func (w *gzipWriter) finish() {
	if w.packing {
		_ = w.zip.Close()
	}
}
