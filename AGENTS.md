# Notes for coding agents

Rules and facts about this repository that are not obvious from the code.
Read this file before you change anything. Add a note here when you learn
something that the next agent needs.

## Always do this before you commit Go code

```
task fix    # go fix ./..., the modernizers of Go 1.27
task lint   # go vet ./...
task fmt    # templ fmt and go fmt
task build  # generate, css, go build
```

`go fix` is not only the old API rewriter. Since Go 1.27 it applies the
modernize analyzers, for example `slices.Backward`, `maps.Copy` and
`new(expr)`. Run it on every Go change, including on the component code that
shadcn-templ copies into the project.

## Tools

- templ is a tool of this module. Call `go tool templ generate`, never a global
  `templ`. The version lives in `go.mod`.
- Tailwind is the standalone binary in `./bin`. The `tools` task downloads it.
  Git ignores `./bin`. The version is pinned in `Taskfile.yml`.
- `task` and `shadcn-templ` are installed with `go install`. See `README.md`.

## templ

- The generated files `*_templ.go` are committed. Run `task generate` after you
  edit a `.templ` file, or the build uses stale code.
- Write the shared HTML shell in `internal/web/layout.templ`. Pages use it with
  `@Layout("title") { ... }`.
- Handlers return a full page for a normal request and a fragment for an htmx
  request. Fragments live next to the page that uses them.

## shadcn-templ

- `shadcn-templ add <name>` writes to `internal/components/<name>`, because
  `components.json` sets the aliases to `github.com/spejder/chat/internal/...`.
  Keep those aliases. Run `task generate` and `go mod tidy` after each add.
- The copied code belongs to this project. You may edit it, and `task fix` does
  edit it. An upgrade of a component overwrites your edits, so run `task fix`
  again after an upgrade.
- Components take a `Props` struct. Pass htmx attributes through
  `Attributes: templ.Attributes{...}`.

## htmx

- The version is 4.0.0, vendored in `assets/js/htmx.min.js`. The file name
  carries no version.
- htmx 4 removed implicit attribute inheritance. Put every `hx-` attribute on
  the element that sends the request, or use the `:inherited` suffix.
- Event names changed to `htmx:before:request` and `htmx:before:swap`. Do not
  copy htmx 2 event names from older examples.

## Assets and embedding

- `assets/assets.go` embeds `css`, `js` and `dist` into the binary.
- `assets/dist/styles.css` is generated and ignored by git. The committed file
  `assets/dist/.gitkeep` keeps the embed pattern valid on a fresh clone.
- Build the CSS before `go build`, or the binary holds an old stylesheet. The
  `build` task does this in the right order.
- The CSS entry file starts with `@import "tailwindcss" source(none);`, so the
  automatic file search is off. Tailwind reads only the `@source` lines of
  `assets/css/globals.css`. Add a line when you put templ files outside
  `internal`. With the automatic search on, words from the Markdown files
  become class names and the output changes from machine to machine.

## Development container

`.devcontainer/devcontainer.json` uses the Go 1.27 image and runs
`.devcontainer/post-create.sh` once. The script installs `task` and
`shadcn-templ`, downloads the dependencies and calls `task tools`.

The `tools` task reads `uname` and downloads the Tailwind binary for the
current operating system and processor. Keep it that way. A container on an
Apple computer runs on arm64.

Forwarded ports: 8080 is the server, 7331 is the templ proxy that reloads the
browser during `task dev`.

Do not mount a named volume on `/go/pkg/mod`. The image has no such directory,
so Docker creates it and gives it to root, and `go install` then fails with
`mkdir /go/pkg/mod/cache: permission denied`.

## Verification

Do not stop at a green build. Start the server on a free port, request the page
and the fragment with curl, then open the page in the browser pane and click
the control. Read the browser console for errors.

## Shell

Do not stop a background process with `pkill -f '<pattern>'` when the pattern
also matches the command line of your own shell. The shell dies, and the tool
call fails with a confusing exit code. Write `pkill -f '[/]chat -addr'` or kill
the process id.

## Commits

Write the subject line in the imperative. Explain why in the body. End the
message with the line `Assisted-by: Claude <noreply@anthropic.com>`.
