#!/usr/bin/env bash

set -euo pipefail

if [[ ! -f frontend/package.json ]]; then
  exit 0
fi

if [[ -f frontend/package-lock.json ]]; then
  npm --prefix frontend ci
else
  npm --prefix frontend install
fi
