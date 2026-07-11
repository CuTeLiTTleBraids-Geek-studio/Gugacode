#!/usr/bin/env bash
set -euo pipefail
export PATH="/usr/local/go/bin:/usr/local/lib/nodejs/bin:${HOME}/go/bin:${PATH}"
export APPIMAGE_EXTRACT_AND_RUN=1

WORK="${HOME}/gugacode-pkg-build"
OUT="/mnt/e/gugacode/Gugacode-main/bin"
APP=gugacode
VER=0.1.0
ARCH=amd64
ADIR="${WORK}/build/linux/appimage"
mkdir -p "$ADIR"
cd "$ADIR"

APPDIR="${APP}.AppDir"
rm -rf "$APPDIR"
mkdir -p "$APPDIR/usr/bin"
cp "$WORK/bin/${APP}" "$APPDIR/usr/bin/"
cp "$WORK/build/appicon.png" "$APPDIR/${APP}.png"
cat > "$APPDIR/${APP}.desktop" <<'D'
[Desktop Entry]
Type=Application
Name=gugacode
Exec=gugacode
Icon=gugacode
Categories=Development;IDE;
Terminal=false
D
cat > "$APPDIR/AppRun" <<'A'
#!/bin/bash
HERE="$(dirname "$(readlink -f "$0")")"
exec "$HERE/usr/bin/gugacode" "$@"
A
chmod +x "$APPDIR/AppRun" "$APPDIR/usr/bin/${APP}"

LD=linuxdeploy-x86_64.AppImage
if [ ! -f "$LD" ]; then
  wget -q -4 -N "https://github.com/linuxdeploy/linuxdeploy/releases/download/continuous/$LD"
  chmod +x "$LD"
fi
./"$LD" --appdir "$APPDIR" --output appimage
mv -f ${APP}*.AppImage "$WORK/bin/${APP}-${VER}-linux-${ARCH}.AppImage"
cp -f "$WORK/bin/${APP}-${VER}-linux-${ARCH}.AppImage" "$OUT/"
rm -f "$OUT/gugacode-3-1-x86_64.pkg.tar.zst" 2>/dev/null || true

cd "$OUT"
sha256sum \
  gugacode \
  gugacode_0.1.0-1_amd64.deb \
  gugacode-0.1.0-1.x86_64.rpm \
  gugacode-0.1.0-1-x86_64.pkg.tar.zst \
  gugacode_0.1.0-r1_x86_64.apk \
  gugacode-0.1.0-linux-amd64-offline.tar.gz \
  gugacode-0.1.0-linux-amd64-desktop.run \
  gugacode-0.1.0-linux-amd64.AppImage \
  gugacode-0.1.0-darwin-amd64.run \
  gugacode-0.1.0-darwin-arm64.run \
  gugacode-0.1.0-linux-amd64-server.run \
  gugacode-0.1.0-linux-arm64-server.run \
  2>/dev/null > SHA256SUMS || true

ls -lh "$OUT"/gugacode*0.1.0* "$OUT"/gugacode "$OUT"/SHA256SUMS 2>/dev/null | head -50
echo APPIMAGE_OK
