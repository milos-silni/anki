#!/usr/bin/env bash
# Install or update anki as a systemd service, mirroring the family-guy setup.
# Usage (from the repo root):  sudo ./deploy/install.sh
set -euo pipefail

cd "$(dirname "$0")/.."

if [ ! -x ./anki ]; then
	echo "build first: CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o anki ." >&2
	exit 1
fi

mkdir -p /opt/anki /var/lib/anki
chown milos:milos /var/lib/anki

# Seed the data file once; never overwrite existing cards on updates.
if [ ! -e /var/lib/anki/cards.json ] && [ -e ./cards.json ]; then
	install -m 644 -o milos -g milos ./cards.json /var/lib/anki/cards.json
fi

# Login password, asked for once and kept in a root-only file.
if [ ! -s /etc/anki/anki.env ]; then
	read -rsp "Choose a login password for anki: " pw; echo
	[ -n "$pw" ] || { echo "password must not be empty" >&2; exit 1; }
	# systemd's EnvironmentFile would mangle these
	case "$pw" in *[\'\"\\\$]* | [[:space:]]* | *[[:space:]]) echo "password must not contain quotes, backslashes or \$, or start/end with a space" >&2; exit 1 ;; esac
	install -d -m 700 /etc/anki
	(umask 077; printf 'ANKI_PASSWORD=%s\n' "$pw" > /etc/anki/anki.env)
fi

install -m 755 ./anki /opt/anki/anki
install -m 644 ./deploy/anki.service /etc/systemd/system/anki.service

systemctl daemon-reload
systemctl enable anki
systemctl restart anki
sleep 1
systemctl --no-pager --lines=10 status anki
