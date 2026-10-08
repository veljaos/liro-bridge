#!/bin/sh
# Runs exactly CI's steps for the jobs on the Ubuntu host, read from
# .github/workflows/ci.yml at run time (A33, D-438). Runs on the Ubuntu VM:
# it needs Go and golangci-lint at CI's versions, which it checks before
# running anything. The body and its rules: scripts/check/check.py.
# The controls that show it can fail: scripts/check/controls.sh.
exec python3 "$(dirname "$0")/scripts/check/check.py" "$@"
