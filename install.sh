#!/usr/bin/env bash
set -euo pipefail

cd -- "$(dirname -- "${BASH_SOURCE[0]}")"
docker compose version >/dev/null

umask 077
mkdir -p data secrets
if ! grep -Eq '^QBT_API_KEY=[^[:space:]]' secrets/.env 2>/dev/null; then
  read -r -s -p 'Clave API de qBittorrent: ' api_key
  printf '\n'
  if [[ -z "$api_key" || "$api_key" =~ [[:space:]] ]]; then
    printf 'Introduce una clave API válida.\n' >&2
    exit 1
  fi
  printf '\nQBT_API_KEY=%s\n' "$api_key" >> secrets/.env
  unset api_key
fi
chmod 600 secrets/.env

touch .env
for setting in "PANEL_UID=$(id -u)" "PANEL_GID=$(id -g)"; do
  if ! grep -Eq "^[[:space:]]*(export[[:space:]]+)?${setting%%=*}[[:space:]]*=" .env; then
    printf '\n%s\n' "$setting" >> .env
  fi
done

image=$(docker compose config --images)
if [[ "$image" == seed-panel:local ]]; then
  docker build -t "$image" .
elif [[ "$image" == coronel845sagaz/seed-panel:latest ]]; then
  docker compose pull panel
fi
docker compose up -d --no-build
printf 'Panel disponible en el puerto publicado por Docker:\n'
docker compose port panel 8080
