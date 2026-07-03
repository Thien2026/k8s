#!/usr/bin/env bash
# DEPRECATED — dùng: bash bootstrap/addons/install-argocd.sh
exec bash "$(cd "$(dirname "$0")/.." && pwd)/addons/install-argocd.sh" "$@"
