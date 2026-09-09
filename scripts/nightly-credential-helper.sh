#!/usr/bin/env bash
set -euo pipefail

printf 'username=x-access-token\npassword=%s\n' "$GH_TOKEN"
