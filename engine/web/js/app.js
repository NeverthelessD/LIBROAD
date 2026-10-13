// LIBROAD — 화면: 공통 · 찾기 · 고르기 · 미리 보기
// made by. Nevertheless_D
'use strict';

const T = new URLSearchParams(location.search).get('t') || '';
const $ = (sel, root = document) => root.querySelector(sel);
const $$ = (sel, root = document) => [...root.querySelectorAll(sel)];
const esc = (s) => String(s ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));

async function api(path, body) {
  const init = { method: body === undefined ? 'GET' : 'POST', headers: { 'x-token': T } };
  if (body !== undefined) { init.body = JSON.stringify(body); init.headers['content-type'] = 'application/json'; }
  let r;
  try { r = await fetch(path, init); } catch { throw new Error('앱 엔진과 연결이 끊겼어요. LIBROAD를 다시 실행해 주세요.'); }
  let j = {};
  try { j = await r.json(); } catch { /* ignore */ }
  if (!r.ok || j.ok === false) throw new Error(j.error || `요청에 실패했어요 (${r.status})`);
  return j;
}

const S = {
  settings: null, kinds: [], lib: null, jobs: [], update: null, view: 'search',
  q: { q: '', scope: '', kinds: [], period: '', from: 0, to: 0, size: '', noBody: false, noOcr: false, sort: '' },
  res: null, items: [], sel: new Set(), anchor: -1, cur: '', viewMode: 'list',
  basket: 0, histSig: 0, libVer: 0, recent: null,
};
window.KINDS = {};

// ════════════════════════════════════════════════ 작은 도우미
let toastTimer;
function toast(msg, ms = 3400, action) {
  const t = $('#toast');
  t.innerHTML = esc(msg) + (action ? ` <button class="link" style="color:var(--brass2);margin-left:8px">${esc(action.label)}</button>` : '');
  if (action) t.querySelector('button').onclick = () => { t.hidden = true; action.fn(); };
  t.hidden = false;
  t.style.animation = 'none'; void t.offsetWidth; t.style.animation = '';
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => (t.hidden = true), action ? Math.max(ms, 7000) : ms);
}
function fmtSize(b) {
  if (!b) return '0 KB';
  if (b < 1024 * 1024) return Math.max(1, Math.round(b / 1024)) + ' KB';
  if (b < 1024 ** 3) return (b / 1048576).toFixed(b < 10 * 1048576 ? 1 : 0) + ' MB';
  return (b / 1024 ** 3).toFixed(2) + ' GB';
}
const pad2 = (n) => String(n).padStart(2, '0');
function fmtDate(sec, long = false) {
  if (!sec) return '—';
  const d = new Date(sec * 1000), now = new Date();
  const days = Math.floor((new Date(now.getFullYear(), now.getMonth(), now.getDate()) - new Date(d.getFullYear(), d.getMonth(), d.getDate())) / 86400000);
  const hm = `${d.getHours() < 12 ? '오전' : '오후'} ${((d.getHours() + 11) % 12) + 1}:${pad2(d.getMinutes())}`;
  if (!long) {
    if (days === 0) return '오늘 ' + hm;
    if (days === 1) return '어제';
    if (days < 7) return days + '일 전';
  }
  const s = `${d.getFullYear()}. ${d.getMonth() + 1}. ${d.getDate()}.`;
  return long ? `${s} ${hm}` : s;
}
function fmtWhen(ms) {
  if (!ms) return '아직 확인 안 함';
  const d = new Date(ms), now = new Date();
  const hm = d.toLocaleTimeString('ko-KR', { hour: 'numeric', minute: '2-digit' });
  return d.toDateString() === now.toDateString() ? `오늘 ${hm}` : `${d.getMonth() + 1}월 ${d.getDate()}일 ${hm}`;
}
function fmtETA(s) {
  s = Math.round(s);
  if (!s || s < 0) return '';
  if (s < 10) return '거의 다 됐어요';
  if (s < 60) return `약 ${Math.ceil(s / 5) * 5}초 남음`;
  const m = Math.floor(s / 60), x = s % 60;
  if (m < 60) return `약 ${m}분${m < 10 && x >= 10 ? ` ${Math.round(x / 10) * 10}초` : ''} 남음`;
  return `약 ${Math.floor(m / 60)}시간 ${m % 60}분 남음`;
}
const comma = (n) => Number(n || 0).toLocaleString('ko-KR');
const L = { fm: 'Finder', mod: '⌘', os: 'macOS', win: false };
function setPlatform(os) {
  if (os === 'windows') Object.assign(L, { fm: '탐색기', mod: 'Ctrl', os: 'Windows', win: true });
  document.documentElement.dataset.os = L.win ? 'windows' : 'mac';
  if (L.win) $$('kbd').forEach((k) => (k.textContent = k.textContent.replace('⌘', 'Ctrl+')));
}
function shortPath(p) {
  const home = S.settings?.home;
  if (!L.win && home && (p === home || p.startsWith(home + '/'))) p = '~' + p.slice(home.length);
  return p;
}
const baseName = (p) => String(p || '').split(/[\\/]/).filter(Boolean).pop() || p;
const dirName = (p) => String(p || '').replace(/[\\/][^\\/]*$/, '') || '/';
const KNAME = { Documents: '문서', Desktop: '바탕 화면', Downloads: '다운로드' };
function folderLabel(p) {
  const b = baseName(p);
  if (S.settings && dirName(p) === S.settings.home && KNAME[b]) return KNAME[b];
  return b;
}
function kindOf(id) { return window.KINDS[id] || { id, label: '기타', color: '#7a7066', short: '?', emoji: '📄' }; }
const thumbURL = (p, s = 320) => `/api/thumb?t=${encodeURIComponent(T)}&s=${s}&p=${encodeURIComponent(p)}`;

// 표지: 그림(썸네일)이 오면 그림, 없으면 그린 표지
function coverHTML(d, withThumb) {
  const svg = coverSVG(d.kind, d.ext, d.name);
  if (!withThumb || d.cloud) return svg;
  return svg + `<img src="${thumbURL(d.path)}" alt="" loading="lazy" onerror="this.remove()">`;
}

// ════════════════════════════════════════════════ 화면 모드
function applyTheme(v) {
  if (v === 'light' || v === 'dark') document.documentElement.dataset.theme = v;
  else delete document.documentElement.dataset.theme;
  try { localStorage.setItem('libroad-theme', v || 'system'); } catch { /* ignore */ }
  $$('#themeToggle button').forEach((b) => b.classList.toggle('on', b.dataset.theme === (v || 'system')));
}
$$('#themeToggle button').forEach((b) => (b.onclick = async () => {
  applyTheme(b.dataset.theme);
  S.settings = await api('/api/settings', { appearance: b.dataset.theme }).catch(() => S.settings);
}));

// ════════════════════════════════════════════════ 창 (모달) · 메뉴
const M = { open: false, onClose: null, stack: [] };
function openModal({ title, sub = '', body, actions = [], onClose, wide = false }) {
  M.open = true; M.onClose = onClose || null;
  $('#modalTitle').textContent = title;
  $('#modalSub').textContent = sub;
  $('#modalBox').classList.toggle('wide', wide);
  setModalActions(actions);
  const b = $('#modalBody'); b.innerHTML = ''; b.append(body);
  $('#modal').hidden = false;
  paintArt(b);
}
function setModalActions(actions) {
  const a = $('#modalActions'); a.innerHTML = '';
  for (const act of actions) {
    const btn = document.createElement('button');
    btn.className = 'btn ' + (act.primary ? 'primary' : act.danger ? 'danger' : 'ghost') + (act.left ? ' left' : '');
    btn.textContent = act.label; btn.disabled = !!act.disabled; btn.onclick = act.onClick;
    if (act.id) btn.id = act.id;
    a.append(btn);
  }
  a.hidden = !actions.length;
}
function closeModal() {
  if (!M.open) return;
  M.open = false; $('#modal').hidden = true;
  const cb = M.onClose; M.onClose = null; cb && cb();
  // 창이 열려 있는 동안 서재가 바뀌었으면 (지운 · 옮긴 파일 등) 결과를 새로 고쳐요
  if (S.needRefresh && !M.open) { S.needRefresh = false; if (S.view === 'search' && S.res) runSearch({ keepSel: true }); }
}
// 다른 앱(Finder 등)에서 파일을 옮기거나 지우고 돌아오면 서재를 바로 다시 살펴요
let lastFocusScan = 0;
window.addEventListener('focus', () => {
  if (Date.now() - lastFocusScan < 20000) return;
  lastFocusScan = Date.now();
  api('/api/library/rescan', {}).catch(() => {});
  if (S.view === 'search' && S.res && !M.open) setTimeout(() => runSearch({ keepSel: true }), 300);
});
$('#modalClose').onclick = closeModal;
$('#modal').addEventListener('mousedown', (e) => { if (e.target.id === 'modal') closeModal(); });

// 간단한 입력 창
function askText(title, label, value = '', okLabel = '확인') {
  return new Promise((resolve) => {
    const body = document.createElement('div');
    body.innerHTML = `<div class="field"><label>${esc(label)}</label><input class="input" id="askIn" value="${esc(value)}"></div>`;
    let done = false;
    const finish = (v) => { if (done) return; done = true; closeModal(); resolve(v); };
    openModal({ title, body, actions: [{ label: '취소', onClick: () => finish(null) }, { label: okLabel, primary: true, onClick: () => finish($('#askIn').value.trim()) }], onClose: () => finish(null) });
    const inp = $('#askIn'); inp.focus(); inp.select();
    inp.onkeydown = (e) => { if (e.key === 'Enter') { e.preventDefault(); finish(inp.value.trim()); } };
  });
}
function confirmBox(title, msg, okLabel = '확인', danger = false) {
  return new Promise((resolve) => {
    const body = document.createElement('div');
    body.innerHTML = `<p style="margin:4px 0 6px;line-height:1.6">${msg}</p>`;
    let done = false;
    const finish = (v) => { if (done) return; done = true; closeModal(); resolve(v); };
    openModal({ title, body, actions: [{ label: '취소', onClick: () => finish(false) }, { label: okLabel, primary: !danger, danger, onClick: () => finish(true) }], onClose: () => finish(false) });
  });
}

function openMenu(anchor, items) {
  const m = $('#menu');
  m.innerHTML = '';
  for (const it of items) {
    if (it === '-') { m.append(document.createElement('hr')); continue; }
    if (it.cap) { const c = document.createElement('div'); c.className = 'mcap'; c.textContent = it.cap; m.append(c); continue; }
    const b = document.createElement('button');
    b.innerHTML = `${it.icon || ''} <span style="overflow:hidden;text-overflow:ellipsis;white-space:nowrap">${esc(it.label)}</span>${it.note ? `<small>${esc(it.note)}</small>` : ''}`;
    if (it.on) b.classList.add('on');
    b.onclick = () => { closeMenu(); it.onClick(); };
    m.append(b);
  }
  m.hidden = false;
  const r = anchor.getBoundingClientRect();
  m.style.left = Math.min(r.left, innerWidth - m.offsetWidth - 10) + 'px';
  m.style.top = Math.min(r.bottom + 6, innerHeight - m.offsetHeight - 10) + 'px';
  setTimeout(() => document.addEventListener('mousedown', menuAway, true), 0);
}
function menuAway(e) { if (!$('#menu').contains(e.target)) closeMenu(); }
function closeMenu() { $('#menu').hidden = true; document.removeEventListener('mousedown', menuAway, true); }

// ════════════════════════════════════════════════ 폴더 고르기
async function chooseFolder(prompt, start) {
  if (S.settings.os === 'windows') return pickFolderWeb(start || S.settings.documents, prompt);
  const j = await api('/api/choose-folder', { start: start || '', prompt }).catch((e) => { toast('⚠️ ' + e.message); return null; });
  return j?.path || '';
}
function pickFolderWeb(start, title) {
  return new Promise((resolve) => {
    const el = document.createElement('div');
    el.className = 'fp-back';
    el.innerHTML = `<div class="fp"><div class="fp-head"><b>📁 폴더 고르기</b><small>${esc(title || '')}</small></div>
      <div class="fp-main"><div class="fp-side" id="fpSide"></div><div class="fp-list-wrap">
      <div class="fp-path"><button class="btn ghost sm" id="fpUp">⬆︎</button><span id="fpPath"></span></div><div class="fp-list" id="fpList"></div></div></div>
      <div class="fp-foot"><button class="btn ghost sm" id="fpNew">＋ 새 폴더</button><span class="sp"></span><button class="btn ghost" id="fpCancel">취소</button><button class="btn primary" id="fpOk">이 폴더 선택</button></div></div>`;
    document.body.append(el);
    let cur = null;
    const done = (v) => { el.remove(); resolve(v); };
    async function go(path) {
      let j; try { j = await api('/api/fs/list', { path }); } catch (e) { toast('⚠️ ' + e.message); return; }
      cur = j;
      $('#fpPath', el).textContent = '‎' + j.path + '‎';
      $('#fpUp', el).disabled = !j.parent;
      $('#fpSide', el).innerHTML = ['<div class="fp-cap">바로 가기</div>', ...j.places.map((p) => `<button data-p="${esc(p.path)}">${p.icon} ${esc(p.name)}</button>`),
        '<div class="fp-cap">디스크</div>', ...j.drives.map((p) => `<button data-p="${esc(p.path)}">${p.icon} ${esc(p.name)}</button>`)].join('');
      $('#fpList', el).innerHTML = j.dirs.length ? j.dirs.map((d) => `<button data-p="${esc(d.path)}">📁 ${esc(d.name)}</button>`).join('')
        : `<div class="fp-empty">${j.readable ? '안에 다른 폴더가 없어요' : '이 폴더는 열 수 없어요'}</div>`;
      $$('[data-p]', el).forEach((b) => (b.onclick = () => go(b.dataset.p)));
    }
    $('#fpUp', el).onclick = () => cur?.parent && go(cur.parent);
    $('#fpCancel', el).onclick = () => done('');
    $('#fpOk', el).onclick = () => done(cur?.path || '');
    $('#fpNew', el).onclick = async () => {
      const name = await askTextInline('새 폴더 이름', 'LIBROAD');
      if (!name || !cur) return;
      try { const j = await api('/api/fs/mkdir', { path: cur.path, name }); go(j.path); } catch (e) { toast('⚠️ ' + e.message); }
    };
    go(start);
  });
}
async function askTextInline(label, v) { return window.prompt(label, v); }

// ════════════════════════════════════════════════ 보기 바꾸기
function setView(v) {
  S.view = v;
  $$('#nav button').forEach((b) => b.classList.toggle('on', b.dataset.view === v));
  $$('.view').forEach((el) => (el.hidden = el.id !== 'view-' + v));
  if (v !== 'search' && v !== 'basket') { $('#preview').hidden = true; }
  else if (S.cur && v === 'search') $('#preview').hidden = false;
  else if (v === 'basket') { $('#preview').hidden = true; }
  renderSelbar();
  if (window.VIEWS && VIEWS[v]) VIEWS[v]();
}
$$('#nav button').forEach((b) => (b.onclick = () => setView(b.dataset.view)));

// ════════════════════════════════════════════════ 찾기
let searchTimer = null, searchSeq = 0;
function hasFilter() {
  const q = S.q;
  return q.kinds.length || q.period || q.size || q.scope;
}
function scheduleSearch(ms = 220) { clearTimeout(searchTimer); searchTimer = setTimeout(runSearch, ms); }

async function runSearch(opts = {}) {
  const q = S.q;
  const active = q.q.trim() !== '' || hasFilter();
  $('#searchHead').classList.toggle('compact', active);
  $('#browse').hidden = active;
  $('#results').hidden = !active;
  $('#qClear').hidden = !q.q;
  if (!active) { S.res = null; S.items = []; clearSel(); renderChips(); renderBrowse(); return; }
  const seq = ++searchSeq;
  const req = { ...q, limit: 200, offset: opts.more ? S.items.length : 0 };
  let r;
  try { r = await api('/api/search', req); } catch (e) { toast('😢 ' + e.message); return; }
  if (seq !== searchSeq) return;
  S.res = r;
  S.libVer = r.ver;
  S.items = opts.more ? S.items.concat(r.items) : r.items;
  if (!opts.more && !opts.keepSel) clearSel(false);
  if (opts.keepSel) { const have = new Set(S.items.map((i) => i.path)); for (const p of [...S.sel]) if (!have.has(p)) S.sel.delete(p); }
  renderChips();
  renderResults();
}

function renderChips() {
  const counts = S.res?.kindCount || S.lib?.byKind || {};
  $('#kindChips').innerHTML = S.kinds.map((k) => {
    const n = counts[k.id] || 0;
    const on = S.q.kinds.includes(k.id);
    return `<button class="chip${on ? ' on' : ''}${!n && !on ? ' zero' : ''}" data-k="${k.id}" style="--c:${k.color}" title="${esc(k.exts.join(', '))}"><span class="sw"></span>${esc(k.label)}<b>${n ? comma(n) : ''}</b></button>`;
  }).join('');
  $$('#kindChips .chip').forEach((b) => (b.onclick = (e) => {
    const k = b.dataset.k;
    if (e.metaKey || e.ctrlKey || e.shiftKey || S.q.kinds.length !== 1 || !S.q.kinds.includes(k)) {
      if (e.metaKey || e.ctrlKey || e.shiftKey) S.q.kinds = S.q.kinds.includes(k) ? S.q.kinds.filter((x) => x !== k) : [...S.q.kinds, k];
      else S.q.kinds = S.q.kinds.length === 1 && S.q.kinds[0] === k ? [] : [k];
    } else S.q.kinds = [];
    runSearch();
  }));
}

function snipHTML(segs) { return segs.map((s) => (s.m ? `<mark>${esc(s.t)}</mark>` : esc(s.t))).join(''); }
function nameHTML(name) {
  const terms = (S.res?.terms || []).filter((t) => !t.ex && !t.cho).map((t) => t.t.toLowerCase()).filter(Boolean);
  if (!terms.length) return esc(name);
  const low = name.toLowerCase();
  const mask = new Array(name.length).fill(false);
  for (const t of terms) { let i = 0; while ((i = low.indexOf(t, i)) >= 0) { for (let k = i; k < i + t.length; k++) mask[k] = true; i += t.length; } }
  let out = '', cur = false, buf = '';
  for (let i = 0; i < name.length; i++) {
    if (mask[i] !== cur) { out += cur ? `<mark>${esc(buf)}</mark>` : esc(buf); buf = ''; cur = mask[i]; }
    buf += name[i];
  }
  return out + (cur ? `<mark>${esc(buf)}</mark>` : esc(buf));
}
function badgesHTML(d) {
  const b = [];
  if (d.hName) b.push('<span class="bdg n">이름</span>');
  if (d.hFolder) b.push('<span class="bdg n">폴더</span>');
  if (d.hBody) b.push('<span class="bdg b">본문</span>');
  if (d.hOcr) b.push('<span class="bdg o">OCR</span>');
  if (d.spot) b.push('<span class="bdg">Spotlight</span>');
  if (d.cloud) b.push('<span class="bdg w">iCloud</span>');
  if (d.state === 'enc') b.push('<span class="bdg w">암호</span>');
  return b.join('');
}

function renderResults() {
  const r = S.res;
  const list = $('#resList');
  const items = S.items;
  const scopeTxt = S.q.scope ? `‘${folderLabel(S.q.scope)}’에서 ` : '';
  const ms = r.took < 1 ? '0.01초' : (r.took / 1000).toFixed(2) + '초';
  $('#resCount').innerHTML = `${scopeTxt}<b>${comma(r.total)}</b>권 찾았어요 <span style="color:var(--faint)">· ${fmtSize(r.totalSize)} · ${ms}</span>`;
  const notes = [];
  if (r.pending > 0) notes.push(`📖 아직 ${comma(r.pending)}개 문서의 내용을 읽는 중이에요. 다 읽으면 결과가 더 늘어날 수 있어요.${r.spot ? ` (맥의 Spotlight로 ${r.spot}개를 더 찾았어요)` : ''}`);
  if (S.q.noOcr && S.lib?.pending?.ocrDone) notes.push('⚡ OCR을 뺀 빠른 검색이에요. 스캔 문서 · 사진 속 글자는 찾지 않아요.');
  if (S.q.noBody) notes.push('🏷️ 파일 이름과 폴더 이름만 찾고 있어요.');
  $('#resNote').hidden = !notes.length;
  $('#resNote').innerHTML = notes.join('<br>');
  $('#resEmpty').hidden = items.length > 0;
  $('#resEmptyTip').textContent = S.q.noBody ? '‘본문’을 켜면 문서 안의 글자까지 찾아요' : S.q.noOcr ? '‘OCR’을 켜면 스캔 문서 · 사진 속 글자도 찾아요' : S.q.kinds.length || S.q.period || S.q.size ? '거르기(종류 · 기간 · 크기)를 풀어 보세요' : '다른 낱말이나 초성으로 찾아보세요';
  $('#moreBtn').hidden = items.length >= r.total;
  $('#moreBtn').textContent = `더 보기 (${comma(r.total - items.length)}개 더)`;
  list.classList.toggle('grid', S.viewMode === 'grid');
  if (S.viewMode === 'grid') {
    list.innerHTML = items.map((d, i) => `
      <div class="gcard${S.sel.has(d.path) ? ' on' : ''}" data-i="${i}" title="${esc(d.path)}">
        <label class="ck"><input type="checkbox" ${S.sel.has(d.path) ? 'checked' : ''}><span></span></label>
        <div class="cover">${coverHTML(d, true)}</div>
        <div class="gt">${nameHTML(d.name)}</div><div class="gm">${fmtDate(d.mod)} · ${fmtSize(d.size)}</div>
      </div>`).join('');
  } else {
    list.innerHTML = items.map((d, i) => {
      const sn = (d.snips || []).slice(0, 1).map(snipHTML).join(' ');
      return `<div class="row${S.sel.has(d.path) ? ' on' : ''}${S.cur === d.path ? ' cur' : ''}" data-i="${i}">
        <label class="ck"><input type="checkbox" ${S.sel.has(d.path) ? 'checked' : ''}><span></span></label>
        <div class="mc">${coverSVG(d.kind, d.ext, d.name)}</div>
        <div class="rt"><div class="rn" title="${esc(d.name)}">${nameHTML(d.name)}</div>
          <div class="rp" title="${esc(d.dir)}">📁 ${esc(shortPath(d.dir))}</div>
          ${sn ? `<div class="rs">${sn}</div>` : d.note ? `<div class="rs" style="color:var(--faint)">${esc(d.note)}</div>` : ''}</div>
        <div class="rm"><div class="badges">${badgesHTML(d)}</div><span>${fmtDate(d.mod)}</span><span>${fmtSize(d.size)}${d.pages ? ` · ${d.pages}쪽` : ''}</span></div>
      </div>`;
    }).join('');
  }
  updateSelAll();
  renderSelbar();
}

// 결과 클릭 · 고르기
function rowIndex(e) { const el = e.target.closest('[data-i]'); return el ? +el.dataset.i : -1; }
$('#resList').addEventListener('click', (e) => {
  const i = rowIndex(e); if (i < 0) return;
  const d = S.items[i];
  if (e.target.closest('.ck')) {
    if (e.target.tagName !== 'INPUT') return;
    toggleSel(d.path, e.target.checked); S.anchor = i; return;
  }
  if (e.shiftKey && S.anchor >= 0) {
    const [a, b] = [Math.min(S.anchor, i), Math.max(S.anchor, i)];
    if (!(e.metaKey || e.ctrlKey)) S.sel.clear();
    for (let k = a; k <= b; k++) S.sel.add(S.items[k].path);
  } else if (e.metaKey || e.ctrlKey) {
    S.sel.has(d.path) ? S.sel.delete(d.path) : S.sel.add(d.path);
    S.anchor = i;
  } else {
    S.sel.clear(); S.sel.add(d.path); S.anchor = i;
  }
  paintSel();
  showPreview(d.path);
});
$('#resList').addEventListener('dblclick', (e) => {
  const i = rowIndex(e); if (i < 0 || e.target.closest('.ck')) return;
  openDoc(S.items[i].path, 'open');
});
function toggleSel(p, on) { on ? S.sel.add(p) : S.sel.delete(p); paintSel(); }
function clearSel(paint = true) { S.sel.clear(); S.anchor = -1; if (paint) paintSel(); }
function paintSel() {
  $$('#resList [data-i]').forEach((el) => {
    const d = S.items[+el.dataset.i]; if (!d) return;
    const on = S.sel.has(d.path);
    el.classList.toggle('on', on);
    el.classList.toggle('cur', S.cur === d.path);
    const c = el.querySelector('input'); if (c) c.checked = on;
  });
  updateSelAll();
  renderSelbar();
}
function updateSelAll() {
  const n = S.items.filter((d) => S.sel.has(d.path)).length;
  const c = $('#selAll');
  c.checked = n > 0 && n === S.items.length;
  c.indeterminate = n > 0 && n < S.items.length;
}
$('#selAll').onchange = (e) => { if (e.target.checked) S.items.forEach((d) => S.sel.add(d.path)); else S.sel.clear(); paintSel(); };
$('#moreBtn').onclick = () => runSearch({ more: true });
$$('#viewMode button').forEach((b) => (b.onclick = () => {
  S.viewMode = b.dataset.v;
  $$('#viewMode button').forEach((x) => x.classList.toggle('on', x === b));
  try { localStorage.setItem('libroad-viewmode', S.viewMode); } catch { /* */ }
  renderResults();
}));

function selectedPaths() {
  if (S.view === 'basket' && window.basketSelected) return window.basketSelected();
  return [...S.sel];
}
function renderSelbar() {
  const n = S.view === 'search' ? S.sel.size : 0;
  $('#selbar').hidden = n === 0;
  $('#selN').textContent = `${comma(n)}개 고름`;
}
$('#selbar').addEventListener('click', (e) => {
  const a = e.target.closest('[data-act]')?.dataset.act; if (!a) return;
  if (a === 'clear') return clearSel();
  doAction(a, [...S.sel]);
});

// ════════════════════════════════════════════════ 검색 입력 · 거르기
const qIn = $('#q');
qIn.addEventListener('input', () => { S.q.q = qIn.value; scheduleSearch(qIn.value.length < 2 ? 380 : 200); });
qIn.addEventListener('keydown', (e) => {
  if (e.key === 'Enter') { e.preventDefault(); clearTimeout(searchTimer); runSearch(); }
  if (e.key === 'ArrowDown' && S.items.length) { e.preventDefault(); qIn.blur(); moveCur(1); }
  if (e.key === 'Escape' && qIn.value) { e.preventDefault(); e.stopPropagation(); qIn.value = ''; S.q.q = ''; runSearch(); }
});
$('#goBtn').onclick = () => { clearTimeout(searchTimer); runSearch(); };
$('#qClear').onclick = () => { qIn.value = ''; S.q.q = ''; runSearch(); qIn.focus(); };
function paintToggles() {
  $('#togBody').classList.toggle('on', !S.q.noBody);
  $('#togOcr').classList.toggle('on', !S.q.noOcr && !S.q.noBody);
  $('#togOcr').disabled = S.q.noBody;
  $('#togOcr').title = S.q.noOcr ? 'OCR 제외 중 — 스캔 문서 · 사진 속 글자는 빼고 빠르게 찾아요. 눌러서 다시 포함' : '스캔 문서 · 사진의 글자(OCR)도 찾기. 눌러서 빼면 더 빨라요';
}
$('#togBody').onclick = () => { S.q.noBody = !S.q.noBody; paintToggles(); savePrefs(); if (S.q.q) runSearch(); };
$('#togOcr').onclick = () => {
  S.q.noOcr = !S.q.noOcr; paintToggles(); savePrefs();
  toast(S.q.noOcr ? '⚡ OCR 제외: 스캔 문서 · 사진 속 글자는 빼고 빠르게 찾아요' : '🔎 OCR 포함: 스캔 문서 · 사진 속 글자도 찾아요');
  if (S.q.q) runSearch();
};
function savePrefs() { try { localStorage.setItem('libroad-q', JSON.stringify({ noBody: S.q.noBody, noOcr: S.q.noOcr })); } catch { /* */ } }
$('#fPeriod').onchange = (e) => {
  S.q.period = e.target.value;
  $('#customRange').hidden = S.q.period !== 'custom';
  if (S.q.period !== 'custom') runSearch();
};
const dateSec = (v) => (v ? Math.floor(new Date(v + 'T00:00:00').getTime() / 1000) : 0);
$('#fFrom').onchange = $('#fTo').onchange = () => { S.q.from = dateSec($('#fFrom').value); S.q.to = dateSec($('#fTo').value); runSearch(); };
$('#fSize').onchange = (e) => { S.q.size = e.target.value; runSearch(); };
$('#fSort').onchange = (e) => { S.q.sort = e.target.value; runSearch(); };

// 찾을 곳 (서재 전체 · 폴더 하나)
function scopeLabel() { return S.q.scope ? folderLabel(S.q.scope) : '서재 전체'; }
function paintScope() {
  $('#scopeName').textContent = scopeLabel();
  $('#scopeBtn').title = S.q.scope ? S.q.scope : '서재에 넣은 모든 폴더에서 찾아요';
  $$('#roots .root').forEach((r) => r.classList.toggle('on', r.dataset.p === S.q.scope));
}
function setScope(p) { S.q.scope = p || ''; paintScope(); runSearch(); }
$('#scopeBtn').onclick = (e) => {
  const roots = S.lib?.roots || [];
  const items = [{ icon: '📚', label: '서재 전체', note: comma(S.lib?.docs || 0) + '권', on: !S.q.scope, onClick: () => setScope('') }, { cap: '폴더 하나만' }];
  for (const r of roots) items.push({ icon: '📁', label: folderLabel(r.path), note: comma(r.count) + '권', on: S.q.scope === r.path, onClick: () => setScope(r.path) });
  if (S.q.scope && !roots.some((r) => r.path === S.q.scope)) items.push({ icon: '📂', label: folderLabel(S.q.scope), note: '지금', on: true, onClick: () => setScope(S.q.scope) });
  items.push('-', { icon: '🔍', label: '다른 폴더 고르기…', onClick: pickScopeFolder });
  openMenu(e.currentTarget, items);
};
async function pickScopeFolder() {
  const p = await chooseFolder('이 폴더 안에서만 찾을게요', S.q.scope || S.settings.documents);
  if (!p) return;
  const roots = S.settings.roots || [];
  if (!roots.some((r) => p === r || p.startsWith(r + '/') || p.startsWith(r + '\\'))) {
    try {
      S.settings = await api('/api/library/roots', { action: 'add', path: p });
      toast(`📚 ‘${folderLabel(p)}’ 폴더를 서재에 넣었어요. 살펴보는 동안 결과가 점점 늘어나요.`, 5000);
    } catch (e) { toast('⚠️ ' + e.message); return; }
  }
  setScope(p);
}

// 저장한 검색
function renderSaved() {
  const list = S.settings?.saved || [];
  const row = $('#savedRow');
  row.hidden = !list.length;
  row.innerHTML = '<span class="cap">⭐ 저장한 검색</span>' + list.map((s) => `<span class="saved-chip" data-id="${s.id}"><button class="go">${esc(s.name)}</button><button class="x" title="지우기">✕</button></span>`).join('');
  $$('.saved-chip', row).forEach((el) => {
    const s = list.find((x) => x.id === el.dataset.id);
    el.querySelector('.go').onclick = () => applyQuery(s.query);
    el.querySelector('.x').onclick = async () => {
      const r = await api('/api/saved', { action: 'remove', id: s.id }).catch(() => null);
      if (r) { S.settings.saved = r.saved; renderSaved(); }
    };
  });
}
function applyQuery(q) {
  Object.assign(S.q, { q: '', scope: '', kinds: [], period: '', from: 0, to: 0, size: '', sort: '' }, q || {});
  qIn.value = S.q.q;
  $('#fPeriod').value = S.q.period; $('#fSize').value = S.q.size; $('#fSort').value = S.q.sort;
  $('#customRange').hidden = S.q.period !== 'custom';
  paintScope(); paintToggles();
  setView('search');
  runSearch();
}
$('#saveSearch').onclick = async () => {
  const def = S.q.q || (S.q.kinds.map((k) => kindOf(k).label).join('·') || scopeLabel());
  const name = await askText('⭐ 이 검색 저장', '이름을 붙여 두면 다음에 한 번에 다시 찾을 수 있어요', def, '저장');
  if (!name) return;
  const r = await api('/api/saved', { action: 'add', name, query: { ...S.q } }).catch((e) => { toast('⚠️ ' + e.message); return null; });
  if (r) { S.settings.saved = r.saved; renderSaved(); toast('⭐ 저장했어요'); }
};

// ════════════════════════════════════════════════ 검색 전: 서가 둘러보기
async function renderBrowse(force) {
  if (!S.recent || force) S.recent = await api('/api/recent').catch(() => null);
  const r = S.recent;
  if (r) {
    $('#recentShelf').innerHTML = r.recent.length ? r.recent.map((d) => `
      <div class="book" data-p="${esc(d.path)}" title="${esc(d.path)}"><div class="cover">${coverHTML(d, true)}</div><div class="bt">${esc(d.name)}</div></div>`).join('')
      : `<div class="empty" style="width:100%;padding:10px 0 18px"><p>${S.lib?.ready ? '아직 서재가 비어 있어요' : '서재를 살피는 중이에요…'}</p><small>왼쪽 ‘내 서재’에 문서 폴더를 넣어 보세요</small></div>`;
    $('#addedList').innerHTML = r.added.slice(0, 8).map((d) => `
      <div class="mini" data-p="${esc(d.path)}"><span class="spine" style="--c:${kindOf(d.kind).color}"></span><div class="mt"><b>${esc(d.name)}</b><small>${fmtDate(d.birth)} · ${esc(folderLabel(d.dir))}</small></div></div>`).join('') || '<div class="note">아직 없어요</div>';
    $$('#recentShelf .book, #addedList .mini').forEach((el) => {
      el.onclick = () => showPreview(el.dataset.p);
      el.ondblclick = () => openDoc(el.dataset.p, 'open');
    });
  }
  const by = S.lib?.byKind || {};
  const maxN = Math.max(1, ...Object.values(by));
  $('#kindGuide').innerHTML = S.kinds.map((k) => `
    <button data-k="${k.id}" style="--c:${k.color}"><span>${k.emoji}</span><span>${esc(k.label)}</span><span class="gbar"><i style="width:${((by[k.id] || 0) / maxN) * 100}%"></i></span><span class="gn">${comma(by[k.id] || 0)}</span></button>`).join('');
  $$('#kindGuide button').forEach((b) => (b.onclick = () => { S.q.kinds = [b.dataset.k]; runSearch(); }));
}
$('#seeAllRecent').onclick = () => { S.q.sort = 'new'; $('#fSort').value = 'new'; S.q.period = 'month'; $('#fPeriod').value = 'month'; runSearch(); };

// ════════════════════════════════════════════════ 미리 보기
let pvSeq = 0;
async function showPreview(p) {
  S.cur = p;
  paintSel();
  if (S.view !== 'search' && S.view !== 'basket') return;
  const pv = $('#preview');
  pv.hidden = false;
  const seq = ++pvSeq;
  let d;
  try { d = await api('/api/doc', { path: p, q: S.q.q, req: S.q }); } catch (e) { pv.innerHTML = `<div class="pv-sec">${esc(e.message)}</div>`; return; }
  if (seq !== pvSeq) return;
  const i = d.info, k = kindOf(i.kind);
  const isImg = i.kind === 'image';
  const stateTxt = { new: '내용을 읽을 차례를 기다리는 중이에요', retry: 'PDF를 한 번 더 읽는 중이에요', ocrwait: S.settings.ocr ? '글자 인식(OCR)을 기다리는 중이에요' : 'OCR이 꺼져 있어서 이름으로만 찾아요 (설정에서 켤 수 있어요)', fail: i.note || '내용을 읽지 못했어요', enc: '암호가 걸려 있어서 이름으로만 찾아요', cloud: 'iCloud에만 있어서 이름으로만 찾아요. 한 번 열면 내려받아져요', spotlight: '맥의 Spotlight로 찾은 문서예요', outside: '서재 밖에 있는 파일이에요' }[i.state];
  pv.innerHTML = `
    <div class="pv-top">
      <button class="icon-btn pv-close" id="pvClose" title="닫기">✕</button>
      <div class="pv-cover${isImg ? ' wide' : ''}">${coverHTML(i, true)}</div>
      <div class="pv-name">${nameHTML(i.name)}</div>
      <div class="pv-kind" style="--c:${k.color}"><i></i>${esc(k.label)} · ${esc((i.ext || '').toUpperCase())}${i.pages ? ` · ${i.pages}쪽` : ''}</div>
    </div>
    <div class="pv-acts">
      <button data-a="open"><span>📖</span>열기</button>
      <button data-a="reveal"><span>🔍</span>${L.fm}에서 보기</button>
      <button data-a="quicklook" ${L.win ? 'hidden' : ''}><span>👁️</span>훑어보기</button>
    </div>
    ${stateTxt ? `<div class="pv-sec"><div class="state-line">${esc(stateTxt)}</div></div>` : ''}
    ${d.snips?.length ? `<div class="pv-sec"><div class="pv-cap">🔖 찾은 곳 ${d.snips.length}군데</div>${d.snips.map((s) => `<div class="pv-snip">${snipHTML(s)}</div>`).join('')}</div>` : ''}
    <div class="pv-sec"><div class="pv-cap">🗂️ 도서 카드</div>
      <div class="card-row">
        <span>위치</span><b><a id="pvDir" title="이 폴더에서 찾기">${esc(shortPath(i.dir))}</a></b>
        <span>크기</span><b>${fmtSize(i.size)}</b>
        <span>고친 날</span><b>${fmtDate(i.mod, true)}</b>
        <span>만든 날</span><b>${fmtDate(i.birth || i.mod, true)}</b>
        ${i.root ? `<span>서재</span><b>${esc(folderLabel(i.root))}</b>` : ''}
      </div></div>
    ${d.head ? `<div class="pv-sec"><div class="pv-cap">📄 본문 미리 보기 <span style="margin-left:auto;font-weight:600">${comma(d.textLen)}자</span></div><div class="pv-text">${esc(d.head)}</div></div>` : ''}
    ${d.ocrHead ? `<div class="pv-sec"><div class="pv-cap">🔤 OCR로 읽은 글자 <span style="margin-left:auto;font-weight:600">${comma(d.ocrLen)}자</span></div><div class="pv-text">${esc(d.ocrHead)}</div></div>` : ''}
    <div class="pv-more">
      <button class="btn ghost sm" data-b="basket">🧺 바구니에 담기</button>
      <button class="btn ghost sm" data-b="move">📂 옮기기</button>
      <button class="btn ghost sm" data-b="rename">✏️ 이름 바꾸기</button>
      ${i.ext !== 'pdf' ? '<button class="btn ghost sm" data-b="pdf">📄 PDF로</button>' : ''}
      <button class="btn ghost sm" data-b="trash" style="color:var(--err)">🗑️ 휴지통</button>
    </div>`;
  $('#pvClose').onclick = () => { S.cur = ''; pv.hidden = true; paintSel(); };
  $$('.pv-acts [data-a]', pv).forEach((b) => (b.onclick = () => openDoc(p, b.dataset.a)));
  $$('.pv-more [data-b]', pv).forEach((b) => (b.onclick = () => doAction(b.dataset.b, [p])));
  $('#pvDir').onclick = () => { setScope(i.dir); setView('search'); };
}
async function openDoc(p, how) {
  try { await api('/api/open', { path: p, how }); } catch (e) { toast('😢 ' + e.message); }
}

function moveCur(delta) {
  if (!S.items.length) return;
  let i = S.items.findIndex((d) => d.path === S.cur);
  i = Math.max(0, Math.min(S.items.length - 1, i < 0 ? 0 : i + delta));
  const d = S.items[i];
  S.sel.clear(); S.sel.add(d.path); S.anchor = i;
  showPreview(d.path);
  const el = $(`#resList [data-i="${i}"]`); el && el.scrollIntoView({ block: 'nearest' });
}

// ════════════════════════════════════════════════ 왼쪽: 내 서재 · 카드
function renderLib() {
  const l = S.lib; if (!l) return;
  $('#roots').innerHTML = l.roots.map((r) => `
    <button class="root${S.q.scope === r.path ? ' on' : ''}" data-p="${esc(r.path)}" title="${esc(r.path)}${r.err ? '\n' + esc(r.err) : ''}">
      <span class="ri" data-art="root"></span><span class="rn">${esc(folderLabel(r.path))}</span>${r.err ? '<span class="re">⚠️</span>' : `<span class="rc">${comma(r.count)}</span>`}</button>`).join('')
    || '<div class="note" style="padding:4px 8px">서재가 비어 있어요. ‘＋ 폴더’로 문서 폴더를 넣어 주세요.</div>';
  paintArt($('#roots'));
  $$('#roots .root').forEach((b) => (b.onclick = () => { setView('search'); setScope(S.q.scope === b.dataset.p ? '' : b.dataset.p); }));
  $('#libCount').textContent = comma(l.docs) + '권';
  const p = l.pending || {};
  const reading = (p.text || 0) + (p.retry || 0);
  $('#libSub').textContent = !l.ready ? '서재를 살피는 중…' : reading ? `${comma(reading)}권 읽는 중` : `${fmtSize(l.bytes)} · 모두 살펴봤어요`;
  const maxN = Math.max(1, ...Object.values(l.byKind || {}));
  $('#libSpines').innerHTML = S.kinds.map((k) => `<i title="${esc(k.label)} ${comma(l.byKind?.[k.id] || 0)}권" style="background:${k.color};height:${l.byKind?.[k.id] ? 22 + (l.byKind[k.id] / maxN) * 78 : 0}%"></i>`).join('');
  const notes = [];
  if (l.ocrSupport === 'no') notes.push('OCR: 이 Mac에서는 쓸 수 없어요');
  else if (!l.ocr) notes.push('OCR <b>꺼짐</b> · 설정에서 켤 수 있어요');
  else if (p.ocr) notes.push(`OCR 기다리는 문서 <b>${comma(p.ocr)}</b>개`);
  else if (p.ocrDone) notes.push(`OCR로 읽은 문서 <b>${comma(p.ocrDone)}</b>개`);
  if (p.cloud) notes.push(`iCloud에만 있는 문서 ${comma(p.cloud)}개`);
  if (l.lastScan) notes.push(`마지막 살핌 ${fmtWhen(l.lastScan)}`);
  $('#libNote').innerHTML = notes.join('<br>');
}
$('#addRootBtn').onclick = addRoot;
async function addRoot() {
  const p = await chooseFolder('서재에 넣을 폴더를 고르세요 (외장 디스크 폴더도 돼요)');
  if (!p) return;
  try { S.settings = await api('/api/library/roots', { action: 'add', path: p }); toast(`📚 ‘${folderLabel(p)}’ 폴더를 서재에 넣었어요`); poll(); }
  catch (e) { toast('⚠️ ' + e.message); }
}

// ════════════════════════════════════════════════ 진행 중인 작업
const JOB_EMOJI = { index: '🔎', read: '📖', ocr: '🔤', ops: '📦', auto: '🤖', cleanup: '🧹', tools: '🧰' };
const seenDone = new Set();
function renderJobs(jobs) {
  const box = $('#jobs');
  const quietKinds = ['index', 'read'];
  const show = jobs.filter((j) => j.state === 'running' || Date.now() - j.finished < (quietKinds.includes(j.kind) ? 2500 : 6000));
  box.hidden = !show.length;
  box.innerHTML = show.map((j) => {
    const ind = j.total <= 0 && j.state === 'running';
    const nums = j.total > 0 ? `<b>${comma(j.done)}</b> / ${comma(j.total)}${j.unit} · ${Math.floor(j.pct)}%` : j.done ? `${comma(j.done)}${j.unit}` : '';
    const right = j.state === 'running' ? (fmtETA(j.eta) || (j.total ? '남은 시간 계산 중…' : '')) : j.state === 'done' ? '✅ 끝났어요' : j.state === 'canceled' ? '멈췄어요' : '⚠️ 문제가 있었어요';
    const cancellable = j.state === 'running' && ['ocr', 'cleanup', 'ops', 'read', 'tools'].includes(j.kind);
    return `<div class="job ${j.state}"><span class="je">${JOB_EMOJI[j.kind] || '⏳'}</span>
      <div class="jt"><b>${esc(j.title)}</b><small>${esc(j.err || j.msg || '')}</small></div>
      <div class="jbar${ind ? ' ind' : ''}"><i style="width:${j.state === 'done' ? 100 : j.pct}%"></i></div>
      <div class="jn">${nums}${nums && right ? ' · ' : ''}${right}</div>
      ${cancellable ? `<button class="jx" data-id="${j.id}" title="멈추기">■</button>` : '<span></span>'}</div>`;
  }).join('');
  $$('.jx', box).forEach((b) => (b.onclick = () => api('/api/job/cancel', { id: b.dataset.id }).then(poll)));
  if (S.view === 'convert' && jobs.some((j) => j.kind === 'tools' && j.state === 'running')) { if (!renderJobs.cv) renderJobs.cv = setTimeout(() => { renderJobs.cv = null; if (S.view === 'convert') VIEWS.convert(); }, 1500); }
  // 위쪽 알림
  const run = jobs.filter((j) => j.state === 'running');
  const top = run.find((j) => j.kind === 'ops' || j.kind === 'auto') || run.find((j) => j.kind === 'cleanup') || run.find((j) => j.total > 0) || run[0];
  $('#topLive').hidden = !top;
  if (top) $('#topLiveText').textContent = `${top.title}${top.total ? ` · ${Math.floor(top.pct)}%` : ''}${top.eta ? ' · ' + fmtETA(top.eta) : ''}`;
  document.title = top ? `⏳ ${Math.floor(top.pct || 0)}% · LIBROAD` : 'LIBROAD';
  // 끝난 작업 알림
  for (const j of jobs) {
    if (j.state === 'running' || seenDone.has(j.id)) continue;
    seenDone.add(j.id);
    if (Date.now() - j.finished > 8000) continue;
    window.onJobDone && window.onJobDone(j);
    if (j.kind === 'tools') { toast(j.state === 'done' ? '🧰 ' + (j.msg || '변환 도구를 받았어요') : '⚠️ ' + (j.err || '변환 도구를 받지 못했어요'), 6000); if (S.view === 'convert') VIEWS.convert(); }
  }
}
$('#topLive').onclick = () => $('#jobs').scrollIntoView({ behavior: 'smooth' });

// ════════════════════════════════════════════════ 주기적 갱신
let fails = 0, ended = false, verTimer = null;
async function poll() {
  if (ended || (window.U && U.restarting)) return;
  try {
    const p = await api('/api/poll');
    fails = 0;
    S.jobs = p.jobs;
    const firstLib = !S.lib;
    S.lib = p.lib;
    renderLib();
    renderJobs(p.jobs);
    window.onUpdateState && onUpdateState(p.update);
    if (p.basket !== S.basket) { S.basket = p.basket; $('#basketCount').hidden = !p.basket; $('#basketCount').textContent = p.basket; }
    if (p.histSig !== S.histSig) { const had = S.histSig; S.histSig = p.histSig; if (had && S.view === 'history') VIEWS.history(); }
    // 색인이 바뀌면 결과를 조용히 새로 고쳐요
    if (p.lib.ver !== S.libVer) {
      S.libVer = p.lib.ver;
      clearTimeout(verTimer);
      verTimer = setTimeout(() => {
        if (S.view === 'search' && S.res && M.open) S.needRefresh = true;
        else if (S.view === 'search' && S.res) runSearch({ keepSel: true });
        else if (S.view === 'search' && !S.res) { renderChips(); renderBrowse(true); }
      }, firstLib ? 0 : 1500);
    }
  } catch {
    if (++fails >= 6) showEnded('LIBROAD 엔진이 종료되었어요');
  }
}
function showEnded(msg) {
  ended = true;
  document.body.innerHTML = `<div class="ended"><img src="/icon.png" alt=""><b>${msg}</b><span>이 창은 닫아도 돼요. 다시 쓰려면 LIBROAD 앱을 실행하세요.</span></div>`;
}

// ════════════════════════════════════════════════ 단축키
document.addEventListener('keydown', (e) => {
  const mod = e.metaKey || e.ctrlKey;
  const typing = /^(INPUT|TEXTAREA|SELECT)$/.test(document.activeElement?.tagName);
  if (M.open) { if (e.key === 'Escape') { e.preventDefault(); closeModal(); } return; }
  if (!$('#menu').hidden && e.key === 'Escape') { closeMenu(); return; }
  if (mod && e.key === 'f') { e.preventDefault(); setView('search'); qIn.focus(); qIn.select(); return; }
  if (mod && e.key === ',') { e.preventDefault(); window.openSettings && openSettings(); return; }
  if (mod && ['1', '2', '3', '4', '5', '6'].includes(e.key)) { e.preventDefault(); setView(['search', 'organize', 'cleanup', 'convert', 'basket', 'history'][+e.key - 1]); return; }
  if (S.view !== 'search' || typing) return;
  if (mod && e.key === 'a' && S.items.length) { e.preventDefault(); S.items.forEach((d) => S.sel.add(d.path)); paintSel(); return; }
  if (e.key === 'Escape') { if (S.sel.size) clearSel(); else if (S.cur) { S.cur = ''; $('#preview').hidden = true; } return; }
  if (e.key === 'ArrowDown' || e.key === 'ArrowUp') { e.preventDefault(); moveCur(e.key === 'ArrowDown' ? (S.viewMode === 'grid' ? 1 : 1) : -1); return; }
  if (e.key === 'ArrowRight' && S.viewMode === 'grid') { e.preventDefault(); moveCur(1); return; }
  if (e.key === 'ArrowLeft' && S.viewMode === 'grid') { e.preventDefault(); moveCur(-1); return; }
  if (e.key === ' ' && S.cur) { e.preventDefault(); openDoc(S.cur, 'quicklook'); return; }
  if (e.key === 'Enter' && S.cur) { e.preventDefault(); openDoc(S.cur, 'open'); return; }
  if (mod && e.key === 'Backspace' && S.sel.size) { e.preventDefault(); doAction('trash', [...S.sel]); return; }
  if (!mod && e.key.length === 1 && !e.altKey) { qIn.focus(); }
});
