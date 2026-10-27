#!/bin/sh
set -eu
kit_root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
if [ "$(id -u)" -ne 0 ]; then exec sudo sh "$kit_root/Cloud-Update.sh" "$@"; fi
chmod u+x "$kit_root/bin/layerhub-update-linux-amd64"
exec "$kit_root/bin/layerhub-update-linux-amd64" cloud --channel beta --apply "$@"
