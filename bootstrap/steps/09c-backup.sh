#!/usr/bin/env bash
exec bash "$(cd "$(dirname "$0")/.." && pwd)/addons/rancher/backup.sh" "$@"
