#!/bin/sh
set -eu

project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
if [ -x "$project_dir/.tools/go/bin/go" ]; then
    PATH="$project_dir/.tools/go/bin:$PATH"
    export PATH
    exec "$project_dir/.tools/go/bin/go" "$@"
fi
if command -v go >/dev/null 2>&1; then
    exec go "$@"
fi
printf '%s\n' 'Go 1.27.3+ is required. Install Go, or extract the official archive into .tools/go.' >&2
exit 1
