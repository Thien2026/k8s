#!/usr/bin/env bash
# DEPRECATED — dùng bootstrap/core/steps/ hoặc ./bootstrap/run.sh
exec bash "$(cd "$(dirname "$0")/.." && pwd)/core/steps/$(basename "$0")" "$@"
