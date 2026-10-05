#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")" && pwd)"
APP="${ROOT}/desktop/dist/builder-dir/mac-arm64/easy-stock.app"
if [[ ! -d "$APP" ]]; then
  printf '桌面程序不存在：%s\n' "$APP" >&2
  exit 1
fi
open "$APP"
