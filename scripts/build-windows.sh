#!/usr/bin/env bash

set -euo pipefail

VERSION="${1:-${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo "dev")}}"
UNAME_VALUE="$(uname -s)"

case "${UNAME_VALUE}" in
    MINGW*|MSYS*|CYGWIN*)
        ;;
    *)
        printf '%s\n' "Windows packaging must run on native Windows so CGO and MuPDF stay enabled." >&2
        printf '%s\n' "Use scripts/package-windows.ps1 on Windows or the GitHub Actions release workflow." >&2
        exit 1
        ;;
esac

if ! command -v pwsh >/dev/null 2>&1; then
    printf '%s\n' "pwsh is required to run scripts/package-windows.ps1" >&2
    exit 1
fi

exec pwsh -File ./scripts/package-windows.ps1 -Version "${VERSION}"
