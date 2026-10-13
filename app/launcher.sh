#!/bin/bash
# ─────────────────────────────────────────────────────────────
#  LIBROAD — 내 문서 도서관 (macOS)
#  made by. Nevertheless_D
#
#  더블클릭하면 이 스크립트가 실행됩니다.
#  1) 앱 안의 엔진을 앱 전용 폴더로 복사하고  2) 엔진을 켠 뒤  3) 앱 화면을 창으로 엽니다.
#  문서 색인 · 설정은 ~/Library/Application Support/LIBROAD 에만 저장돼요. 문서 내용은 밖으로 보내지 않아요.
# ─────────────────────────────────────────────────────────────
export PATH="/usr/bin:/bin:/usr/sbin:/sbin"

HERE="$(cd "$(dirname "$0")" && pwd)"
APP="$(cd "$HERE/../.." && pwd)"
RES="$HERE/../Resources"
SUPPORT="$HOME/Library/Application Support/LIBROAD"
BIN="$SUPPORT/bin"
BUILD="$(cat "$RES/version.txt" 2>/dev/null)"
mkdir -p "$BIN"

dialog() {
  osascript -e 'on run argv' \
    -e 'display dialog (item 1 of argv) with title "LIBROAD" buttons {"확인"} default button 1 with icon caution' \
    -e 'end run' "$1" >/dev/null 2>&1
}

# 앱 창 열기: Chrome·Edge·Brave가 있으면 주소창 없는 "앱 창"으로, 없으면 기본 브라우저(Safari 등)로 엽니다.
open_ui() {
  local url="http://127.0.0.1:$1/?t=$2" b
  for b in "Google Chrome" "Microsoft Edge" "Brave Browser"; do
    if [ -d "/Applications/$b.app" ] || [ -d "$HOME/Applications/$b.app" ]; then
      open -na "$b" --args --app="$url" --window-size=1380,920 && return
    fi
  done
  open "$url"
}

ping_engine() { # port token
  curl -fs -m 2 -H "x-token: $2" "http://127.0.0.1:$1/api/ping" 2>/dev/null
}

RESTART="${LIBROAD_RESTART:-}"
PREF_PORT=""
TOKEN=""

# 1) 이미 실행 중이면 창만 다시 열기 (다른 버전이 켜져 있으면 끄고 새로 시작)
if [ -z "$RESTART" ] && [ -s "$SUPPORT/port" ] && [ -s "$SUPPORT/token" ]; then
  P="$(cat "$SUPPORT/port")"; T="$(cat "$SUPPORT/token")"
  R="$(ping_engine "$P" "$T")"
  case "$R" in
    *"\"build\":\"$BUILD\""*) open_ui "$P" "$T"; exit 0 ;;
    *"\"ok\":true"*)
      curl -fs -m 3 -X POST -H "x-token: $T" "http://127.0.0.1:$P/api/quit" >/dev/null 2>&1
      sleep 1.2 ;;
  esac
fi

# 업데이트 후 다시 시작: 같은 포트·토큰을 써서 열려 있던 창이 그대로 이어지게 합니다
if [ -n "$RESTART" ]; then
  PREF_PORT="${LIBROAD_PORT:-}"
  TOKEN="${LIBROAD_TOKEN:-}"
  for _ in $(seq 1 30); do
    [ -n "$PREF_PORT" ] && [ -n "$(ping_engine "$PREF_PORT" "$TOKEN")" ] || break
    sleep 0.3
  done
fi

# 2) 엔진을 앱 전용 폴더로 복사해서 실행
#    (다운로드 폴더에서 바로 실행하거나, 나중에 앱을 옮기거나 지워도 영향을 받지 않도록)
ARCH="$(uname -m)"
SRC="$RES/libroad-$ARCH"
[ -f "$SRC" ] || SRC="$RES/libroad-arm64"
ENGINE="$BIN/libroad-engine"
if [ ! -x "$ENGINE" ] || [ "$(cat "$BIN/build" 2>/dev/null)" != "$BUILD-$ARCH" ]; then
  rm -f "$ENGINE.new"
  if cat "$SRC" > "$ENGINE.new" && chmod 755 "$ENGINE.new" && mv -f "$ENGINE.new" "$ENGINE"; then
    printf '%s' "$BUILD-$ARCH" > "$BIN/build"
  fi
fi
xattr -c "$ENGINE" 2>/dev/null
if [ ! -x "$ENGINE" ]; then
  dialog "😢 LIBROAD를 준비하지 못했어요.
LIBROAD를 응용 프로그램 폴더로 옮긴 뒤 다시 실행해 주세요."
  exit 1
fi

# 3) 엔진 켜기
[ -n "$TOKEN" ] || TOKEN="$(LC_ALL=C tr -dc 'A-Za-z0-9' </dev/urandom 2>/dev/null | head -c 32)"
rm -f "$SUPPORT/port"
LIBROAD_TOKEN="$TOKEN" LIBROAD_SUPPORT="$SUPPORT" LIBROAD_APP="$APP" LIBROAD_PORT="$PREF_PORT" LIBROAD_RESTART="$RESTART" \
  nohup "$ENGINE" > "$SUPPORT/engine.out" 2>&1 < /dev/null &
disown

for _ in $(seq 1 75); do [ -s "$SUPPORT/port" ] && break; sleep 0.2; done
if [ ! -s "$SUPPORT/port" ]; then
  dialog "😢 LIBROAD를 시작하지 못했어요. 앱을 다시 실행해 보세요.
계속 안 되면 컴퓨터를 재시동한 뒤 시도해 주세요."
  exit 1
fi
PORT="$(cat "$SUPPORT/port")"

# 4) 앱 창 열기 (다시 시작이고 같은 포트라면 열려 있던 창이 스스로 새로고침해요)
if [ -n "$RESTART" ] && [ "$PORT" = "$PREF_PORT" ]; then
  exit 0
fi
open_ui "$PORT" "$TOKEN"
exit 0
