#!/bin/bash
# LIBROAD 빌드 스크립트 — 맥용 LIBROAD.app 을 만들고
# 업데이트용 releases/LIBROAD-<버전>.zip 과 releases/latest.json 을 씁니다.
#
#   scripts/build.sh                          # VERSION 파일의 버전으로 빌드
#   scripts/build.sh --notes "고친 점 1" "고친 점 2"
#
# 필요: Go 1.24+, python3. Linux/macOS 어디서든 크로스 빌드돼요.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
VERSION="$(tr -d ' \n' < "$ROOT/VERSION")"
NOTES=()
if [ "${1:-}" = "--notes" ]; then shift; NOTES=("$@"); fi

DIST="$ROOT/dist"
APP="$DIST/LIBROAD.app"
rm -rf "$DIST"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources" "$ROOT/releases"

for pair in "arm64:arm64" "amd64:x86_64"; do
  GA="${pair%%:*}"; MA="${pair##*:}"
  echo "▶ 엔진 빌드 (darwin/$GA, v$VERSION)"
  ( cd "$ROOT/engine" && GOTOOLCHAIN=local CGO_ENABLED=0 GOOS=darwin GOARCH="$GA" \
      go build -trimpath -ldflags "-s -w -X main.build=$VERSION" -o "$APP/Contents/Resources/libroad-$MA" . )
  chmod 755 "$APP/Contents/Resources/libroad-$MA"
done

cp "$ROOT/app/launcher.sh" "$APP/Contents/MacOS/LIBROAD"
chmod 755 "$APP/Contents/MacOS/LIBROAD"
sed "s/__VERSION__/$VERSION/g" "$ROOT/app/Info.plist" > "$APP/Contents/Info.plist"
cp "$ROOT/app/AppIcon.icns" "$APP/Contents/Resources/AppIcon.icns"
printf '%s' "$VERSION" > "$APP/Contents/Resources/version.txt"
printf 'APPL????' > "$APP/Contents/PkgInfo"

ZIP="LIBROAD-$VERSION.zip"
python3 "$ROOT/scripts/zipapp.py" "$DIST" "LIBROAD.app" "$ROOT/releases/$ZIP"
cp "$ROOT/releases/$ZIP" "$DIST/LIBROAD.zip"
python3 "$ROOT/scripts/feed.py" "$ROOT/releases/latest.json" "$VERSION" "$ROOT/releases/$ZIP" "${NOTES[@]+"${NOTES[@]}"}"
echo "✔ dist/LIBROAD.zip (처음 설치용) · releases/$ZIP + latest.json (자동 업데이트용)"
