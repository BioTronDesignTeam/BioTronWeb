#!/usr/bin/env bash

set -euo pipefail

if [[ ! -f package.json ]]; then
  exit 0
fi

if [[ -f package-lock.json ]]; then
  npm ci
else
  npm install
fi
