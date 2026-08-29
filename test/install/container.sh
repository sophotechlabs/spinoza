#!/usr/bin/env sh
set -eu

if [ -n "${SETUP:-}" ]; then
    sh -c "$SETUP"
fi

sh /install.sh
spinoza --version

installed="$(command -v spinoza)"
copyright=/usr/local/share/doc/spinoza/copyright

if [ ! -f "$copyright" ]; then
    echo "container: the install left no copyright at $copyright"
    exit 1
fi

if ! grep -q 'WITHOUT WARRANTIES OF ANY KIND' "$copyright"; then
    echo "container: $copyright is not the license text"
    exit 1
fi

SPINOZA_UNINSTALL=1 sh /install.sh

if [ -e "$installed" ]; then
    echo "container: $installed is still there after the uninstall"
    exit 1
fi

if [ -e "$copyright" ]; then
    echo "container: $copyright is still there after the uninstall"
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

echo "container: installed the binary and the license, ran, uninstalled both and reported an empty second pass"
