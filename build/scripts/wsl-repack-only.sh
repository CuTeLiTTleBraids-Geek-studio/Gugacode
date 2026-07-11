#!/usr/bin/env bash
# Re-run packaging from existing WSL worktree binary with correct version.
set -euo pipefail
export PATH="/usr/local/go/bin:/usr/local/lib/nodejs/bin:${HOME}/go/bin:${PATH}"

SRC=/mnt/e/gugacode/Gugacode-main
WORK="${HOME}/gugacode-pkg-build"
OUT_WIN="$SRC/bin"
APP_NAME=gugacode
VERSION=0.1.0
ARCH=amd64

if [ ! -x "$WORK/bin/${APP_NAME}" ]; then
  echo "Missing $WORK/bin/${APP_NAME}; run full wsl-package-all.sh first"
  exit 1
fi

# Fix version in work copy config awareness
cd "$WORK"

# Clean bad-named packages from previous run
rm -f "$WORK/bin"/*\'3\'* "$WORK/bin"/*"'3'"* 2>/dev/null || true
find "$WORK/bin" -maxdepth 1 -type f -name "*3*" ! -name 'gugacode' ! -name 'gugacode-server-*' 2>/dev/null | while read -r f; do
  case "$(basename "$f")" in
    gugacode-server-*) ;;
    *) rm -f "$f" || true ;;
  esac
done

# regenerate nfpm with correct version via sourcing package logic - inline
mkdir -p build/linux/nfpm/scripts
cat > "build/linux/${APP_NAME}.desktop" <<EOF
[Desktop Entry]
Type=Application
Name=gugacode
GenericName=Code Editor
Comment=AI-Powered Coding Desktop App
Exec=${APP_NAME}
Icon=${APP_NAME}
Categories=Development;IDE;
Terminal=false
StartupWMClass=${APP_NAME}
MimeType=text/plain;
EOF

cat > build/linux/nfpm/${APP_NAME}.yaml <<EOF
name: "${APP_NAME}"
arch: "${ARCH}"
platform: "linux"
version: "${VERSION}"
section: "devel"
priority: "optional"
maintainer: "gugacode contributors <dianasoylu423@gmail.com>"
description: |
  Offline-first desktop AI IDE for Go and TypeScript/JavaScript.
vendor: "gugacode"
homepage: "https://github.com/CuTeLiTTleBraids-Geek-studio/Gugacode"
license: "MIT"
release: "1"
contents:
  - src: "./bin/${APP_NAME}"
    dst: "/usr/local/bin/${APP_NAME}"
    file_info:
      mode: 0755
  - src: "./build/appicon.png"
    dst: "/usr/share/icons/hicolor/128x128/apps/${APP_NAME}.png"
  - src: "./build/linux/${APP_NAME}.desktop"
    dst: "/usr/share/applications/${APP_NAME}.desktop"
depends:
  - libgtk-3-0
  - libwebkit2gtk-4.1-0
overrides:
  rpm:
    depends:
      - gtk3
      - webkit2gtk4.1
  archlinux:
    depends:
      - gtk3
      - webkit2gtk-4.1
  apk:
    depends:
      - gtk+3.0
      - webkit2gtk-4.1
scripts:
  postinstall: "./build/linux/nfpm/scripts/postinstall.sh"
  postremove: "./build/linux/nfpm/scripts/postremove.sh"
EOF

cat > build/linux/nfpm/scripts/postinstall.sh <<'S'
#!/bin/sh
command -v update-desktop-database >/dev/null 2>&1 && update-desktop-database -q /usr/share/applications 2>/dev/null || true
exit 0
S
cat > build/linux/nfpm/scripts/postremove.sh <<'S'
#!/bin/sh
command -v update-desktop-database >/dev/null 2>&1 && update-desktop-database -q /usr/share/applications 2>/dev/null || true
exit 0
S
chmod +x build/linux/nfpm/scripts/*.sh

for fmt in deb rpm archlinux apk; do
  echo "[INFO] packing $fmt"
  nfpm pkg --config "build/linux/nfpm/${APP_NAME}.yaml" --packager "$fmt" --target "$WORK/bin/" || echo "warn $fmt failed"
done

# portable tar.gz + run
STAGE="$WORK/bin/stage-linux"
rm -rf "$STAGE" && mkdir -p "$STAGE"
cp "$WORK/bin/${APP_NAME}" "$STAGE/"
cp "build/linux/${APP_NAME}.desktop" "$STAGE/"
cp "build/appicon.png" "$STAGE/${APP_NAME}.png"
cat > "$STAGE/install.sh" <<'INSTALL'
#!/bin/bash
set -e
APP_NAME="gugacode"
INSTALL_DIR="/opt/${APP_NAME}"
BIN_PATH="/usr/local/bin/${APP_NAME}"
DESKTOP_FILE="/usr/share/applications/${APP_NAME}.desktop"
ICON_FILE="/usr/share/icons/hicolor/128x128/apps/${APP_NAME}.png"
if [ "$(id -u)" -ne 0 ]; then exec sudo bash "$0" "$@"; fi
mkdir -p "${INSTALL_DIR}"
cp "${APP_NAME}" "${INSTALL_DIR}/${APP_NAME}"
chmod +x "${INSTALL_DIR}/${APP_NAME}"
ln -sf "${INSTALL_DIR}/${APP_NAME}" "${BIN_PATH}"
[ -f "${APP_NAME}.png" ] && mkdir -p "$(dirname "${ICON_FILE}")" && cp "${APP_NAME}.png" "${ICON_FILE}"
[ -f "${APP_NAME}.desktop" ] && sed "s|^Exec=.*|Exec=${BIN_PATH}|" "${APP_NAME}.desktop" > "${DESKTOP_FILE}" && chmod 644 "${DESKTOP_FILE}"
echo "Installed: ${BIN_PATH}"
INSTALL
chmod +x "$STAGE/install.sh" "$STAGE/${APP_NAME}"
TARBALL="$WORK/bin/${APP_NAME}-${VERSION}-linux-${ARCH}-offline.tar.gz"
tar -czf "$TARBALL" -C "$STAGE" .
RUN_OUT="$WORK/bin/${APP_NAME}-${VERSION}-linux-${ARCH}-desktop.run"
{
  cat <<HEADER
#!/bin/bash
set -e
ARCHIVE_LINE=\$(grep -an '^__ARCHIVE_BELOW__\$' "\$0" | tail -1 | cut -d: -f1)
TMPDIR="/tmp/gugacode-install-\$(date +%s)-\$\$"
mkdir -p "\$TMPDIR"
echo "Extracting gugacode ${VERSION} offline desktop installer..."
tail -n +\$((ARCHIVE_LINE + 1)) "\$0" | tar xzf - -C "\$TMPDIR"
cd "\$TMPDIR"
bash install.sh
RC=\$?
rm -rf "\$TMPDIR"
exit \$RC
__ARCHIVE_BELOW__
HEADER
  cat "$TARBALL"
} > "$RUN_OUT"
chmod +x "$RUN_OUT"

# AppImage rename if present
if [ -f "$WORK/bin/gugacode-x86_64.AppImage" ]; then
  mv -f "$WORK/bin/gugacode-x86_64.AppImage" "$WORK/bin/${APP_NAME}-${VERSION}-linux-${ARCH}.AppImage"
elif ls "$WORK/bin"/*.AppImage >/dev/null 2>&1; then
  for a in "$WORK/bin"/*.AppImage; do
    mv -f "$a" "$WORK/bin/${APP_NAME}-${VERSION}-linux-${ARCH}.AppImage" || true
    break
  done
fi

# macOS/linux server runs with correct version
make_run() {
  local platform="$1" arch="$2" mode="$3" bin="$4" install_kind="$5"
  [ -f "$bin" ] || return 0
  local stage="$WORK/bin/stage-${platform}-${arch}-${mode}"
  rm -rf "$stage" && mkdir -p "$stage"
  cp "$bin" "$stage/gugacode-server"
  cp build/appicon.png "$stage/gugacode.png" 2>/dev/null || true
  if [ "$install_kind" = "macos" ]; then
    cat > "$stage/install.sh" <<'MAC'
#!/bin/bash
set -e
APP_BUNDLE="/Applications/gugacode.app"
[ "$(uname)" = "Darwin" ] || { echo "macOS only"; exit 1; }
rm -rf "$APP_BUNDLE"
mkdir -p "$APP_BUNDLE/Contents/MacOS" "$APP_BUNDLE/Contents/Resources"
cp gugacode-server "$APP_BUNDLE/Contents/MacOS/gugacode-server"
chmod +x "$APP_BUNDLE/Contents/MacOS/gugacode-server"
cat > "$APP_BUNDLE/Contents/MacOS/gugacode" <<'L'
#!/bin/bash
DIR="$(dirname "$0")"
"$DIR/gugacode-server" &
sleep 2
open "http://localhost:34115" 2>/dev/null || true
wait
L
chmod +x "$APP_BUNDLE/Contents/MacOS/gugacode"
cat > "$APP_BUNDLE/Contents/Info.plist" <<'P'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleExecutable</key><string>gugacode</string>
<key>CFBundleIdentifier</key><string>com.gugacode.app</string>
<key>CFBundleName</key><string>Gugacode</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>LSMinimumSystemVersion</key><string>12.0</string>
</dict></plist>
P
[ -f gugacode.png ] && cp gugacode.png "$APP_BUNDLE/Contents/Resources/" || true
codesign --force --deep --sign - "$APP_BUNDLE" 2>/dev/null || true
echo "Installed server-mode app: open $APP_BUNDLE"
MAC
  else
    cat > "$stage/install.sh" <<'LIN'
#!/bin/bash
set -e
if [ "$(id -u)" -ne 0 ]; then exec sudo bash "$0" "$@"; fi
mkdir -p /opt/gugacode
cp gugacode-server /opt/gugacode/gugacode
chmod +x /opt/gugacode/gugacode
ln -sf /opt/gugacode/gugacode /usr/local/bin/gugacode
echo "Installed server mode: /usr/local/bin/gugacode"
LIN
  fi
  chmod +x "$stage/install.sh"
  local suffix="${mode}"
  [ "$mode" = "server" ] && [ "$platform" = "linux" ] && suffix="server"
  local tg="$WORK/bin/${APP_NAME}-${VERSION}-${platform}-${arch}-${suffix}-offline.tar.gz"
  [ "$platform" = "darwin" ] && tg="$WORK/bin/${APP_NAME}-${VERSION}-${platform}-${arch}-offline.tar.gz"
  tar -czf "$tg" -C "$stage" .
  local run="$WORK/bin/${APP_NAME}-${VERSION}-${platform}-${arch}.run"
  [ "$platform" = "linux" ] && [ "$mode" = "server" ] && run="$WORK/bin/${APP_NAME}-${VERSION}-linux-${arch}-server.run"
  {
    cat <<HEADER
#!/bin/bash
set -e
ARCHIVE_LINE=\$(grep -an '^__ARCHIVE_BELOW__\$' "\$0" | tail -1 | cut -d: -f1)
TMPDIR="/tmp/gugacode-install-\$(date +%s)-\$\$"
mkdir -p "\$TMPDIR"
tail -n +\$((ARCHIVE_LINE + 1)) "\$0" | tar xzf - -C "\$TMPDIR"
cd "\$TMPDIR"
bash install.sh
RC=\$?
rm -rf "\$TMPDIR"
exit \$RC
__ARCHIVE_BELOW__
HEADER
    cat "$tg"
  } > "$run"
  chmod +x "$run"
  echo "[OK] $run"
}

make_run darwin amd64 server "$WORK/bin/gugacode-server-darwin-amd64" macos
make_run darwin arm64 server "$WORK/bin/gugacode-server-darwin-arm64" macos
make_run linux amd64 server "$WORK/bin/gugacode-server-linux-amd64" linux
make_run linux arm64 server "$WORK/bin/gugacode-server-linux-arm64" linux

# Clean stages
rm -rf "$WORK/bin"/stage-*

# SHA256
cd "$WORK/bin"
find . -maxdepth 1 -type f ! -name 'SHA256SUMS' -printf '%P\n' | sort | while read -r f; do
  sha256sum "$f"
done > SHA256SUMS

# Copy clean set to Windows bin (remove misnamed first)
mkdir -p "$OUT_WIN"
# remove previous bad version names
find "$OUT_WIN" -maxdepth 1 -type f \( -name "*'3'*" -o -name "*\"3\"*" \) -delete 2>/dev/null || true
# also remove files with weird quote version
python3 - <<'PY' || true
import os, glob
d = "/mnt/e/gugacode/Gugacode-main/bin"
for p in glob.glob(d + "/*"):
    b = os.path.basename(p)
    if "'3'" in b or '"3"' in b or "gugacode-'3'" in b:
        try: os.remove(p)
        except: pass
PY

cp -f "$WORK/bin/${APP_NAME}" "$OUT_WIN/" 2>/dev/null || true
for pat in \
  "${APP_NAME}_${VERSION}_${ARCH}.deb" \
  "${APP_NAME}-${VERSION}-1.${ARCH}.rpm" \
  "${APP_NAME}-${VERSION}-1-x86_64.pkg.tar.zst" \
  "${APP_NAME}-${VERSION}-1.${ARCH}.pkg.tar.zst" \
  "${APP_NAME}-${VERSION}-r1.apk" \
  "${APP_NAME}_${VERSION}-r1_x86_64.apk" \
  "${APP_NAME}-${VERSION}-linux-${ARCH}-offline.tar.gz" \
  "${APP_NAME}-${VERSION}-linux-${ARCH}-desktop.run" \
  "${APP_NAME}-${VERSION}-linux-${ARCH}.AppImage" \
  "${APP_NAME}-${VERSION}-darwin-amd64.run" \
  "${APP_NAME}-${VERSION}-darwin-arm64.run" \
  "${APP_NAME}-${VERSION}-darwin-amd64-offline.tar.gz" \
  "${APP_NAME}-${VERSION}-darwin-arm64-offline.tar.gz" \
  "${APP_NAME}-${VERSION}-linux-amd64-server.run" \
  "${APP_NAME}-${VERSION}-linux-arm64-server.run" \
  "${APP_NAME}-server-darwin-amd64" \
  "${APP_NAME}-server-darwin-arm64" \
  "${APP_NAME}-server-linux-amd64" \
  "${APP_NAME}-server-linux-arm64" \
  "SHA256SUMS"
do
  [ -f "$WORK/bin/$pat" ] && cp -f "$WORK/bin/$pat" "$OUT_WIN/"
done

# copy whatever nfpm actually named
for f in "$WORK/bin"/*.deb "$WORK/bin"/*.rpm "$WORK/bin"/*.apk "$WORK/bin"/*.pkg.tar.zst \
         "$WORK/bin"/*.tar.gz "$WORK/bin"/*.run "$WORK/bin"/*.AppImage; do
  [ -f "$f" ] || continue
  base=$(basename "$f")
  case "$base" in
    *\'3\'*|*"'3'"*) continue ;;
    *) cp -f "$f" "$OUT_WIN/" ;;
  esac
done
cp -f "$WORK/bin/SHA256SUMS" "$OUT_WIN/"

echo "=== Final artifacts ==="
ls -lh "$OUT_WIN" | sed -n '1,100p'
echo DONE
