package assets_test

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/spejder/chat/assets"
)

// TestHtmxIsTheVersionInTheDocuments makes sure that the vendored file is the
// version that README.md names. The file name carries no version.
func TestHtmxIsTheVersionInTheDocuments(t *testing.T) {
	t.Parallel()

	content, err := fs.ReadFile(assets.FS, "js/htmx.min.js")
	if err != nil {
		t.Fatalf("read the htmx file: %v", err)
	}

	if !strings.Contains(string(content), `version="4.0.0`) {
		t.Error("the htmx file is not version 4.0.0")
	}
}

// TestTheStylesheetEntryIsEmbedded makes sure that the CSS directory travels
// with the binary.
func TestTheStylesheetEntryIsEmbedded(t *testing.T) {
	t.Parallel()

	if _, err := fs.Stat(assets.FS, "css/globals.css"); err != nil {
		t.Errorf("the CSS entry file is missing: %v", err)
	}
}
