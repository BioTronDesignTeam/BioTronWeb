#!/usr/bin/env bash
# One install for every frontend and the shared library, then warm the Go
# module cache for every backend through the workspace.
set -euo pipefail
npm ci
go work sync
