#!/usr/bin/env bash

set -euo pipefail

VERSION="${1:-}"

if [[ -z "${VERSION}" ]]; then
    echo "usage: $0 <version>" >&2
    exit 1
fi

BINARY_NAME="citebox-desktop"
APP_NAME="CiteBox"
BUNDLE_ID="com.xuzhougeng.citebox"
PACKAGE_DIR="dist/${BINARY_NAME}-macos-${VERSION}"
APP_DIR="${PACKAGE_DIR}/${APP_NAME}.app"
CONTENTS_DIR="${APP_DIR}/Contents"
MACOS_DIR="${CONTENTS_DIR}/MacOS"
RESOURCES_DIR="${CONTENTS_DIR}/Resources"
DMG_ROOT="${PACKAGE_DIR}/dmg-root"
DMG_PATH="dist/${BINARY_NAME}-macos-${VERSION}.dmg"
ICON_TMP="$(mktemp -d)"
BUILD_TIME="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
BUILDINFO_PKG="github.com/xuzhougeng/citebox/internal/buildinfo"
PLIST_VERSION="${VERSION#v}"
MACOS_DEPLOYMENT_TARGET="${MACOSX_DEPLOYMENT_TARGET:-14.0}"
HOST_ARCH="$(go env GOARCH)"
FITZ_LIB="third_party/go-fitz/libs/libmupdf_darwin_${HOST_ARCH}.a"
BUILD_TAGS=()

if [[ "${CITEBOX_FORCE_NOCGO:-}" == "1" ]]; then
    printf '%s\n' "CITEBOX_FORCE_NOCGO=1, building macOS desktop package with -tags nocgo"
    BUILD_TAGS=(-tags nocgo)
elif [[ ! -f "${FITZ_LIB}" ]]; then
    printf '%s\n' "MuPDF static library not found at ${FITZ_LIB}, building macOS desktop package with -tags nocgo"
    BUILD_TAGS=(-tags nocgo)
fi

cleanup() {
    rm -rf "${ICON_TMP}"
}
trap cleanup EXIT

trim_value() {
    local value="${1:-}"
    local first_char=""
    local last_char=""

    value="${value#"${value%%[![:space:]]*}"}"
    value="${value%"${value##*[![:space:]]}"}"

    if [[ ${#value} -ge 2 ]]; then
        first_char="${value:0:1}"
        last_char="${value:${#value}-1:1}"
        if [[ "${first_char}" == "${last_char}" && ( "${first_char}" == "\"" || "${first_char}" == "'" ) ]]; then
            value="${value:1:${#value}-2}"
        fi
    fi

    printf '%s' "${value}"
}

rm -rf "${PACKAGE_DIR}"
rm -f "${DMG_PATH}"
mkdir -p "${MACOS_DIR}" "${RESOURCES_DIR}"

MACOSX_DEPLOYMENT_TARGET="${MACOS_DEPLOYMENT_TARGET}" \
CGO_ENABLED=1 GOOS=darwin go build \
    -trimpath \
    "${BUILD_TAGS[@]}" \
    -ldflags "-s -w -X ${BUILDINFO_PKG}.Version=${VERSION} -X ${BUILDINFO_PKG}.BuildTime=${BUILD_TIME}" \
    -o "${MACOS_DIR}/${APP_NAME}" \
    ./cmd/desktop

cp -R web "${RESOURCES_DIR}/"
cp README.md "${PACKAGE_DIR}/"
go run ./scripts/fetch_pdfjs.go "${RESOURCES_DIR}/web/static/vendor/pdfjs"

SOURCE_ICON="${ICON_TMP}/AppIcon-1024.png"
ICONSET_DIR="${ICON_TMP}/AppIcon.iconset"

go run ./scripts/render_app_icon -png "${SOURCE_ICON}" -size 1024
mkdir -p "${ICONSET_DIR}"

for size in 16 32 128 256 512; do
    sips -z "${size}" "${size}" "${SOURCE_ICON}" --out "${ICONSET_DIR}/icon_${size}x${size}.png" >/dev/null
    retina_size=$((size * 2))
    sips -z "${retina_size}" "${retina_size}" "${SOURCE_ICON}" --out "${ICONSET_DIR}/icon_${size}x${size}@2x.png" >/dev/null
done

iconutil -c icns "${ICONSET_DIR}" -o "${RESOURCES_DIR}/AppIcon.icns"

cat > "${CONTENTS_DIR}/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleDevelopmentRegion</key>
    <string>en</string>
    <key>CFBundleDisplayName</key>
    <string>${APP_NAME}</string>
    <key>CFBundleExecutable</key>
    <string>${APP_NAME}</string>
    <key>CFBundleIconFile</key>
    <string>AppIcon</string>
    <key>CFBundleIdentifier</key>
    <string>${BUNDLE_ID}</string>
    <key>CFBundleInfoDictionaryVersion</key>
    <string>6.0</string>
    <key>CFBundleName</key>
    <string>${APP_NAME}</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleShortVersionString</key>
    <string>${PLIST_VERSION}</string>
    <key>CFBundleVersion</key>
    <string>${PLIST_VERSION}</string>
    <key>LSMinimumSystemVersion</key>
    <string>${MACOS_DEPLOYMENT_TARGET}</string>
    <key>LSApplicationCategoryType</key>
    <string>public.app-category.productivity</string>
    <key>NSHighResolutionCapable</key>
    <true/>
</dict>
</plist>
EOF

if [[ -n "${CODESIGN_IDENTITY:-}" ]]; then
    CODESIGN_IDENTITY="$(trim_value "${CODESIGN_IDENTITY}")"
    CODESIGN_KEYCHAIN="$(trim_value "${CODESIGN_KEYCHAIN:-}")"

    if [[ -z "${CODESIGN_IDENTITY}" ]]; then
        echo "CODESIGN_IDENTITY is empty after trimming" >&2
        exit 1
    fi

    codesign_args=(--force --deep --options runtime --sign "${CODESIGN_IDENTITY}")
    if [[ -n "${CODESIGN_KEYCHAIN}" ]]; then
        codesign_args+=(--keychain "${CODESIGN_KEYCHAIN}")
    fi

    codesign "${codesign_args[@]}" "${APP_DIR}"
fi

mkdir -p "${DMG_ROOT}"
cp -R "${APP_DIR}" "${DMG_ROOT}/"
ln -s /Applications "${DMG_ROOT}/Applications"

cat > "${DMG_ROOT}/README.txt" <<EOF
CiteBox Desktop (macOS)
=======================

Quick start:
1. Open the DMG.
2. Drag CiteBox into Applications.
3. Launch CiteBox from Applications.

Default account: citebox / citebox123
Desktop mode stores data in:
- ~/Library/Application Support/CiteBox/

Override paths with:
- STORAGE_DIR
- DATABASE_PATH
- UPLOAD_DIR
EOF

hdiutil create \
    -volname "${APP_NAME}" \
    -srcfolder "${DMG_ROOT}" \
    -ov \
    -format UDZO \
    "${DMG_PATH}" >/dev/null

printf '%s\n' "Created ${DMG_PATH}"
printf '%s\n' "macOS deployment target: ${MACOS_DEPLOYMENT_TARGET}"
