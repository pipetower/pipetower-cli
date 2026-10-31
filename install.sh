#!/bin/sh
# Installs the latest pt release: curl -fsSL https://raw.githubusercontent.com/pipetower/pipetower-cli/main/install.sh | sh
# Set PT_INSTALL_DIR to choose the directory (default: /usr/local/bin, or ~/.local/bin without write access).
set -eu

REPO="pipetower/pipetower-cli"

fail() {
    echo "pt install: $1" >&2
    exit 1
}

detect_os() {
    case "$(uname -s)" in
        Linux) echo linux ;;
        Darwin) echo darwin ;;
        *) fail "unsupported operating system $(uname -s), download pt from https://github.com/$REPO/releases" ;;
    esac
}

detect_arch() {
    case "$(uname -m)" in
        x86_64 | amd64) echo amd64 ;;
        arm64 | aarch64) echo arm64 ;;
        *) fail "unsupported architecture $(uname -m)" ;;
    esac
}

latest_version() {
    curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" |
        sed -n 's/.*"tag_name": *"v\{0,1\}\([^"]*\)".*/\1/p' | head -n 1
}

install_dir() {
    if [ -n "${PT_INSTALL_DIR:-}" ]; then
        echo "$PT_INSTALL_DIR"
    elif [ -w /usr/local/bin ]; then
        echo /usr/local/bin
    else
        echo "$HOME/.local/bin"
    fi
}

main() {
    command -v curl >/dev/null || fail "curl is required"
    os=$(detect_os)
    arch=$(detect_arch)
    version=$(latest_version)
    [ -n "$version" ] || fail "could not find the latest release"

    archive="pt_${version}_${os}_${arch}.tar.gz"
    tmp=$(mktemp -d)
    trap 'rm -rf "$tmp"' EXIT

    echo "Downloading pt $version for $os/$arch…"
    curl -fsSL "https://github.com/$REPO/releases/download/v$version/$archive" -o "$tmp/$archive"
    curl -fsSL "https://github.com/$REPO/releases/download/v$version/checksums.txt" -o "$tmp/checksums.txt"
    (cd "$tmp" && grep " $archive\$" checksums.txt | sha256sum -c - >/dev/null 2>&1) ||
        (cd "$tmp" && grep " $archive\$" checksums.txt | shasum -a 256 -c - >/dev/null) ||
        fail "checksum verification failed"

    tar -xzf "$tmp/$archive" -C "$tmp" pt
    dir=$(install_dir)
    mkdir -p "$dir"
    install -m 0755 "$tmp/pt" "$dir/pt"

    echo "Installed pt $version to $dir/pt"
    case ":$PATH:" in
        *":$dir:"*) ;;
        *) echo "Add $dir to your PATH to use pt." ;;
    esac
    echo "Next: put PIPETOWER_URL and PIPETOWER_TOKEN into ~/.pipetower and run: pt status"
}

main
