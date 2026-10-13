#!/usr/bin/env python3
"""Linux에서 helper.js 대신 쓰는 시험용 도우미 (pdftotext + tesseract kor+eng).
LIBROAD_HELPER_CMD="python3 scripts/test/fakehelper.py" 로 쓰고, TESSDATA_PREFIX 에 kor 데이터를 두세요."""
import json, os, shutil, subprocess, sys, tempfile, time

def emit(o):
    sys.stdout.write(json.dumps(o, ensure_ascii=False) + "\n"); sys.stdout.flush()

def pages_of(p):
    out = subprocess.run(["pdfinfo", p], capture_output=True, text=True).stdout
    for l in out.splitlines():
        if l.startswith("Pages:"):
            return int(l.split()[1])
    return 0

def ocr_img(p):
    r = subprocess.run(["tesseract", p, "-", "-l", "kor+eng"], capture_output=True, text=True)
    return r.stdout

mode = sys.argv[1]
if mode == "check":
    emit({"check": True, "vision": True, "pdfkit": True, "langs": ["ko-KR", "en-US"], "os": "Linux test"}); sys.exit()
items = json.load(open(sys.argv[2]))
maxp = int(sys.argv[3]) if len(sys.argv) > 3 else 60
delay = float(os.environ.get("FAKEHELPER_DELAY", "0"))
for p in items:
    try:
        if mode == "pdftext":
            t = subprocess.run(["pdftotext", "-layout", p, "-"], capture_output=True, text=True).stdout
            emit({"path": p, "ok": True, "text": t, "pages": pages_of(p)})
        elif mode == "ocr":
            if p.lower().endswith(".pdf"):
                n = min(pages_of(p), maxp); texts = []
                with tempfile.TemporaryDirectory() as td:
                    for i in range(1, n + 1):
                        subprocess.run(["pdftoppm", "-r", "150", "-f", str(i), "-l", str(i), "-png", p, os.path.join(td, "pg")], check=True)
                        img = sorted(f for f in os.listdir(td) if f.endswith(".png"))[-1]
                        texts.append(ocr_img(os.path.join(td, img))); time.sleep(delay)
                        emit({"tick": 1, "path": p, "page": i, "pages": n})
                emit({"path": p, "ok": True, "text": "\n".join(texts), "pages": pages_of(p)})
            else:
                t = ocr_img(p); time.sleep(delay)
                emit({"tick": 1, "path": p, "page": 1, "pages": 1})
                emit({"path": p, "ok": True, "text": t, "pages": 1})
        elif mode == "trash":
            trash = os.environ.get("FAKE_TRASH", "/tmp/fake-trash"); os.makedirs(trash, exist_ok=True)
            dst = os.path.join(trash, os.path.basename(p)); n = 2
            while os.path.exists(dst):
                b, e = os.path.splitext(os.path.basename(p)); dst = os.path.join(trash, f"{b} {n}{e}"); n += 1
            shutil.move(p, dst)
            emit({"path": p, "ok": True, "trashed": dst})
    except Exception as e:
        emit({"path": p, "ok": False, "error": str(e)})
