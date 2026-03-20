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

cleanup() {
    rm -rf "${ICON_TMP}"
}
trap cleanup EXIT

rm -rf "${PACKAGE_DIR}"
rm -f "${DMG_PATH}"
mkdir -p "${MACOS_DIR}" "${RESOURCES_DIR}"

CGO_ENABLED=1 GOOS=darwin go build \
    -trimpath \
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
    <key>LSApplicationCategoryType</key>
    <string>public.app-category.productivity</string>
    <key>NSHighResolutionCapable</key>
    <true/>
</dict>
</plist>
EOF

if [[ -n "${CODESIGN_IDENTITY:-}" ]]; then
    codesign --force --deep --options runtime --sign "${CODESIGN_IDENTITY}" "${APP_DIR}"
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
