#!/bin/sh
set -e

if [ "$ADMIN_FACING" = "true" ]; then
  exec env ADMIN_PORT="$PORT" PORT=8000 ./stellar-disbursement-platform serve
else
  exec ./stellar-disbursement-platform serve
fi
