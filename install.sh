#!/bin/sh
set -e

# AllYourBase installer
# Usage: curl -fsSL https://allyourbase.io/install.sh | sh

REPO="stuartcrobinson/allyourbase"
INSTALL_DIR="/usr/local/bin"
BINARY="ayb"

main() {
    os=$(uname -s | tr '[:upper:]' '[:lower:]')
    arch=$(uname -m)

    case "$arch" in
        x86_64|amd64) arch="amd64" ;;
        aarch64|arm64) arch="arm64" ;;
        *) echo "Unsupported architecture: $arch" >&2; exit 1 ;;
    esac

    case "$os" in
        linux) os="linux" ;;
        darwin) os="darwin" ;;
        *) echo "Unsupported OS: $os" >&2; exit 1 ;;
    esac

    # Get latest version
    version=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"v?([^"]+)".*/\1/')
    if [ -z "$version" ]; then
        echo "Error: could not determine latest version" >&2
        exit 1
    fi

    echo "Installing ayb v${version} (${os}/${arch})..."

    # Download archive
    archive="ayb_${version}_${os}_${arch}.tar.gz"
    url="https://github.com/${REPO}/releases/download/v${version}/${archive}"

    tmpdir=$(mktemp -d)
    trap 'rm -rf "$tmpdir"' EXIT

    echo "Downloading ${url}..."
    curl -fsSL "$url" -o "${tmpdir}/${archive}"

    # Download checksums and verify
    checksums_url="https://github.com/${REPO}/releases/download/v${version}/checksums.txt"
    if curl -fsSL "$checksums_url" -o "${tmpdir}/checksums.txt" 2>/dev/null; then
        expected=$(grep "$archive" "${tmpdir}/checksums.txt" | awk '{print $1}')
        if [ -n "$expected" ]; then
            if command -v sha256sum >/dev/null 2>&1; then
                actual=$(sha256sum "${tmpdir}/${archive}" | awk '{print $1}')
            elif command -v shasum >/dev/null 2>&1; then
                actual=$(shasum -a 256 "${tmpdir}/${archive}" | awk '{print $1}')
            fi
            if [ -n "$actual" ] && [ "$actual" != "$expected" ]; then
                echo "Checksum mismatch!" >&2
                echo "  expected: $expected" >&2
                echo "  got:      $actual" >&2
                exit 1
            fi
            echo "Checksum verified."
        fi
    fi

    # Extract
    tar -xzf "${tmpdir}/${archive}" -C "$tmpdir"

    # Install
    if [ -w "$INSTALL_DIR" ]; then
        mv "${tmpdir}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
    else
        echo "Installing to ${INSTALL_DIR} (requires sudo)..."
        sudo mv "${tmpdir}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
    fi
    chmod +x "${INSTALL_DIR}/${BINARY}"

    echo ""
    echo "ayb v${version} installed to ${INSTALL_DIR}/${BINARY}"
    echo ""
    echo "Get started:"
    echo "  ayb start                    # embedded Postgres, zero config"
    echo "  ayb start --database-url URL # external Postgres"
    echo ""
}

main
