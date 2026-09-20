#!/usr/bin/env bash
#
# Prepare the development container. The devcontainer runs this file once,
# after it creates the container.

set -euo pipefail

echo "Installing the command line tools."
go install github.com/go-task/task/v3/cmd/task@latest
go install github.com/axadrn/shadcn-templ/v2/cmd/shadcn-templ@latest

echo "Downloading the module dependencies."
go mod download

echo "Downloading the Tailwind binary."
task tools

echo "Done. Run 'task dev' and open http://localhost:8080."
