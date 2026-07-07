#!/usr/bin/env bash
# DEPRECATED — dùng: bash bootstrap/addons/install-monitoring.sh
exec bash "$(cd "$(dirname "$0")/.." && pwd)/addons/install-monitoring.sh" "$@"
