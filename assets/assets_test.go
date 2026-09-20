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

// TestTheThemeFollowsTheSystem guards the dark mode setup. The project has no
// theme switch, so the dark values must sit under the media query. A new
// version of the registry theme brings the class based variant back, and this
// test fails when that happens.
func TestTheThemeFollowsTheSystem(t *testing.T) {
	t.Parallel()

	content, err := fs.ReadFile(assets.FS, "css/globals.css")
	if err != nil {
		t.Fatalf("read the CSS entry file: %v", err)
	}

	css := string(content)

	if !strings.Contains(css, "@media (prefers-color-scheme: dark)") {
		t.Error("the dark values do not sit under the media query")
	}

	if strings.Contains(css, "@custom-variant dark") {
		t.Error("the class based dark variant is back, so the dark utilities need a .dark class")
	}
}
