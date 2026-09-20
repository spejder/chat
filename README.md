# chat

A web based chat application. The server is written in Go. The pages are
rendered on the server with templ, styled with shadcn-templ components, and
updated in the browser with htmx.

This is step one. The application shows one page with a button. A click on the
button sends a request, and the server answers with an HTML fragment.

## Technology

| Part            | Choice       | Version |
| --------------- | ------------ | ------- |
| Language        | Go           | 1.27    |
| Templates       | templ        | 0.3     |
| Components      | shadcn-templ | 2.0     |
| Styling         | Tailwind CSS | 4.3.3   |
| Browser updates | htmx         | 4.0.0   |
| Linting         | golangci-lint | 2.13.2 |

The file `assets/js/htmx.min.js` is htmx 4.0.0. The name carries no version, so
read the version here. htmx 4 is not the default version on npm, so every
reference must name 4.0.0.

All static files are compiled into the binary with `go:embed`. The program runs
with no other files next to it.

## Prerequisites

Install Go 1.27 or later. Then install the two command line tools:

```
go install github.com/go-task/task/v3/cmd/task@latest
go install github.com/axadrn/shadcn-templ/v2/cmd/shadcn-templ@latest
```

The templ command is a Go tool of this module, so `go tool templ` always uses
the version in `go.mod`. The Tailwind command is downloaded to `./bin` by the
`tools` task, and git ignores that directory.

## Development container

The repository holds a devcontainer. Open the folder in an editor that supports
devcontainers, or start it from the command line:

```
devcontainer up --workspace-folder .
```

The container is the Go 1.27 image. It installs `task` and `shadcn-templ`,
downloads the module dependencies and the Tailwind binary, and forwards the
ports 8080 and 7331.

With the devcontainer you can skip the next section.

## Development

Start the server and the file watchers:

```
task dev
```

Open `http://localhost:8080`. The templ watcher reloads the browser after each
change.

Build the binary:

```
task build
```

Check the code before you commit:

```
task fix
task lint
task fmt
```

Run `task` alone to see all tasks.

## Add a component

```
shadcn-templ add <name>
task generate
go mod tidy
```

The command copies the source into `internal/components`, so the project owns
the code and you can edit it.

## Layout

| Path                  | Content                                   |
| --------------------- | ----------------------------------------- |
| `cmd/chat`            | The program that starts the server        |
| `internal/server`     | Routes and HTTP handlers                  |
| `internal/web`        | Page templates and fragments              |
| `internal/components` | shadcn-templ components                   |
| `internal/utils`      | Helpers that the components need          |
| `assets`              | CSS, JavaScript and the embed declaration |
