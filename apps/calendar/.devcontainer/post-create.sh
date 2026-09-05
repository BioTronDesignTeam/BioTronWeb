#!/usr/bin/env bash

set -euo pipefail

install_node_package() {
  local directory="$1"

  if [[ ! -f "${directory}/package.json" ]]; then
    return
  fi

  if [[ -f "${directory}/package-lock.json" ]]; then
    npm --prefix "${directory}" ci
  else
    npm --prefix "${directory}" install
  fi
}

install_node_package prisma
install_node_package frontend

if [[ -f backend/go.mod ]]; then
  (
    cd backend
    go mod download
  )
fi
