# LIBROAD — notes for Claude

LIBROAD is a personal macOS (Apple Silicon) app for **Nevertheless_D**, who is not a programmer.
Talk to them in Korean, in plain words. Never ask them to use Terminal; give them a double-clickable result.
It is a "document library": search (file name, body text, 초성, OCR text), classify, move/copy/rename/trash with undo, tidy up.

## What it is

A "script-type" app: `LIBROAD.app/Contents/MacOS/LIBROAD` is a bash launcher (`app/launcher.sh`). It copies
`Contents/Resources/libroad-<arm64|x86_64>` (Go binaries) to `~/Library/Application Support/LIBROAD/bin/libroad-engine`, starts it on
127.0.0.1 with a random token, and opens the UI in a Chrome/Edge/Brave `--app` window (or the default browser).
The engine stays alive while the window is open, 30 min after the last request, or forever while 자동 정리 is on.

- `engine/` — Go + `golang.org/x/text` (pinned v0.27.0: newer needs a newer Go; build with `GOTOOLCHAIN=local`) + `github.com/ledongthuc/pdf`. Embeds `web/` and `helper.js`.
  - `main.go` routes/settings (appearance, roots, ocr, notify, autoUpdate, rules, auto, basket, saved, lastDest, seen), folder/file pickers (osascript), open/reveal/Quick Look
  - `kinds.go` formats. `Mac: true` kinds (pages/keynote/numbers) are dropped on Windows.
  - `extract.go` txt/md/csv (UTF-8/UTF-16/CP949), rtf, docx/pptx/xlsx/hwpx/epub (zip XML) · `cfb.go` OLE2 reader · `legacy.go` hwp (BodyText records, PrvText fallback for 배포용/암호) / doc (piece table) / xls (SST+CONTINUE) / ppt (text atoms), textutil fallback · `iwork.go` pages/key/numbers (IWA = framed snappy + protobuf; TSWP storage 2001/2005 field 3, Numbers TableDataList 6005; loose fallback; '09 XML) · `pdf.go` PDF in a child process (`libroad-engine --pdftext <file>`, 90 s timeout); garbled / spaceless / empty → `StRetry`
  - `helper.js` JXA run by `osascript -l JavaScript helper.js <check|pdftext|ocr|trash> list.json [maxPages]` — PDFKit text, Vision OCR (ko-KR + en-US, macOS 13+ for Korean), NSFileManager trash. JSON lines out (`tick` lines = one OCR page). **Untested on a real Mac by Claude — verify there.** `helper.go` runs it with an idle watchdog.
  - `library.go` index (gob+gzip `index.gob.gz`, version `indexVersion`), incremental by size+mtime, rescans every 3 min or on Kick, iCloud dataless files → `StCloud` (name only), states New→Text/Retry→OCRWait→OCRDone/Fail/Enc. OCR ≤ 60 pages per PDF, images ≤ 25 MB, newest first, stoppable.
  - `search.go` AND terms, "phrases", `-exclude`, 초성 (file names), scope (root or any folder under it), kind/period/size, `noBody` / `noOcr`, sort, snippets with marks, Spotlight (`mdfind`) supplement while the index is still reading. Thumbnails via `qlmanage -t` (cached in Support/thumbs).
  - `ops.go` plan (dry run) → run (job) → history (`history.json`, undo). Never overwrites: `uniquePath` adds " (2)". Trash via helper (resulting URL; fallback finds the file in ~/.Trash by size+mtime). Undo removes only folders the op created. `organize.go` modes kind/kinddate/date/rules, default rules, 자동 정리 loop. `cleanup.go` dupes (size → quick hash → sha256), versions (`versionStem` regexes), big, old, empty folders (top-most only). `jobs.go` progress + smoothed ETA.
  - `convert.go` PDF로 바꾸기: images via `sips`, Pages/Keynote/Numbers via the iWork app (AppleScript export, falls back to LibreOffice), everything else via LibreOffice headless (`--convert-to pdf`, own profile `Support/tools/lo-profile`, one at a time, 4 min timeout), merge via helper.js `merge` (PDFKit). 변환 도구 = LibreOffice dmg (newest under download.documentfoundation.org/libreoffice/stable, sha256 from `.sha256`) + Temurin 21 JRE (api.adoptium.net; LibreOffice finds it via `JAVA_HOME`) + H2Orestart.oxt (hwp/hwpx import, Java; installed with `unopkg add`), all into `Support/tools`, then a `--terminate_after_init` warm-up. Do **not** pass `--infilter=Hwp2002_Reader` (fails); instead the PDF of an hwp is checked with `pdfLooksFine` and converted once more if LibreOffice fell back to plain-text import. txt/md/csv are re-encoded to UTF-8 first. Created PDFs are history items with act `create` (undo = trash).
  - `update.go` auto-update from `releases/latest.json` in this repo (raw.githubusercontent.com), sha256 check, swaps the .app (backup in Support/backup for rollback), restart keeps port+token so the open window reloads. Windows paths (`latest-win.json`, `LIBROAD.exe`) are kept for later.
- `engine/web/` — `index.html`, `css/app.css`, `js/art.js` (SVG illustrations + drawn book covers), `js/app.js` (core, search, selection, preview, jobs strip, shortcuts), `js/ops.js` (plan preview, move/copy/rename/organize/trash dialogs, undo toast), `js/views.js` (분류하기 · 정리하기 · 바구니 · 작업 기록), `js/panels.js` (settings, update, welcome, init). Classic scripts sharing globals, loaded in that order.

Design: library green (#1f5c45) + brass (#b0822f/#d7ad5f) on cream paper; dark = walnut/ink. Serif = DM Serif Display (+ AppleMyungjo for Korean). Kind colours live in `kinds.go`.
Footer must say `made by. Nevertheless_D`.

## Releasing an update (this is how the user's app updates itself)

1. Make the change. Bump `VERSION` (semver, e.g. 1.0.1).
2. `scripts/build.sh --notes "바뀐 점 1" "바뀐 점 2"` (Korean, user-facing). Writes `releases/LIBROAD-<v>.zip` and `releases/latest.json`.
3. Commit everything (including the zip and latest.json) and push to `main`. Installed apps pick it up within 6 hours or via the version badge.
4. Keep only the three newest zips in `releases/`.
5. Also send the user `dist/LIBROAD.zip` for a fresh install if they ask.

Never publish a `latest.json` whose zip is missing or whose `version.txt` differs — the app verifies both.
Commit as `Nevertheless_D <NeverthelessD@users.noreply.github.com>`.

## Testing without a Mac

- Samples: `python3 scripts/test/make_samples.py /tmp/lrhome <pyhwp tests/hwp5_tests/fixtures>` (docx/xlsx/pptx, LibreOffice-made doc/xls/ppt/rtf/pdf, scanned pdf + images, hwpx, real hwp from pyhwp, epub, fake IWA pages/numbers/key package, dupes, big/old/iCloud files).
- `LIBROAD_SAMPLES=/tmp/lrhome LIBROAD_SELF=/tmp/libroad go test -run 'TestSamples|TestVersionStem|TestChoseong' -v ./engine`
- Run: `GOTOOLCHAIN=local go build -o /tmp/libroad ./engine`, then with `HOME` (fake home with Documents/Desktop/Downloads), `LIBROAD_SUPPORT`, `LIBROAD_TOKEN`, `LIBROAD_PORT`,
  `LIBROAD_HELPER_CMD="python3 scripts/test/fakehelper.py"` (pdftotext + tesseract; `TESSDATA_PREFIX` with kor.traineddata from tessdata_fast; `FAKE_TRASH` dir),
  `LIBROAD_TRASH` (fake trash without the helper), `LIBROAD_TEST_CHOOSE` / `LIBROAD_TEST_CHOOSE_FILES` (`a|b`), `LIBROAD_FAKE_DATALESS=1` (`*.icloudtest.*` = iCloud-only),
  `LIBROAD_AUTO_EVERY` (seconds, auto-organize), `LIBROAD_TEST_SLOW` (ms per file, to see progress/ETA), `LIBROAD_FAKE_OS=windows`.
- Update test: fake `LIBROAD.app` with a Linux engine named `libroad-x86_64` + `version.txt` + launcher; serve a zip + latest.json with `python3 -m http.server`;
  run the launcher with `HOME=<tmp> LIBROAD_UPDATE_URL=http://127.0.0.1:PORT/latest.json bash LIBROAD.app/Contents/MacOS/LIBROAD`, then check → install → restart → rollback via the API.
- PDF test: LibreOffice for Linux (`LibreOffice_<v>_Linux_x86-64_deb.tar.gz`, unpack the debs with `dpkg-deb -x`), `LIBROAD_LO_DIR=<…>/opt/libreoffice<v>/program`, `LIBROAD_H2O_URL=/path/H2Orestart.oxt` (local file), run the engine as a **non-root** user (`runuser -u claude`; unopkg refuses root). Real HWP samples: hwp.js repo (`website/static/files/noori.hwp`, `packages/parser/src/__tests__/data/basicsReport.hwp`); the pyhwp fixtures don't convert.
- Drive the UI with Playwright (Chromium is preinstalled). Thumbnails 404 off-mac (the UI falls back to drawn covers).

Things only a real Mac can confirm: PDFKit/Vision/trash/merge helper (JXA), the 변환 도구 install (hdiutil, Temurin tarball layout `jre/*/Contents/Home`), iWork export via AppleScript (Automation prompt), Quick Look thumbnails and 훑어보기, Spotlight, osascript pickers,
privacy prompts for 문서/바탕 화면/다운로드 (Info.plist has the usage strings), Chrome app window, Gatekeeper.

## Later: Windows

Same features, but drop pages/key/numbers (already done by `kinds.go` on GOOS=windows; `isPackageExt` is false there). Needed: a Windows launcher like
YTROAD's (`launcher_windows.go`, one exe = launcher + `--engine`), in-app folder/file picker (`/api/fs/list` exists; UI switches on `settings.os`),
OCR via Windows.Media.Ocr (PowerShell) or skip, Recycle Bin (Shell API) instead of helper.js, no Quick Look/Spotlight/textutil, `latest-win.json` feed.

## User-facing text

Never mention GitHub (or the repo name) in anything the user sees — app screens, update messages, usage-guide images.
Say "새 버전이 나오면 앱이 알아서 받아요" instead. The user asked for this for all of their apps.
