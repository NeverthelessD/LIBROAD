// LIBROAD helper — macOS 기본 기능(PDFKit · Vision · 휴지통)을 쓰는 작은 도우미
// osascript -l JavaScript helper.js <mode> <list.json> [maxPages]
//   mode: check | pdftext | ocr | trash
// 결과는 한 줄에 하나씩 JSON으로 내보내요. 문서 내용은 이 컴퓨터 밖으로 나가지 않아요.
// made by. Nevertheless_D
ObjC.import('Foundation');
ObjC.import('AppKit');
try { ObjC.import('PDFKit'); } catch (e) { try { ObjC.import('Quartz'); } catch (e2) {} }
var HAS_VISION = true;
try { ObjC.import('Vision'); } catch (e) { HAS_VISION = false; }

var OUT = $.NSFileHandle.fileHandleWithStandardOutput;
var MAX_TEXT = 600000;

function emit(obj) {
  var s = JSON.stringify(obj) + '\n';
  OUT.writeData($(s).dataUsingEncoding($.NSUTF8StringEncoding));
}

function readList(file) {
  var s = $.NSString.stringWithContentsOfFileEncodingError(file, $.NSUTF8StringEncoding, null);
  var js = ObjC.unwrap(s);
  return js ? JSON.parse(js) : [];
}

function str(o) {
  try { var v = ObjC.unwrap(o); return typeof v === 'string' ? v : ''; } catch (e) { return ''; }
}

function fileURL(p) { return $.NSURL.fileURLWithPath(p); }

function openPDF(p) {
  var doc = $.PDFDocument.alloc.initWithURL(fileURL(p));
  if (!doc || doc.isNil()) return null;
  return doc;
}

// ───────────────────────── 글자 인식 (Vision)
var LANGS = null;
function langs(req) {
  if (LANGS) return LANGS;
  var want = ['ko-KR', 'en-US'];
  var have = [];
  try {
    var arr = req.supportedRecognitionLanguagesAndReturnError(null);
    var n = arr.count;
    for (var i = 0; i < n; i++) have.push(str(arr.objectAtIndex(i)));
  } catch (e) {
    try {
      var arr2 = $.VNRecognizeTextRequest.supportedRecognitionLanguagesForTextRecognitionLevelRevisionError(0, 2, null);
      for (var j = 0; j < arr2.count; j++) have.push(str(arr2.objectAtIndex(j)));
    } catch (e2) {}
  }
  LANGS = have.length ? want.filter(function (l) { return have.indexOf(l) >= 0; }) : want;
  if (!LANGS.length) LANGS = ['en-US'];
  return LANGS;
}

function newRequest() {
  var req = $.VNRecognizeTextRequest.alloc.init;
  req.setRecognitionLevel(0); // 0 = 정확하게
  req.setUsesLanguageCorrection(true);
  try { req.setRecognitionLanguages($(langs(req))); } catch (e) {}
  return req;
}

function recognize(handler) {
  var req = newRequest();
  handler.performRequestsError($.NSArray.arrayWithObject(req), null);
  var res = req.results;
  var lines = [];
  if (!res || res.isNil()) return '';
  var n = res.count;
  for (var i = 0; i < n; i++) {
    var obs = res.objectAtIndex(i);
    var cands = obs.topCandidates(1);
    if (cands && cands.count > 0) {
      var s = str(cands.objectAtIndex(0).string);
      if (s) lines.push(s);
    }
  }
  return lines.join('\n');
}

function ocrImage(p) {
  var handler = $.VNImageRequestHandler.alloc.initWithURLOptions(fileURL(p), $.NSDictionary.dictionary);
  return recognize(handler);
}

function ocrPDF(p, maxPages) {
  var doc = openPDF(p);
  if (!doc) throw new Error('PDF를 열 수 없어요');
  if (doc.isLocked) return { text: '', pages: doc.pageCount, encrypted: true };
  var pages = doc.pageCount;
  var n = Math.min(pages, maxPages);
  var parts = [];
  for (var i = 0; i < n; i++) {
    var pool = $.NSAutoreleasePool.alloc.init;
    try {
      var pg = doc.pageAtIndex(i);
      var w = 612, h = 792;
      try { var b = pg.boundsForBox(0); if (b && b.size && b.size.width > 0 && b.size.height > 0) { w = b.size.width; h = b.size.height; } } catch (e) {}
      var scale = Math.min(3, 2200 / Math.max(w, h));
      var tiff = null;
      try {
        var img = pg.thumbnailOfSizeForBox({ width: Math.round(w * scale), height: Math.round(h * scale) }, 0);
        tiff = img.TIFFRepresentation;
      } catch (e) {
        // 그림으로 바꾸는 다른 방법 (해상도는 조금 낮아요)
        var img2 = $.NSImage.alloc.initWithData(pg.dataRepresentation);
        tiff = img2.TIFFRepresentation;
      }
      var handler = $.VNImageRequestHandler.alloc.initWithDataOptions(tiff, $.NSDictionary.dictionary);
      var t = recognize(handler);
      if (t) parts.push(t);
    } catch (e) {}
    emit({ tick: 1, path: p, page: i + 1, pages: n });
    pool.drain;
    if (parts.join('\n').length > MAX_TEXT) break;
  }
  return { text: parts.join('\n'), pages: pages };
}

// ───────────────────────── PDF 글자 (PDFKit)
function pdfText(p) {
  var doc = openPDF(p);
  if (!doc) throw new Error('PDF를 열 수 없어요');
  if (doc.isLocked) return { text: '', pages: doc.pageCount, encrypted: true };
  var pages = doc.pageCount;
  var parts = [], len = 0;
  for (var i = 0; i < pages && i < 800; i++) {
    var s = str(doc.pageAtIndex(i).string);
    if (s) { parts.push(s); len += s.length; }
    if (len > MAX_TEXT) break;
  }
  return { text: parts.join('\n'), pages: pages };
}

// ───────────────────────── 휴지통
function trash(p) {
  var fm = $.NSFileManager.defaultManager;
  var ref = Ref();
  var ok = fm.trashItemAtURLResultingItemURLError(fileURL(p), ref, null);
  var where = '';
  try { where = str(ref[0].path); } catch (e) {}
  return { ok: !!ok, trashed: where };
}

function run(argv) {
  var mode = argv[0];
  if (mode === 'check') {
    var o = { check: true, vision: false, langs: [], os: str($.NSProcessInfo.processInfo.operatingSystemVersionString) };
    try {
      if (HAS_VISION && $.VNRecognizeTextRequest) {
        var req = $.VNRecognizeTextRequest.alloc.init;
        o.vision = !!req && !req.isNil();
        o.langs = langs(req);
      }
    } catch (e) { o.error = String(e); }
    try { o.pdfkit = !!$.PDFDocument && typeof $.PDFDocument.alloc === 'object'; } catch (e) { o.pdfkit = false; }
    emit(o);
    return;
  }
  var list = readList(argv[1]);
  if (mode === 'merge') {
    // 여러 PDF를 차례대로 하나로 합치기
    var outPath = argv[2];
    try {
      var out = $.PDFDocument.alloc.init;
      for (var m = 0; m < list.length; m++) {
        var src = openPDF(list[m]);
        if (!src) throw new Error('PDF를 열 수 없어요: ' + list[m]);
        if (src.isLocked) throw new Error('암호가 걸린 PDF는 합칠 수 없어요');
        var n = src.pageCount;
        for (var k = 0; k < n; k++) out.insertPageAtIndex(src.pageAtIndex(k), out.pageCount);
      }
      var ok = out.writeToFile(outPath);
      emit({ path: outPath, ok: !!ok, pages: out.pageCount });
    } catch (e) {
      emit({ path: outPath, ok: false, error: String(e) });
    }
    return;
  }
  var maxPages = parseInt(argv[2] || '60', 10);
  for (var i = 0; i < list.length; i++) {
    var p = list[i];
    try {
      if (mode === 'pdftext') {
        var r = pdfText(p);
        emit({ path: p, ok: true, text: r.text, pages: r.pages, encrypted: !!r.encrypted });
      } else if (mode === 'ocr') {
        var lower = p.toLowerCase();
        if (lower.slice(-4) === '.pdf') {
          var r2 = ocrPDF(p, maxPages);
          emit({ path: p, ok: true, text: r2.text, pages: r2.pages, encrypted: !!r2.encrypted });
        } else {
          var t = ocrImage(p);
          emit({ tick: 1, path: p, page: 1, pages: 1 });
          emit({ path: p, ok: true, text: t, pages: 1 });
        }
      } else if (mode === 'trash') {
        var r3 = trash(p);
        emit({ path: p, ok: r3.ok, trashed: r3.trashed, error: r3.ok ? '' : '휴지통으로 옮기지 못했어요' });
      }
    } catch (e) {
      emit({ path: p, ok: false, error: String(e) });
    }
  }
}
