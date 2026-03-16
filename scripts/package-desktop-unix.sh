#!/usr/bin/env bash

set -euo pipefail

PLATFORM="${1:-}"
VERSION="${2:-}"

if [[ -z "${PLATFORM}" || -z "${VERSION}" ]]; then
    echo "usage: $0 <macos|linux> <version>" >&2
    exit 1
fi

case "${PLATFORM}" in
    macos)
        GOOS_VALUE="darwin"
        DISPLAY_NAME="macOS"
        DATA_DIR='~/Library/Application Support/CiteBox/'
        ;;
    linux)
        GOOS_VALUE="linux"
        DISPLAY_NAME="Linux"
        DATA_DIR='~/.config/CiteBox/'
        ;;
    *)
        echo "unsupported platform: ${PLATFORM}" >&2
        exit 1
        ;;
esac

BINARY_NAME="citebox-desktop"
PACKAGE_DIR="dist/${BINARY_NAME}-${PLATFORM}-${VERSION}"
ARCHIVE_PATH="${PACKAGE_DIR}.tar.gz"
HOST_ARCH="$(go env GOARCH)"

rm -rf "${PACKAGE_DIR}"
mkdir -p "${PACKAGE_DIR}"

CGO_ENABLED=1 GOOS="${GOOS_VALUE}" go build \
    -trimpath \
    -ldflags "-s -w" \
    -o "${PACKAGE_DIR}/${BINARY_NAME}" \
    ./cmd/desktop

cp -R web "${PACKAGE_DIR}/"
cp README.md "${PACKAGE_DIR}/"
go run ./scripts/fetch_pdfjs.go "${PACKAGE_DIR}/web/static/vendor/pdfjs"

cat > "${PACKAGE_DIR}/start.sh" <<'EOF'
#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
cd "${SCRIPT_DIR}"

exec ./citebox-desktop
EOF

cat > "${PACKAGE_DIR}/README.txt" <<EOF
CiteBox Desktop (${DISPLAY_NAME})
=========================

Contents:
- citebox-desktop
- web/
- start.sh

Quick start:
1. tar -xzf $(basename "${ARCHIVE_PATH}")
2. cd $(basename "${PACKAGE_DIR}")
3. chmod +x citebox-desktop start.sh
4. ./start.sh

Default account: wanglab / wanglab789
Binary architecture: ${HOST_ARCH}

Desktop mode stores data in:
- ${DATA_DIR}

Override paths with:
- STORAGE_DIR
- DATABASE_PATH
- UPLOAD_DIR
EOF

chmod +x "${PACKAGE_DIR}/${BINARY_NAME}" "${PACKAGE_DIR}/start.sh"

rm -f "${ARCHIVE_PATH}"
tar -C dist -czf "${ARCHIVE_PATH}" "$(basename "${PACKAGE_DIR}")"

printf '%s\n' "Created ${ARCHIVE_PATH}"
