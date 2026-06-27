#!/usr/bin/env bash
# DEPRECATED — dùng: bash bootstrap/addons/install-harbor.sh
exec bash "$(cd "$(dirname "$0")/.." && pwd)/addons/install-harbor.sh" "$@"
