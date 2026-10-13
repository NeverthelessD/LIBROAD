# LIBROAD

내 문서 도서관 — 맥에 흩어진 문서를 **찾고 · 분류하고 · 옮기고 · 정리**하는 앱 (macOS, Apple Silicon) — made by. Nevertheless_D

| 메뉴 | 하는 일 |
|---|---|
| **찾기** | 파일 이름 · 문서 안의 글자 · 초성(ㅅㅇㄱㅎ) · 스캔 문서와 사진 속 글자(OCR)까지. 서재 전체 또는 폴더 하나만. 종류 · 기간 · 크기로 거르기, 검색 저장, 미리 보기 |
| **분류하기** | 종류별 · 종류+연도 · 날짜별 · 나만의 규칙으로 폴더에 나눠 담기, 다운로드 폴더 자동 정리 |
| **정리하기** | 똑같은 문서(내용 비교) · 버전 정리(최종 · 최종2 · 수정본…) · 큰 파일 · 오래된 문서 · 빈 폴더 |
| **바구니** | 여러 번 찾아 고른 문서를 모아 두고 한꺼번에 옮기기 · 복사 · 이름 바꾸기 · 분류 · 휴지통 |
| **작업 기록** | 한 일을 모두 적어 두고 되돌리기 |

- 형식: PDF · 한글(hwp, hwpx) · 워드(doc, docx) · 엑셀(xls, xlsx, csv) · 파워포인트(ppt, pptx) · 텍스트(txt, md, rtf) · Pages · Keynote · Numbers · 전자책(epub) · 스캔 이미지(jpg, png, heic)
- 파일은 하나 또는 여러 개 고를 수 있어요 (⌘ · ⇧ 클릭, 모두 선택, 파일 고르기 창).
- 옮기기 · 이름 바꾸기 · 분류 · 휴지통은 **실행 전에 미리 보기**, 같은 이름은 덮어쓰지 않고 ‘(2)’, 지우기는 언제나 휴지통으로.
- 진행 막대에서 진행률과 **남은 시간**을 보여 줘요.
- OCR은 macOS의 글자 인식(Vision)을 써요. 설정에서 켜고 끌 수 있고, 찾을 때 ‘OCR’ 버튼으로 빼고 빠르게 찾을 수도 있어요.
- 라이트/다크 모드(기본: macOS 설정), 자동 업데이트 + 수동 확인 · 이전 버전 되돌리기.
- **모든 작업은 이 Mac 안에서만** 해요. 문서 내용은 어디에도 보내지 않아요.

## 설치

1. [`releases/`](releases) 폴더에서 가장 최신 `LIBROAD-x.y.z.zip`을 받아 압축을 풀어요.
2. `LIBROAD.app`을 **응용 프로그램** 폴더로 옮겨요. (업데이트가 동작하려면 꼭 옮겨 주세요)
3. 처음 열 때 경고가 뜨면 **시스템 설정 → 개인정보 보호 및 보안 → "그래도 열기"**.
4. “문서 · 바탕 화면 · 다운로드 폴더에 접근하려고 합니다” 창이 뜨면 **허용**.

색인 · 설정은 `~/Library/Application Support/LIBROAD` 한 폴더에만 저장돼요.
지우려면 앱을 휴지통에 버리고 그 폴더도 지우면 끝이에요.

## 사용한 오픈소스

- [ledongthuc/pdf](https://github.com/ledongthuc/pdf) (BSD) — PDF 글자 읽기 (맥에서는 PDFKit으로 한 번 더)
- [golang.org/x/text](https://pkg.go.dev/golang.org/x/text) (BSD) — 예전 한국어 텍스트(CP949) · 한글 자모 합치기
- 글꼴: [Pretendard](https://github.com/orioncactus/pretendard), [DM Serif Display](https://fonts.google.com/specimen/DM+Serif+Display) (SIL OFL)

## 폴더 구조

```
engine/     Go 엔진 (로컬 서버 · 색인 · 문서 읽기 · 찾기 · 옮기기 · 정리 · 업데이트) + helper.js (PDFKit · Vision · 휴지통)
engine/web/ 앱 화면 (HTML · CSS · JS)
app/        LIBROAD.app 껍데기 (실행 스크립트 · Info.plist · 아이콘)
scripts/    빌드 스크립트 · 시험 도구
releases/   업데이트 파일 (LIBROAD-x.y.z.zip, latest.json)
docs/       사용 가이드 이미지
```
