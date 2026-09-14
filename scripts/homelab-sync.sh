#!/usr/bin/env bash
# homelab: влить релиз upstream (YouROK/TorrServer) в ветку homelab. Подробности — HOMELAB.md.
#
#   scripts/homelab-sync.sh [--web] [<тег>]   без тега — последний релиз MatriX.*
#   scripts/homelab-sync.sh --check           только проверить, что крючки на месте, и прогнать тесты
#
# --web — ещё и собрать веб (нужны node 16 и yarn, как в Dockerfile upstream).
# В GitHub Actions пишет в $GITHUB_OUTPUT: status=merged|uptodate|conflict|failed, tag, details.
# Коммит слияния остаётся локальным: push делает вызывающий (workflow или человек).
set -uo pipefail

UPSTREAM_URL=https://github.com/YouROK/TorrServer.git
WEB=0
CHECK_ONLY=0
TAG=""
for arg in "$@"; do
  case "$arg" in
    --web) WEB=1 ;;
    --check) CHECK_ONLY=1 ;;
    *) TAG="$arg" ;;
  esac
done

cd "$(git rev-parse --show-toplevel)" || exit 1

out() { [ -n "${GITHUB_OUTPUT:-}" ] && printf '%s\n' "$@" >> "$GITHUB_OUTPUT"; return 0; }
finish() { # status details
  out "status=$1" "tag=$TAG" "details<<EOF_DETAILS" "$2" "EOF_DETAILS"
  echo "== $1: $2"
  [ "$1" = merged ] || [ "$1" = uptodate ] || [ "$1" = checked ]
  exit $?
}

# Крючки в файлах upstream: файл → сколько строк с пометкой homelab там должно быть.
# Меняешь крючки — поправь и этот список, и таблицу в HOMELAB.md.
HOOKS=(
  "server/torr/storage/torrstor/cache.go:6"
  "server/torr/storage/torrstor/reader.go:1"
  "server/server.go:1"
  "server/torr/apihelper.go:2"
  "server/web/api/route.go:1"
  "web/src/components/App/Sidebar.jsx:2"
  "web/src/components/App/PWAFooter/index.jsx:2"
  "web/src/components/App/PWAFooter/style.js:1"
  "web/src/components/TorrentList/index.jsx:2"
  "web/src/components/TorrentCard/index.jsx:2"
  "web/src/components/DialogTorrentDetailsContent/index.jsx:2"
)

check_hooks() {
  local bad="" spec file want got
  for spec in "${HOOKS[@]}"; do
    file=${spec%%:*}
    want=${spec##*:}
    got=$(grep -c "homelab" "$file" 2>/dev/null || true)
    [ "${got:-0}" -eq "$want" ] || bad+="$file: крючков $got, ожидалось $want"$'\n'
  done
  [ -z "$bad" ] || { printf '%s' "$bad"; return 1; }
}

run_checks() {
  local log
  log=$(check_hooks) || { echo "$log"; return 1; }
  log=$(cd server && go vet ./torr/storage/torrstor/ ./web/api/ 2>&1 && go test -count=1 ./torr/storage/torrstor/ ./settings/ 2>&1 && go build ./... 2>&1) \
    || { echo "$log" | tail -40; return 1; }
  if [ "$WEB" = 1 ]; then
    # react-scripts 4 на node ≥ 17 падает на md4 в webpack без legacy provider
    if [ "$(node -p 'process.versions.node.split(".")[0]')" -ge 17 ]; then
      export NODE_OPTIONS="${NODE_OPTIONS:-} --openssl-legacy-provider"
    fi
    log=$(cd web && yarn install --frozen-lockfile --silent 2>&1 && npx eslint --ext .js,.jsx src/components/Homelab 2>&1 && CI=false yarn build 2>&1) \
      || { echo "$log" | tail -40; return 1; }
  fi
}

if [ "$CHECK_ONLY" = 1 ]; then
  details=$(run_checks) || finish failed "$details"
  finish checked "крючки на месте, тесты прошли"
fi

git remote get-url upstream >/dev/null 2>&1 || git remote add upstream "$UPSTREAM_URL"
git fetch --quiet --tags upstream master || finish failed "не удалось получить upstream"
[ -n "$TAG" ] || TAG=$(git tag --list 'MatriX.*' --sort=-v:refname | head -1)
[ -n "$TAG" ] || finish failed "в upstream нет тегов MatriX.*"

if git merge-base --is-ancestor "$TAG" HEAD; then
  finish uptodate "$TAG уже влит"
fi

BASE=$(git rev-parse HEAD)
if ! git merge --no-ff --no-edit -m "Слияние upstream $TAG" "$TAG" >/dev/null 2>&1; then
  conflicts=$(git diff --name-only --diff-filter=U)
  git merge --abort
  finish conflict "конфликты с $TAG в файлах:"$'\n'"$conflicts"
fi

echo "$TAG" > server/settings/homelab_upstream.txt
git add server/settings/homelab_upstream.txt
git commit --quiet --amend --no-edit

if ! details=$(run_checks); then
  git reset --quiet --hard "$BASE"
  finish failed "после слияния $TAG не прошли проверки:"$'\n'"$details"
fi
finish merged "$TAG влит, крючки на месте, тесты прошли"
