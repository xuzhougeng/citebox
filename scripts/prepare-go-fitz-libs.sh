#!/usr/bin/env bash

set -euo pipefail

usage() {
    cat <<'EOF' >&2
usage: scripts/prepare-go-fitz-libs.sh [--goos <linux|darwin|windows>] [--goarch <amd64|arm64>] [--force]
EOF
}

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
FITZ_GO_FILE="${ROOT_DIR}/third_party/go-fitz/fitz.go"
LIBS_DIR="${ROOT_DIR}/third_party/go-fitz/libs"

GOOS_VALUE=""
GOARCH_VALUE=""
FORCE=0

while [[ $# -gt 0 ]]; do
    case "$1" in
        --goos)
            GOOS_VALUE="${2:-}"
            shift 2
            ;;
        --goarch)
            GOARCH_VALUE="${2:-}"
            shift 2
            ;;
        --force)
            FORCE=1
            shift
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            usage
            exit 1
            ;;
    esac
done

require_command() {
    local command_name="$1"

    if ! command -v "${command_name}" >/dev/null 2>&1; then
        echo "required command not found: ${command_name}" >&2
        exit 1
    fi
}

detect_fitz_version() {
    local version=""

    version="$(sed -n 's/^var FzVersion = "\(.*\)"/\1/p' "${FITZ_GO_FILE}" | head -n 1)"
    if [[ -z "${version}" ]]; then
        echo "failed to detect go-fitz version from ${FITZ_GO_FILE}" >&2
        exit 1
    fi

    printf '%s\n' "${version}"
}

download_file() {
    local url="$1"
    local destination="$2"

    curl --fail --location --retry 5 --retry-delay 2 --silent --show-error "${url}" -o "${destination}"

    if [[ ! -s "${destination}" ]]; then
        echo "downloaded file is missing or empty: ${destination}" >&2
        exit 1
    fi
}

if [[ -z "${GOOS_VALUE}" ]]; then
    GOOS_VALUE="$(go env GOOS)"
fi

if [[ -z "${GOARCH_VALUE}" ]]; then
    GOARCH_VALUE="$(go env GOARCH)"
fi

case "${GOOS_VALUE}" in
    linux|darwin)
        case "${GOARCH_VALUE}" in
            amd64|arm64)
                ;;
            *)
                echo "unsupported GOARCH for ${GOOS_VALUE}: ${GOARCH_VALUE}" >&2
                exit 1
                ;;
        esac
        ;;
    windows)
        if [[ "${GOARCH_VALUE}" != "amd64" ]]; then
            echo "unsupported GOARCH for windows: ${GOARCH_VALUE}" >&2
            exit 1
        fi
        ;;
    *)
        echo "unsupported GOOS: ${GOOS_VALUE}" >&2
        exit 1
        ;;
esac

require_command curl

FITZ_VERSION="$(detect_fitz_version)"
FITZ_TAG="v${FITZ_VERSION}"
BASE_URL="https://raw.githubusercontent.com/gen2brain/go-fitz/${FITZ_TAG}/libs"
FILES=(
    "libmupdf_${GOOS_VALUE}_${GOARCH_VALUE}.a"
    "libmupdfthird_${GOOS_VALUE}_${GOARCH_VALUE}.a"
)

mkdir -p "${LIBS_DIR}"

for file_name in "${FILES[@]}"; do
    destination="${LIBS_DIR}/${file_name}"

    if [[ -f "${destination}" && "${FORCE}" -ne 1 ]]; then
        printf '%s\n' "go-fitz lib already present: ${destination}"
        continue
    fi

    printf '%s\n' "Downloading ${file_name} from gen2brain/go-fitz ${FITZ_TAG}"
    download_file "${BASE_URL}/${file_name}" "${destination}"
done

printf '%s\n' "Prepared go-fitz libs for ${GOOS_VALUE}/${GOARCH_VALUE} from ${FITZ_TAG}"
