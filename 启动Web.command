#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")" && pwd)"
bash "${ROOT}/scripts/local-web.sh"
open http://127.0.0.1:20073
