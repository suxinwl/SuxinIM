#!/usr/bin/env bash
set -euo pipefail
umask 077
backup_tools_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
exec python3 "$backup_tools_dir/backup-tool.py" create "$@"
