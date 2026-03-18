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
        ;;
    linux)
        GOOS_VALUE="linux"
        ;;
    *)
        echo "unsupported platform: ${PLATFORM}" >&2
        exit 1
        ;;
esac

BINARY_NAME="citebox"
PACKAGE_DIR="dist/${BINARY_NAME}-${PLATFORM}-${VERSION}"
ARCHIVE_PATH="${PACKAGE_DIR}.tar.gz"

rm -rf "${PACKAGE_DIR}"
mkdir -p "${PACKAGE_DIR}/data/library/papers"
mkdir -p "${PACKAGE_DIR}/data/library/figures"

GOOS="${GOOS_VALUE}" GOARCH=amd64 go build \
    -trimpath \
    -ldflags "-s -w" \
    -o "${PACKAGE_DIR}/${BINARY_NAME}" \
    ./cmd/server

cp -R web "${PACKAGE_DIR}/"
cp README.md "${PACKAGE_DIR}/"
go run ./scripts/fetch_pdfjs.go "${PACKAGE_DIR}/web/static/vendor/pdfjs"

cat > "${PACKAGE_DIR}/start.sh" <<'EOF'
#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
cd "${SCRIPT_DIR}"

printf '%s\n' "========================================"
printf '%s\n' "  CiteBox"
printf '%s\n' "========================================"
printf '\n'
printf '%s\n' "Starting server..."
printf '%s\n' "Default URL: http://localhost:8080"
printf '%s\n' "Default account: citebox / citebox123"
printf '%s\n' "Press Ctrl+C to stop"
printf '\n'

./citebox
EOF

cat > "${PACKAGE_DIR}/start-with-config.sh" <<'EOF'
#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
cd "${SCRIPT_DIR}"

export SERVER_PORT=8080
export ADMIN_USERNAME=citebox
export ADMIN_PASSWORD=citebox123
# export PDF_EXTRACTOR_URL=http://127.0.0.1:8000
# export STORAGE_DIR=./data/library
# export DATABASE_PATH=./data/library.db

printf '%s\n' "Starting server with custom configuration..."
printf '%s\n' "Current port: ${SERVER_PORT}"
printf '\n'

./citebox
EOF

cat > "${PACKAGE_DIR}/README.txt" <<EOF
CiteBox ${PLATFORM} package
==========================

Contents:
- citebox
- web/
- data/
- start.sh
- start-with-config.sh

Quick start:
1. tar -xzf $(basename "${ARCHIVE_PATH}")
2. cd $(basename "${PACKAGE_DIR}")
3. chmod +x citebox start.sh start-with-config.sh
4. ./start.sh

Default URL: http://localhost:8080
Default account: citebox / citebox123
EOF

chmod +x "${PACKAGE_DIR}/${BINARY_NAME}" "${PACKAGE_DIR}/start.sh" "${PACKAGE_DIR}/start-with-config.sh"

rm -f "${ARCHIVE_PATH}"
tar -C dist -czf "${ARCHIVE_PATH}" "$(basename "${PACKAGE_DIR}")"

printf '%s\n' "Created ${ARCHIVE_PATH}"
