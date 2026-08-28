#!/usr/bin/env sh
set -eu

installer="${1:-install.sh}"
installer="$(cd "$(dirname "$installer")" && pwd)/$(basename "$installer")"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT INT TERM

fail() {
    echo "test-uninstall: $1" >&2
    exit 1
}

seed() {
    mkdir -p "$work/bin"
    printf 'binary\n' > "$work/bin/spinoza"
    printf 'app\n' > "$work/bin/Spinoza"
    mkdir -p "$work/.local/share/applications"
    printf 'entry\n' > "$work/.local/share/applications/spinoza.desktop"
    mkdir -p "$work/.local/share/icons/hicolor/512x512/apps"
    printf 'icon\n' > "$work/.local/share/icons/hicolor/512x512/apps/spinoza.png"
    mkdir -p "$work/Applications/Spinoza.app/Contents"
    printf 'bundle\n' > "$work/Applications/Spinoza.app/Contents/Info.plist"
    printf 'mine\n' > "$work/bin/notes.txt"
}

run_uninstall() {
    HOME="$work" SPINOZA_INSTALL_DIR="$work/bin" SPINOZA_UNINSTALL=1 sh "$installer"
}

seed
said="$(run_uninstall)"

for gone in \
    "$work/bin/spinoza" \
    "$work/bin/Spinoza" \
    "$work/.local/share/applications/spinoza.desktop" \
    "$work/.local/share/icons/hicolor/512x512/apps/spinoza.png" \
    "$work/Applications/Spinoza.app"
do
    if [ -e "$gone" ]; then
        fail "$gone survived the uninstall"
    fi
done

if [ ! -f "$work/bin/notes.txt" ]; then
    fail "the uninstall took a file that was not spinoza's"
fi

for named in spinoza Spinoza "the desktop entry" "the icon" "Spinoza.app"; do
    case "$said" in
        *"$named"*)
            ;;
        *)
            fail "the uninstall did not name $named in what it removed"
            ;;
    esac
done

case "$said" in
    *"Settings and kubeconfigs were left alone"*)
        ;;
    *)
        fail "the uninstall did not say what it left alone"
        ;;
esac

again="$(run_uninstall)"
case "$again" in
    *"not installed"*)
        ;;
    *)
        fail "a second uninstall did not report an empty pass"
        ;;
esac

echo "test-uninstall: removed the binary, the app, the desktop entry and the icon, and left everything else"
