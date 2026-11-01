#!/usr/bin/env bash
set -euo pipefail
cd /srv/openim38-upgrade
[[ $(pwd -P) == /srv/openim38-upgrade && -f instance.json && -f compose.yaml ]] || exit 2
stack_tools_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
python3 "$stack_tools_dir/stack_guard.py"
stack_docker=(docker --host unix:///var/run/docker.sock compose --project-directory /srv/openim38-upgrade -f /srv/openim38-upgrade/compose.yaml -p openim38-upgrade)
case "${1:-status}" in
 start) "${stack_docker[@]}" up -d --no-build ;;
 stop) "${stack_docker[@]}" stop ;;
 status) "${stack_docker[@]}" ps ;;
 logs) stack_log_service="${2:-enterprise-service}"; [[ "$stack_log_service" =~ ^[a-z][a-z0-9-]*$ ]] || exit 2; "${stack_docker[@]}" logs --tail=100 "$stack_log_service" ;;
 nginx-check) "${stack_docker[@]}" exec -T nginx nginx -t ;;
 nginx-reload) "${stack_docker[@]}" exec -T nginx nginx -t;"${stack_docker[@]}" exec -T nginx nginx -s reload ;;
 *) echo 'Usage: manage.sh start|stop|status|logs [service]|nginx-check|nginx-reload' >&2;exit 2 ;;
esac
