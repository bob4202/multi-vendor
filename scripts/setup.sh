#!/usr/bin/env bash
# Local development setup. Safe to run again: it never overwrites a value
# you've already set, and every step is idempotent.
#
#   scripts/setup.sh            env files + install + database + seed + storefront key
#   scripts/setup.sh --env-only just create/fill the .env files
#   scripts/setup.sh --reset    wipe the local database and seed fresh demo data
#                               (asks first; FORCE=1 skips the question)
#
# Run from the repo root (make setup / make env do that for you).
set -euo pipefail

cd "$(dirname "$0")/.."
BACKEND_ENV=backend/.env
FRONTEND_ENV=frontend/.env
# The seeded branch the local storefront shows.
DEMO_BRANCH_HOST=stratford-demo

say()  { printf '\033[1;36m==>\033[0m %s\n' "$*"; }
ok()   { printf '    \033[32m✓\033[0m %s\n' "$*"; }
skip() { printf '    \033[2m- %s\033[0m\n' "$*"; }
die()  { printf '\033[1;31merror:\033[0m %s\n' "$*" >&2; exit 1; }

# Value of KEY in an env file (empty if missing or blank).
get_env() { grep -E "^$2=" "$1" 2>/dev/null | tail -n1 | cut -d= -f2- || true; }

# Set KEY=VALUE in an env file, replacing the existing line or appending.
# Written via a temp file so it behaves the same on Linux and macOS.
set_env() {
  local file=$1 key=$2 value=$3 tmp
  tmp=$(mktemp)
  awk -v k="$key" -v v="$value" '
    $0 ~ "^"k"=" { print k"="v; done=1; next }
    { print }
    END { if (!done) print k"="v }
  ' "$file" >"$tmp"
  cat "$tmp" >"$file" && rm -f "$tmp"
}

require() {
  for cmd in "$@"; do
    command -v "$cmd" >/dev/null 2>&1 || die "$cmd is required but not installed"
  done
}

setup_env_files() {
  say "Environment files"
  require openssl

  for pair in "$BACKEND_ENV:backend/.env.example" "$FRONTEND_ENV:frontend/.env.example"; do
    local env=${pair%%:*} example=${pair#*:}
    if [[ -f $env ]]; then
      skip "$env already exists, keeping it"
    else
      cp "$example" "$env"
      chmod 600 "$env" # holds secrets
      ok "created $env from $example"
    fi
  done

  if [[ -z $(get_env "$BACKEND_ENV" APP_ENCRYPTION_KEY) ]]; then
    set_env "$BACKEND_ENV" APP_ENCRYPTION_KEY "$(openssl rand -base64 32)"
    ok "generated APP_ENCRYPTION_KEY"
  else
    skip "APP_ENCRYPTION_KEY already set"
  fi

  local admin_key
  admin_key=$(get_env "$BACKEND_ENV" ADMIN_API_KEY)
  if [[ -z $admin_key || $admin_key == change-me ]]; then
    set_env "$BACKEND_ENV" ADMIN_API_KEY "$(openssl rand -hex 32)"
    ok "generated ADMIN_API_KEY"
  else
    skip "ADMIN_API_KEY already set"
  fi

  # Web Push keys. Generated once; changing them later breaks every
  # browser's existing subscription.
  if [[ -z $(get_env "$BACKEND_ENV" VAPID_PRIVATE_KEY) ]]; then
    local vapid
    vapid=$(cd backend && go run ./cmd/vapid)
    set_env "$BACKEND_ENV" VAPID_PUBLIC_KEY "$(grep '^VAPID_PUBLIC_KEY=' <<<"$vapid" | cut -d= -f2-)"
    set_env "$BACKEND_ENV" VAPID_PRIVATE_KEY "$(grep '^VAPID_PRIVATE_KEY=' <<<"$vapid" | cut -d= -f2-)"
    ok "generated VAPID keys (push notifications)"
  else
    skip "VAPID keys already set"
  fi

  # The frontend calls the API on the backend's own address.
  if [[ -z $(get_env "$FRONTEND_ENV" BASE_URL) ]]; then
    local addr port
    addr=$(get_env "$BACKEND_ENV" HTTP_ADDR)
    port=${addr##*:}
    set_env "$FRONTEND_ENV" BASE_URL "http://localhost:${port:-8080}/"
    ok "set BASE_URL"
  fi
}

wait_for_postgres() {
  local compose="docker compose -f backend/docker-compose.yml"
  for _ in $(seq 1 30); do
    if $compose exec -T postgres pg_isready -U postgres >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  die "Postgres didn't become ready in 30s (check: docker compose -f backend/docker-compose.yml logs)"
}

setup_storefront_key() {
  say "Storefront branch for the local shop"
  if [[ ${1:-} != --force && -n $(get_env "$FRONTEND_ENV" STOREFRONT_KEY) && -n $(get_env "$FRONTEND_ENV" BRANCH_ID) ]]; then
    skip "BRANCH_ID and STOREFRONT_KEY already set"
    return
  fi
  # Issues a fresh key for the demo branch (keys are only stored hashed,
  # so an earlier one can't be read back) and prints the two .env lines.
  local lines
  lines=$(cd backend && set -a && . ./.env && set +a && go run ./cmd/admin storefront-key --host "$DEMO_BRANCH_HOST")
  set_env "$FRONTEND_ENV" BRANCH_ID "$(grep '^BRANCH_ID=' <<<"$lines" | cut -d= -f2-)"
  set_env "$FRONTEND_ENV" STOREFRONT_KEY "$(grep '^STOREFRONT_KEY=' <<<"$lines" | cut -d= -f2-)"
  ok "storefront set to the demo branch ($DEMO_BRANCH_HOST)"
}

# Wipes every table in the local database (the whole public schema), then
# migrates and seeds from scratch. Only ever touches a database on this
# machine, and only after the name is typed back.
reset_database() {
  local url host db
  url=$(get_env "$BACKEND_ENV" DATABASE_URL)
  [[ -n $url ]] || die "DATABASE_URL isn't set in $BACKEND_ENV"
  host=$(sed -E 's#^[a-z]+://([^@/]*@)?([^:/?]+).*#\2#' <<<"$url")
  db=$(sed -E 's#^.*/([^/?]+)(\?.*)?$#\1#' <<<"$url")
  if [[ $host != localhost && $host != 127.0.0.1 ]]; then
    die "refusing to reset a database that isn't local (host: $host)"
  fi

  say "Reset database \"$db\" on $host"
  if [[ ${FORCE:-} != 1 ]]; then
    printf '    This deletes ALL data in "%s" (orders, vendors, everything) and\n' "$db"
    printf '    reseeds demo data. Type the database name to continue: '
    local answer
    read -r answer
    [[ $answer == "$db" ]] || die "cancelled, nothing was changed"
  fi

  make --no-print-directory db-up
  wait_for_postgres
  docker compose -f backend/docker-compose.yml exec -T postgres \
    psql -q -v ON_ERROR_STOP=1 -U postgres -d "$db" \
    -c 'DROP SCHEMA public CASCADE' -c 'CREATE SCHEMA public' >/dev/null
  ok "wiped $db"

  say "Migrating and seeding the database"
  make --no-print-directory migrate
  make --no-print-directory seed

  # Fresh rows mean new branch ids and keys: always re-point the storefront.
  setup_storefront_key --force
  say "Fresh demo data is ready. Restart make dev if it was running."
}

setup_env_files
if [[ ${1:-} == --reset ]]; then
  require docker go
  reset_database
  exit 0
fi
if [[ ${1:-} == --env-only ]]; then
  say "Done. Edit $BACKEND_ENV and $FRONTEND_ENV if you need to change anything."
  exit 0
fi

require go npm docker

say "Installing dependencies"
make --no-print-directory install

say "Starting Postgres"
make --no-print-directory db-up
wait_for_postgres
ok "Postgres is ready"

say "Migrating and seeding the database"
make --no-print-directory migrate
make --no-print-directory seed

setup_storefront_key

say "All set. Start everything with: make dev"
cat <<'MSG'
    Shop:          http://localhost:3000
    Vendor portal: http://localhost:3000/vendor   (owner@group.example)
    Admin:         http://localhost:3000/admin    (admin@groceryshop.example)
    Password for the demo logins: see the seed output above (default shop-demo-2026)
MSG
