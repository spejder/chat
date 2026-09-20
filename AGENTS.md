# Notes for coding agents

Rules and facts about this repository that are not obvious from the code.
Read this file before you change anything. Add a note here when you learn
something that the next agent needs.

## Always do this before you commit Go code

```
task fix    # go fix ./..., the modernizers of Go 1.27
task lint   # golangci-lint run
task fmt    # templ fmt, then golangci-lint fmt
task build  # generate, css, go build
```

`go fix` is not only the old API rewriter. Since Go 1.27 it applies the
modernize analyzers, for example `slices.Backward`, `maps.Copy` and
`new(expr)`. Run it on every Go change, including on the component code that
shadcn-templ copies into the project.

## Tools

- templ is a tool of this module. Call `go tool templ generate`, never a global
  `templ`. The version lives in `go.mod`.
- Tailwind and golangci-lint are binaries in `./bin`. The `tools` task
  downloads both. Git ignores `./bin`. Both versions are pinned in
  `Taskfile.yml`. Do not call a golangci-lint from the PATH, because its
  version can differ from the pinned one.
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

## Web platform rules

This project's Baseline target is Baseline Widely available. Baseline is the
set of web features that every major browser supports.

- Prefer a native element over a script: `<dialog>`, `popover`, `<details>`,
  and the matching `<input type="...">`.
- Do not add a polyfill. If a feature sits outside the target, write the
  fallback by hand or leave the feature out.
- Do not use `hx-on`. It turns a string into code, and the Content Security
  Policy blocks that.
- Do not rewrite the markup of a registry component. Wrap it instead.
- The guidance comes from the Chrome plugin `modern-web-guidance`. Follow it
  where it does not fight templ or shadcn-templ.

## Dark mode

The page follows the operating system. There is no switch and nothing is
stored.

- `assets/css/globals.css` holds the dark values in
  `@media (prefers-color-scheme: dark) { :root { ... } }`, after the light
  values. Both blocks have the same specificity, so the later one wins.
- The file must not hold `@custom-variant dark`. That line binds the `dark:`
  utilities to a `.dark` class, and nothing sets that class. Without the line,
  Tailwind uses the media query, which is what we want.
- A new version of the registry theme writes the class based setup again.
  `assets/assets_test.go` fails when that happens.
- `assets/img/favicon.svg` carries its own media query, and the layout sets
  `<meta name="color-scheme" content="light dark">`, so form controls and
  scrollbars follow too.

## The HTTP answer

`internal/server/headers.go` sets the Content Security Policy and three more
headers on every answer. Every source is `'self'`. The one relaxation is
`style-src-attr 'unsafe-inline'`, because registry components write CSS
variables into a `style` attribute. Do not widen the policy for anything else.
Read the browser console after you add a component.

`internal/server/compress.go` packs text answers with gzip. It leaves images,
fonts and range requests alone.

`assets.Integrity(path)` returns the value for the integrity attribute of the
stylesheet and the script. The browser hashes the file it received and refuses
it when the two hashes differ. The value and the address come from the same
bytes at program start, so they can never disagree. An unknown path gives an
empty value, because a wrong value blocks the file. The icon link carries no
integrity attribute, because the browser reads that attribute only for scripts
and stylesheets.

`assets.URL(path)` returns the address of a static file with a hash of the
content. The layout calls it. A request that carries the current hash gets a
cache time of one year. Every other request gets 60 seconds and an ETag.

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

## Linting

`.golangci.yml` uses the version 2 file format. It holds the standard linters
plus a set of stricter ones, and `gofumpt` and `goimports` as formatters.

- The generated `*_templ.go` files carry a "Code generated" header, and the
  `generated: lax` setting keeps them out of the reports.
- The code under `internal/components` and `internal/utils` comes from the
  shadcn-templ registry. The rules that only report style, `revive`,
  `gocritic` and `unparam`, are off for those paths, because an upgrade
  overwrites the code. The rules that report real faults stay on. A finding
  there comes back after every upgrade, so fix it again.
- `golangci-lint run` includes `govet`. Do not add a separate `go vet` step.

## Tests

- Test files live next to the code in an external package, for example
  `package server_test`. The one exception is `cmd/chat/main_test.go`, because
  `defaultAddr` is unexported.
- Build a request with `httptest.NewRequestWithContext(t.Context(), ...)`. The
  `noctx` linter refuses `httptest.NewRequest`.
- The route tests read the rendered markup, so they also cover the templates
  and the htmx attributes. A wrong `hx-post` target fails the build.
- `assets/assets_test.go` holds the htmx version. Change that test when you
  update `assets/js/htmx.min.js`.
- Do not write tests for `internal/components` and `internal/utils`. That code
  comes from the shadcn-templ registry.
- `task test` runs `task generate` first, so a changed `.templ` file is always
  part of the run.

## Markup validation

`task html` renders the pages with `cmd/renderhtml` into `tmp/html` and checks
them with html-validate.

- The version is pinned to 10 in `Taskfile.yml`, because version 11 needs a
  newer Node than the development container holds.
- `.htmlvalidate.json` turns `doctype-style` off. templ writes
  `<!doctype html>` in lower case, which is valid, and templ owns that output.
- The rule `no-unknown-attributes` stays off. It reports every `hx-` attribute,
  and the element metadata of html-validate has no pattern that covers all
  elements at once.
- `cmd/renderhtml` wraps a fragment in a small document, because a validator
  reads a file as a whole document.

## Continuous integration

`.github/workflows/ci.yml` runs four jobs on every push to main and on every
pull request. The build job runs `task build` and `task test`. The lint job
runs `task lint`, `task fmt` and `task fix`. The markup job runs `task html`.
The Lighthouse job starts the binary and audits the page against the
thresholds in `.github/lighthouserc.json`.

Both jobs end with `git diff --exit-code`. A pull request fails when the
generated templ code, the format or the modernizers produce a change. Run
`task build`, `task fmt` and `task fix` and commit the result before you push.

The jobs cache `./bin` under a key that holds the hash of `Taskfile.yml`, so a
version change in that file downloads the tools again.

## Development container

`.devcontainer/devcontainer.json` uses the Go 1.27 image and runs
`.devcontainer/post-create.sh` once. The script installs `task` and
`shadcn-templ`, downloads the dependencies and calls `task tools`.

The `tools` task reads `uname` and downloads the Tailwind binary for the
current operating system and processor. Keep it that way. A container on an
Apple computer runs on arm64.

Forwarded ports: 8080 is the server, 7331 is the templ proxy that reloads the
browser during `task dev`.

`.devcontainer/devcontainer-lock.json` pins the feature to one digest. The
devcontainer command writes it. Commit every change of the file.

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
