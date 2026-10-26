#!/bin/sh
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root/source"
command -v go >/dev/null || { echo 'Install Go 1.27+ first.' >&2; exit 1; }
export CGO_ENABLED=0
mkdir -p "$root/binaries/linux-amd64" "$root/binaries/windows-amd64"
GOOS=linux GOARCH=amd64 go build -mod=vendor -trimpath -o "$root/binaries/linux-amd64/layerhub" ./cmd/server
GOOS=linux GOARCH=amd64 go build -mod=vendor -trimpath -o "$root/binaries/linux-amd64/layerhub-mail-worker" ./cmd/mail-worker
GOOS=windows GOARCH=amd64 go build -mod=vendor -trimpath -o "$root/binaries/windows-amd64/layerhub.exe" ./cmd/server
python3 tools/make-source.py
printf '%s\n' 'Binaries rebuilt. Recreate portable archives after changing source or web assets.'
