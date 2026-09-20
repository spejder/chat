// Command renderhtml writes the pages and the fragments of the application
// into a directory, so an HTML validator can read them as files.
//
// Usage: go run ./cmd/renderhtml [directory]
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/a-h/templ"

	"github.com/spejder/chat/internal/web"
)

// fragmentShell puts a fragment into a legal document, because a validator
// reads a file as a whole document.
const fragmentShell = `<!DOCTYPE html>
<html lang="en">
<head><meta charset="utf-8"><title>Fragment</title></head>
<body>%s</body>
</html>
`

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "renderhtml: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	dir := "tmp/html"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("make the directory: %w", err)
	}

	// The time is fixed, so two runs write the same bytes.
	at := time.Date(2026, time.September, 20, 13, 45, 7, 0, time.UTC)

	pages := map[string]templ.Component{
		"home.html": web.Home(),
	}

	fragments := map[string]templ.Component{
		"greeting.html": web.Greeting(at),
	}

	for name, page := range pages {
		markup, err := render(page)
		if err != nil {
			return fmt.Errorf("render %s: %w", name, err)
		}

		if err := write(filepath.Join(dir, name), markup); err != nil {
			return err
		}
	}

	for name, fragment := range fragments {
		markup, err := render(fragment)
		if err != nil {
			return fmt.Errorf("render %s: %w", name, err)
		}

		if err := write(filepath.Join(dir, name), fmt.Sprintf(fragmentShell, markup)); err != nil {
			return err
		}
	}

	return nil
}

// render turns a component into markup.
func render(component templ.Component) (string, error) {
	var out strings.Builder

	if err := component.Render(context.Background(), &out); err != nil {
		return "", err
	}

	return out.String(), nil
}

// write puts the markup into a file.
func write(path, markup string) error {
	if err := os.WriteFile(path, []byte(markup), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}

	return nil
}
