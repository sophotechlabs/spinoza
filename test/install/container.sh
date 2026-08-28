#!/usr/bin/env sh
set -eu

if [ -n "${SETUP:-}" ]; then
    sh -c "$SETUP"
fi

sh /install.sh
spinoza --version

installed="$(command -v spinoza)"

SPINOZA_UNINSTALL=1 sh /install.sh

if [ -e "$installed" ]; then
    echo "container: $installed is still there after the uninstall"
    exit 1
fi

again="$(SPINOZA_UNINSTALL=1 sh /install.sh)"
case "$again" in
    *"not installed"*)
        ;;
    *)
        echo "container: a second uninstall did not say there was nothing to remove"
        printf '%s\n' "$again"
        exit 1
        ;;
esac

echo "container: installed, ran, uninstalled and reported an empty second pass"
