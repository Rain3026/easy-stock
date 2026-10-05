#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/local-env.sh"
cd "${EASY_STOCK_ROOT}"
mkdir -p .runtime/web-data

owned_pid() {
  local role="$1" pid command
  [[ -f ".runtime/${role}.pid" ]] || return 1
  pid="$(cat ".runtime/${role}.pid")"
  [[ "$pid" =~ ^[0-9]+$ ]] || return 1
  command="$(ps -p "$pid" -o command= 2>/dev/null || true)"
  case "$role:$command" in
    backend:*"${EASY_STOCK_ROOT}/desktop/resources/backend/easy-stock-backend"*) printf '%s' "$pid" ;;
    frontend:*"${EASY_STOCK_ROOT}/node_modules/vite/bin/vite.js"*) printf '%s' "$pid" ;;
    *) return 1 ;;
  esac
}

if [[ "${1:-start}" == stop ]]; then
  for role in frontend backend; do
    pid="$(owned_pid "$role" || true)"
    if [[ -n "$pid" ]]; then kill "$pid"; fi
    rm -f ".runtime/${role}.pid"
  done
  printf 'EasyStock Web 已停止。\n'
  exit 0
fi

for service in backend:20081 frontend:20073; do
  role="${service%:*}"; port="${service#*:}"
  listener="$(lsof -tiTCP:"$port" -sTCP:LISTEN 2>/dev/null || true)"
  owned="$(owned_pid "$role" || true)"
  if [[ -n "$listener" && "$listener" != "$owned" ]]; then
    printf '端口 %s 被其他进程占用；请先释放该端口。\n' "$port" >&2
    exit 1
  fi
done

if ! owned_pid backend >/dev/null; then
  screen -dmS easy-stock-local-backend bash -c 'echo "$$" > "$1/.runtime/backend.pid"; exec "$1/desktop/resources/backend/easy-stock-backend" > "$1/.runtime/backend.log" 2>&1' bash "${EASY_STOCK_ROOT}"
fi
if ! owned_pid frontend >/dev/null; then
  screen -dmS easy-stock-local-frontend bash -c 'cd "$1/frontend"; echo "$$" > "$1/.runtime/frontend.pid"; exec node "$1/node_modules/vite/bin/vite.js" preview --host 127.0.0.1 --port 20073 --strictPort > "$1/.runtime/frontend.log" 2>&1' bash "${EASY_STOCK_ROOT}"
fi

for endpoint in http://127.0.0.1:20081/api/health http://127.0.0.1:20073; do
  ready=0
  for attempt in {1..60}; do
    if curl --noproxy '*' -fsS "$endpoint" >/dev/null 2>&1; then ready=1; break; fi
    sleep 0.5
  done
  if [[ "$ready" != 1 ]]; then
    printf '服务未就绪：%s；请查看 .runtime/*.log\n' "$endpoint" >&2
    exit 1
  fi
done
printf 'EasyStock Web：http://127.0.0.1:20073\n'
