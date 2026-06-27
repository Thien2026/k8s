#!/usr/bin/env bash
exec bash "$(cd "$(dirname "$0")/.." && pwd)/core/steps/$(basename "$0")" "$@"
