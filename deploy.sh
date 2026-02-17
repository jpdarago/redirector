#!/usr/bin/env bash
set -euo pipefail

HOST="jpdarago.com"
BINARY="redirector"
REMOTE_PATH="/usr/local/bin/${BINARY}"
SERVICE="redirector.service"
USR=jpdarago

echo "Building ${BINARY}..."
devenv shell -- env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "${BINARY}" .

echo "Copying to ${HOST}..."
scp "${BINARY}" "${USR}@${HOST}:~/${BINARY}"

echo "Installing and restarting service..."
ssh -t "${USR}@${HOST}" "sudo systemctl stop ${SERVICE} && sudo cp ~/${BINARY} ${REMOTE_PATH} && sudo systemctl start ${SERVICE}"

echo "Done."
