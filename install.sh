#!/usr/bin/env sh
set -eu

repo="sophotechlabs/spinoza"
releases="https://github.com/$repo/releases"

main() {
    detect_downloader
    detect_platform

    version="${SPINOZA_VERSION:-}"
    if [ -z "$version" ]; then
        version="$(resolve_latest)"
    fi

    asset="spinoza_${version}_${os}_${arch}.tar.gz"
    dir="$(install_dir)"

    make_temp
    trap cleanup EXIT INT TERM

    echo "Downloading spinoza $version ($os/$arch)"
    download "$releases/download/$version/$asset" "$temp/$asset"
    download "$releases/download/$version/checksums.txt" "$temp/checksums.txt"
    verify "$temp/$asset" "$asset"
    attest "$temp/$asset" "$asset"

    tar -xzf "$temp/$asset" -C "$temp"
    if [ ! -f "$temp/spinoza" ]; then
        die "the archive did not contain a spinoza binary"
    fi

    previous=""
    if [ -x "$dir/spinoza" ]; then
        previous="$("$dir/spinoza" --version 2>/dev/null || true)"
    fi

    mkdir -p "$dir"
    install -m 0755 "$temp/spinoza" "$dir/spinoza"

    if [ -n "$previous" ]; then
        echo "Updated spinoza $previous -> $version in $dir"
    else
        echo "Installed spinoza $version in $dir"
    fi

    app_dir=""
    if [ -z "${SPINOZA_SKIP_APP:-}" ]; then
        if [ "$os" = "darwin" ]; then
            install_app
        fi
        if [ "$os" = "linux" ]; then
            install_linux_app
        fi
    fi

    report_path "$dir"
    report_app
}

install_app() {
    app_asset="spinoza_${version}_darwin_app.zip"
    if ! listed "$app_asset"; then
        return 0
    fi
    if ! command -v unzip >/dev/null 2>&1; then
        echo "Skipped the desktop app: unzip is not on PATH"
        return 0
    fi
    download "$releases/download/$version/$app_asset" "$temp/$app_asset"
    verify "$temp/$app_asset" "$app_asset"
    attest "$temp/$app_asset" "$app_asset"
    unzip -q -o "$temp/$app_asset" -d "$temp/app"
    if [ ! -d "$temp/app/Spinoza.app" ]; then
        die "the app archive did not contain Spinoza.app"
    fi
    for candidate in /Applications "$HOME/Applications"; do
        if try_app_dir "$candidate"; then
            app_dir="$candidate"
            echo "Installed the Spinoza app in $candidate"
            return 0
        fi
    done
    echo "Could not write the app to /Applications or ~/Applications"
}

install_linux_app() {
    app_asset="spinoza_${version}_linux_${arch}_app.tar.gz"
    if ! listed "$app_asset"; then
        return 0
    fi
    if ! glibc; then
        echo "Skipped the desktop app: it is built against glibc and this system is not"
        return 0
    fi
    download "$releases/download/$version/$app_asset" "$temp/$app_asset"
    verify "$temp/$app_asset" "$app_asset"
    attest "$temp/$app_asset" "$app_asset"
    mkdir -p "$temp/app"
    tar -xzf "$temp/$app_asset" -C "$temp/app"
    if [ ! -f "$temp/app/Spinoza" ]; then
        die "the app archive did not contain a Spinoza binary"
    fi
    install -m 0755 "$temp/app/Spinoza" "$dir/Spinoza"
    icons="$HOME/.local/share/icons/hicolor/512x512/apps"
    mkdir -p "$icons"
    install -m 0644 "$temp/app/spinoza.png" "$icons/spinoza.png"
    entries="$HOME/.local/share/applications"
    mkdir -p "$entries"
    write_desktop_entry "$entries/spinoza.desktop"
    app_dir="$entries"
    echo "Installed the Spinoza app in $dir, with a desktop entry in $entries"
}

glibc() {
    if ! command -v ldd >/dev/null 2>&1; then
        return 1
    fi
    if ldd --version 2>&1 | grep -qi musl; then
        return 1
    fi
    return 0
}

write_desktop_entry() {
    cat > "$1" <<DESKTOP
[Desktop Entry]
Type=Application
Name=Spinoza
Comment=Self-hosted Kubernetes GUI
Exec=$dir/Spinoza
Icon=spinoza
Terminal=false
Categories=Development;
StartupWMClass=Spinoza
DESKTOP
}

try_app_dir() {
    target="$1"
    if ! mkdir -p "$target" 2>/dev/null; then
        return 1
    fi
    if ! rm -rf "$target/Spinoza.app" 2>/dev/null; then
        return 1
    fi
    if ! cp -R "$temp/app/Spinoza.app" "$target/Spinoza.app" 2>/dev/null; then
        return 1
    fi
    return 0
}

listed() {
    found="$(awk -v name="$1" '$2 == name { print $1 }' "$temp/checksums.txt")"
    if [ -z "$found" ]; then
        return 1
    fi
    return 0
}

report_app() {
    if [ -z "$app_dir" ]; then
        return 0
    fi
    if [ "$os" = "darwin" ]; then
        echo "Open the desktop app with 'open -a Spinoza', or find it in Spotlight"
        return 0
    fi
    echo "Open the desktop app from your launcher, or run '$dir/Spinoza'"
}

detect_downloader() {
    if command -v curl >/dev/null 2>&1; then
        downloader=curl
        return 0
    fi
    if command -v wget >/dev/null 2>&1; then
        downloader=wget
        return 0
    fi
    die "neither curl nor wget is on PATH"
}

detect_platform() {
    case "$(uname -s)" in
        Darwin)
            os=darwin
            ;;
        Linux)
            os=linux
            ;;
        *)
            die "$(uname -s) is not supported; see $releases for the other builds"
            ;;
    esac

    case "$(uname -m)" in
        x86_64 | amd64)
            arch=amd64
            ;;
        arm64 | aarch64)
            arch=arm64
            ;;
        *)
            die "$(uname -m) is not supported; see $releases for the other builds"
            ;;
    esac
}

resolve_latest() {
    if [ "$downloader" = curl ]; then
        final="$(curl -fsSL -o /dev/null -w '%{url_effective}' "$releases/latest")"
    else
        final="$(wget -q -S --spider -O /dev/null "$releases/latest" 2>&1 | awk 'tolower($1) == "location:" {print $2}' | tail -n 1)"
    fi
    case "$final" in
        */releases/tag/*)
            printf '%s\n' "${final##*/}"
            ;;
        *)
            die "$repo has no published release yet; set SPINOZA_VERSION to pick one"
            ;;
    esac
}

download() {
    if [ "$downloader" = curl ]; then
        curl -fsSL "$1" -o "$2"
        return 0
    fi
    wget -q "$1" -O "$2"
}

verify() {
    expected="$(awk -v name="$2" '$2 == name { print $1 }' "$temp/checksums.txt")"
    if [ -z "$expected" ]; then
        die "$2 is not listed in checksums.txt"
    fi
    actual="$(sha256_of "$1")"
    if [ "$expected" != "$actual" ]; then
        die "checksum mismatch for $2"
    fi
}

attest() {
    if [ -z "${SPINOZA_VERIFY_ATTESTATION:-}" ]; then
        return 0
    fi
    if ! command -v gh >/dev/null 2>&1; then
        die "SPINOZA_VERIFY_ATTESTATION is set but gh is not on PATH"
    fi
    if ! gh attestation verify "$1" --repo "$repo" >/dev/null 2>&1; then
        die "$2 carries no build provenance signed for $repo"
    fi
    echo "Verified the build provenance of $2"
}

sha256_of() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$1" | awk '{ print $1 }'
        return 0
    fi
    if command -v shasum >/dev/null 2>&1; then
        shasum -a 256 "$1" | awk '{ print $1 }'
        return 0
    fi
    if command -v openssl >/dev/null 2>&1; then
        openssl dgst -sha256 "$1" | awk '{ print $NF }'
        return 0
    fi
    die "no sha256 tool found, refusing to install unverified"
}

install_dir() {
    if [ -n "${SPINOZA_INSTALL_DIR:-}" ]; then
        printf '%s\n' "$SPINOZA_INSTALL_DIR"
        return 0
    fi
    if [ "$(id -u)" = 0 ]; then
        printf '%s\n' /usr/local/bin
        return 0
    fi
    printf '%s\n' "$HOME/.local/bin"
}

make_temp() {
    if [ -n "${TMPDIR:-}" ] && [ -d "${TMPDIR}" ]; then
        temp="$(mktemp -d "$TMPDIR/spinoza-XXXXXX")"
        return 0
    fi
    temp="$(mktemp -d /tmp/spinoza-XXXXXX)"
}

cleanup() {
    rm -rf "$temp"
}

report_path() {
    if [ "$(command -v spinoza || true)" = "$1/spinoza" ]; then
        echo "Run it in your browser with 'spinoza --open'"
        return 0
    fi
    hint="$1"
    case "$hint" in
        "$HOME"/*)
            hint="\$HOME${hint#"$HOME"}"
            ;;
    esac
    echo "$1 is not on your PATH. Add it with:"
    case "${SHELL:-}" in
        *zsh)
            echo "   echo 'export PATH=$hint:\$PATH' >> ~/.zshrc"
            ;;
        *fish)
            echo "   fish_add_path -U $1"
            ;;
        *)
            echo "   echo 'export PATH=$hint:\$PATH' >> ~/.bashrc"
            ;;
    esac
    echo "Or run it now with '$1/spinoza --open'"
}

die() {
    echo "install: $1" >&2
    exit 1
}

main "$@"
