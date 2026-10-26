#!/bin/sh
# Fresh native Debian/Ubuntu amd64 installation only; never upgrades existing data.
set -eu
export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
test "$(id -u)" = 0 || { echo 'Run with sudo.' >&2; exit 1; }
here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
bundle=$(CDPATH= cd -- "$here/../.." && pwd)
test "$(uname -m)" = x86_64 || { echo 'This package contains amd64 binaries only.' >&2; exit 1; }
if test -e /opt/layerhub/current || test -L /opt/layerhub/current || test -e /etc/layerhub/layerhub.env || test -e /var/lib/layerhub/data || systemctl cat layerhub.service >/dev/null 2>&1; then
    echo 'Existing LayerHub installation detected. No changes made; use the upgrade guide.' >&2
    exit 1
fi
if getent passwd layerhub >/dev/null; then
    echo 'Existing layerhub account requires review. No changes made.' >&2
    exit 1
fi
binary="$bundle/binaries/linux-amd64/layerhub"
web="$bundle/source/web"
if test -f "$bundle/bin/layerhub"; then
    binary="$bundle/bin/layerhub"
    web="$bundle/web"
fi
test -f "$binary"
test -f "$web/source/layerhub-source.tar.gz"
target=/opt/layerhub/releases/v0.1.0-alpha
test ! -e "$target"
useradd --system --user-group --home-dir /var/lib/layerhub --create-home --shell /usr/sbin/nologin layerhub
install -d -m 755 /opt/layerhub /opt/layerhub/releases "$target" "$target/bin"
install -m 755 "$binary" "$target/bin/layerhub"
cp -R "$web" "$target/web"
find "$target/web" -type d -exec chmod 755 {} +
find "$target/web" -type f -exec chmod 644 {} +
ln -s "$target" /opt/layerhub/current
install -d -m 700 -o layerhub -g layerhub /var/lib/layerhub
install -d -m 700 /etc/layerhub
install -m 600 "$here/layerhub.env.example" /etc/layerhub/layerhub.env
install -m 644 "$here/layerhub.service" /etc/systemd/system/layerhub.service
systemd-analyze verify /etc/systemd/system/layerhub.service
systemctl daemon-reload
systemctl enable --now layerhub.service
printf '%s\n' 'Installed in offline mode; print start is disabled.' 'Health: curl --noproxy "*" http://127.0.0.1:5181/healthz' 'Initial credentials (local root only): sudo cat /var/lib/layerhub/data/bootstrap-admin.txt'
