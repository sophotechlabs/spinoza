#!/usr/bin/env sh
set -eu

if [ -n "${SETUP:-}" ]; then
    sh -c "$SETUP"
fi

sh /install.sh
spinoza --version
