#!/usr/bin/env bash
# DEPRECATED — dùng: bash bootstrap/addons/install-rancher.sh
exec bash "$(cd "$(dirname "$0")/.." && pwd)/addons/install-rancher.sh" "$@"
