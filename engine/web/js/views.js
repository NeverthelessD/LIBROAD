// LIBROAD — 분류하기 · 정리하기 · 바구니 · 작업 기록
// made by. Nevertheless_D
'use strict';

const VIEWS = {};
window.VIEWS = VIEWS;

function titleHTML(art, h, p) {
  return `<div class="view-title"><div class="va" data-art="${art}"></div><div><h2>${h}</h2><p>${p}</p></div></div>`;
}
function folderPickHTML(id, path, btn = '바꾸기…') {
  return `<div class="folder-pick"><span class="fi" data-art="folder"></span><div class="fx"><b>${esc(folderLabel(path))}</b><small>‎${esc(shortPath(path))}‎</small></div><button class="btn ghost sm" id="${id}">${btn}</button></div>`;
}
function docRowHTML(d, i, checked, extra = '') {
  return `<div class="row${checked ? ' on' : ''}${S.cur === d.path ? ' cur' : ''}" data-i="${i}">
    <label class="ck"><input type="checkbox" ${checked ? 'checked' : ''}><span></span></label>
    <div class="mc">${coverSVG(d.kind, d.ext, d.name)}</div>
    <div class="rt"><div class="rn" title="${esc(d.name)}">${esc(d.name)}</div><div class="rp">📁 ${esc(shortPath(d.dir))}</div>${extra}</div>
    <div class="rm"><span>${fmtDate(d.mod)}</span><span>${fmtSize(d.size)}</span></div></div>`;
}

// ════════════════════════════════════════════════ 분류하기
const ORG = { source: '', mode: 'kind', dest: '', recursive: false };
VIEWS.organize = () => {
  const st = S.settings, au = st.auto;
  ORG.source = ORG.source || st.downloads;
  ORG.dest = ORG.dest || st.defaultDest;
  const el = $('#view-organize');
  const every = [[10, '10분마다'], [30, '30분마다'], [60, '1시간마다'], [180, '3시간마다'], [1440, '하루에 한 번']];
  el.innerHTML = `
    ${titleHTML('organize', '분류하기', '흩어진 문서를 종류 · 날짜 · 나만의 규칙대로 폴더에 나눠 담아요. 실행 전에 꼭 미리 보여 드려요.')}
    <div class="grid2">
      <div class="card">
        <div class="card-title">📂 폴더 한 번에 정리</div>
        <div class="card-sub">고른 폴더 안의 문서를 정리해요. (검색 결과나 바구니에서 고른 문서도 ‘분류’로 정리할 수 있어요)</div>
        <div class="field"><span class="fl">정리할 폴더</span>${folderPickHTML('orgSrc', ORG.source)}
          <label class="switch-row" style="margin-top:6px"><input type="checkbox" id="orgRec" ${ORG.recursive ? 'checked' : ''}><span class="switch"></span>안쪽 폴더까지 모두</label></div>
        <div class="field"><span class="fl">나누는 방법</span>${modesHTML(ORG.mode)}</div>
        <div class="field"><span class="fl">정리한 문서를 둘 곳</span>${folderPickHTML('orgDest', ORG.dest)}</div>
        <button class="btn primary" id="orgGo" style="width:100%">🔍 미리 보기</button>
      </div>
      <div class="card">
        <div class="card-title">🤖 자동 정리 <span class="sp"></span><label class="switch-row"><input type="checkbox" id="auOn" ${au.enabled ? 'checked' : ''}><span class="switch"></span></label></div>
        <div class="card-sub">켜 두면 앱이 켜져 있는 동안 정해진 간격마다 이 폴더의 문서를 알아서 정리해요. 막 내려받은 파일(${au.minAge}분 이내)은 건드리지 않아요.</div>
        <div class="field"><span class="fl">지켜볼 폴더</span>${folderPickHTML('auSrc', au.source)}</div>
        <div class="field"><span class="fl">정리한 문서를 둘 곳</span>${folderPickHTML('auDest', au.dest)}</div>
        <div class="field" style="flex-direction:row;gap:10px;align-items:center;flex-wrap:wrap">
          <select class="input" id="auEvery">${every.map(([v, t]) => `<option value="${v}" ${au.every === v ? 'selected' : ''}>${t}</option>`).join('')}</select>
          <select class="input" id="auMode">${MODES.map((m) => `<option value="${m.v}" ${au.mode === m.v ? 'selected' : ''}>${m.i} ${m.t}</option>`).join('')}</select>
          ${au.mode === 'rules' ? `<label class="switch-row" style="font-size:12px"><input type="checkbox" id="auRest" ${au.rest === 'kind' ? 'checked' : ''}><span class="switch"></span>규칙 밖 문서는 종류별로</label>` : ''}
        </div>
        <div class="kv"><span>마지막 정리</span><b>${au.lastRun ? fmtWhen(au.lastRun * 1000) + (au.lastCount ? ` · ${au.lastCount}개` : ' · 정리할 것 없음') : '아직 안 했어요'}</b></div>
        <div style="display:flex;gap:8px;margin-top:8px"><button class="btn ghost" id="auPreview" style="flex:1">🔍 미리 보기</button><button class="btn brass" id="auNow" style="flex:1">⚡ 지금 한 번 정리</button></div>
      </div>
    </div>
    <div class="card" style="margin-top:16px">
      <div class="card-title">📏 나만의 규칙 <span class="sp"></span><button class="link" id="ruleReset">기본 규칙으로</button><button class="btn ghost sm" id="ruleAdd">＋ 규칙 추가</button></div>
      <div class="card-sub">‘나만의 규칙’으로 나눌 때 위에서부터 차례로 맞는 규칙 하나를 써요. 폴더 이름에 <b>{종류}</b> <b>{연도}</b> <b>{월}</b>을 쓰면 문서에 맞게 바뀌어요.</div>
      <div class="rules" id="ruleList"></div>
    </div>`;
  paintArt(el);
  $$('.mode', el).forEach((b) => (b.onclick = () => { ORG.mode = b.dataset.m; VIEWS.organize(); }));
  $('#orgSrc').onclick = async () => { const p = await chooseFolder('정리할 폴더를 고르세요', ORG.source); if (p) { ORG.source = p; VIEWS.organize(); } };
  $('#orgDest').onclick = async () => { const p = await chooseFolder('정리한 문서를 둘 폴더를 고르세요', ORG.dest); if (p) { ORG.dest = p; VIEWS.organize(); } };
  $('#orgRec').onchange = (e) => { ORG.recursive = e.target.checked; };
  $('#orgGo').onclick = () => organizeDialog([], { source: ORG.source, mode: ORG.mode, dest: ORG.dest, recursive: ORG.recursive });
  const setAuto = async (patch, msg) => {
    try { const r = await api('/api/auto', patch); S.settings.auto = r.auto; if (msg) toast(msg); VIEWS.organize(); } catch (e) { toast('⚠️ ' + e.message); }
  };
  $('#auOn').onchange = (e) => setAuto({ enabled: e.target.checked }, e.target.checked ? `🤖 자동 정리를 켰어요 · ${every.find(([v]) => v === au.every)?.[1] || ''}` : '자동 정리를 껐어요');
  $('#auSrc').onclick = async () => { const p = await chooseFolder('지켜볼 폴더를 고르세요', au.source); if (p) setAuto({ source: p }); };
  $('#auDest').onclick = async () => { const p = await chooseFolder('정리한 문서를 둘 폴더를 고르세요', au.dest); if (p) setAuto({ dest: p }); };
  $('#auEvery').onchange = (e) => setAuto({ every: +e.target.value });
  $('#auMode').onchange = (e) => setAuto({ mode: e.target.value });
  const ar = $('#auRest'); if (ar) ar.onchange = (e) => setAuto({ rest: e.target.checked ? 'kind' : 'stay' });
  $('#auPreview').onclick = () => organizeDialog([], { source: au.source, mode: au.mode, dest: au.dest });
  $('#auNow').onclick = async () => {
    try { await api('/api/auto/run', {}); toast('⚡ 정리를 시작했어요'); poll(); } catch (e) { toast('🙂 ' + e.message); }
  };
  renderRules();
  $('#ruleAdd').onclick = () => editRule(null);
  $('#ruleReset').onclick = async () => {
    if (!(await confirmBox('기본 규칙으로', '지금 규칙을 지우고 기본 규칙(영수증 · 계약서 · 증명서 · 발표 자료)으로 되돌릴까요?', '되돌리기'))) return;
    const r = await api('/api/rules', { reset: true }); S.settings.rules = r.rules; renderRules();
  };
};

function ruleDesc(r) {
  const parts = [];
  if (r.words) parts.push(`이름에 <b>${esc(r.words.split(',').map((x) => x.trim()).filter(Boolean).slice(0, 4).join(', '))}</b>${r.words.split(',').length > 4 ? ' …' : ''}`);
  if (r.kinds?.length) parts.push(`종류: <b>${r.kinds.map((k) => esc(kindOf(k).label)).join(', ')}</b>`);
  if (r.days) parts.push(`<b>${r.days}일</b> 넘은 문서`);
  return parts.join(' · ') || '<span style="color:var(--err)">조건이 없어요</span>';
}
function renderRules() {
  const rules = S.settings.rules || [];
  const list = $('#ruleList');
  list.innerHTML = rules.map((r, i) => `
    <div class="rule${r.on ? '' : ' off'}" data-i="${i}">
      <label class="switch-row"><input type="checkbox" class="ron" ${r.on ? 'checked' : ''}><span class="switch"></span></label>
      <div class="rule-t"><b>${esc(r.name)}</b><small>${ruleDesc(r)} <span class="arrow">→ 📁 ${esc(r.target || r.name)}</span></small></div>
      <div style="display:flex;gap:4px"><button class="btn ghost sm" data-a="up" title="위로" ${i === 0 ? 'disabled' : ''}>↑</button><button class="btn ghost sm" data-a="edit">고치기</button><button class="btn ghost sm" data-a="del" title="지우기">✕</button></div>
    </div>`).join('') || '<div class="note">규칙이 없어요. ‘＋ 규칙 추가’를 눌러 만들어 보세요.</div>';
  $$('.rule', list).forEach((row) => {
    const i = +row.dataset.i;
    row.querySelector('.ron').onchange = (e) => { rules[i].on = e.target.checked; saveRules(rules); };
    row.querySelector('[data-a="edit"]').onclick = () => editRule(i);
    row.querySelector('[data-a="del"]').onclick = () => { rules.splice(i, 1); saveRules(rules); };
    row.querySelector('[data-a="up"]').onclick = () => { [rules[i - 1], rules[i]] = [rules[i], rules[i - 1]]; saveRules(rules); };
  });
}
async function saveRules(rules) {
  try { const r = await api('/api/rules', { rules }); S.settings.rules = r.rules; renderRules(); } catch (e) { toast('⚠️ ' + e.message); }
}
function editRule(i) {
  const rules = S.settings.rules || [];
  const r = i === null ? { name: '', on: true, words: '', kinds: [], days: 0, target: '' } : { ...rules[i], kinds: [...(rules[i].kinds || [])] };
  const body = document.createElement('div');
  const paint = () => {
    body.innerHTML = `
      <div class="field"><label>규칙 이름</label><input class="input" id="reName" value="${esc(r.name)}" placeholder="예: 학교 서류"></div>
      <div class="field"><label>파일 이름에 이 말이 있으면 (쉼표로 여러 개)</label><textarea class="input" id="reWords" rows="2" placeholder="예: 가정통신문, 알림장, 학교">${esc(r.words)}</textarea></div>
      <div class="field"><span class="fl">이 종류만 (안 고르면 모든 종류)</span><div class="kpick">${S.kinds.map((k) => `<button class="chip${r.kinds.includes(k.id) ? ' on' : ''}" data-k="${k.id}" style="--c:${k.color}"><span class="sw"></span>${esc(k.label)}</button>`).join('')}</div></div>
      <div class="field"><label>이만큼 지난 문서만</label><select class="input" id="reDays">${[[0, '상관없음'], [30, '30일 넘은'], [90, '3달 넘은'], [180, '반년 넘은'], [365, '1년 넘은']].map(([v, t]) => `<option value="${v}" ${r.days === v ? 'selected' : ''}>${t}</option>`).join('')}</select></div>
      <div class="field"><label>옮길 폴더 <span class="note">(이름만 쓰면 ‘정리한 문서를 둘 곳’ 안에 만들어요)</span></label>
        <div style="display:flex;gap:6px"><input class="input" id="reTarget" style="flex:1" value="${esc(r.target)}" placeholder="예: 학교/{연도}"><button class="btn ghost sm" id="rePick">폴더 고르기…</button></div></div>`;
    $$('.kpick .chip', body).forEach((b) => (b.onclick = () => { const k = b.dataset.k; r.kinds = r.kinds.includes(k) ? r.kinds.filter((x) => x !== k) : [...r.kinds, k]; sync(); paint(); }));
    $('#rePick', body).onclick = async () => { sync(); const p = await chooseFolder('이 규칙의 문서를 옮길 폴더를 고르세요'); if (p) { r.target = p; paint(); } };
  };
  const sync = () => {
    r.name = $('#reName', body).value.trim(); r.words = $('#reWords', body).value.trim();
    r.days = +$('#reDays', body).value; r.target = $('#reTarget', body).value.trim();
  };
  paint();
  openModal({ title: i === null ? '📏 새 규칙' : '📏 규칙 고치기', body, actions: [{ label: '취소', onClick: closeModal }, { label: '저장', primary: true, onClick: () => {
    sync();
    if (!r.words && !r.kinds.length) return toast('이름에 들어갈 말이나 종류 중 하나는 정해 주세요');
    if (!r.name) r.name = r.words.split(',')[0] || kindOf(r.kinds[0]).label;
    if (!r.target) r.target = r.name;
    if (i === null) rules.push(r); else rules[i] = r;
    closeModal(); saveRules(rules);
  } }] });
}

// ════════════════════════════════════════════════ 정리하기
const CL = { kind: 'dupes', scope: '', minMB: 50, years: 2, job: '', result: null, sel: new Set(), running: false };
const CL_TOOLS = [
  { k: 'dupes', i: '👯', t: '똑같은 문서', d: '내용이 완전히 같은 파일' },
  { k: 'versions', i: '🗂️', t: '버전 정리', d: '최종 · 최종2 · 수정본…' },
  { k: 'big', i: '🐘', t: '큰 파일', d: '자리를 많이 차지하는' },
  { k: 'old', i: '🕸️', t: '오래된 문서', d: '오래 손대지 않은' },
  { k: 'empty', i: '📭', t: '빈 폴더', d: '아무것도 없는 폴더' },
];
VIEWS.cleanup = () => {
  const el = $('#view-cleanup');
  const roots = S.lib?.roots || [];
  el.innerHTML = `
    ${titleHTML('cleanup', '정리하기', '찾기만 하고, 지우거나 옮기는 건 고른 뒤에 해요. 지울 때는 언제나 휴지통으로 보내요.')}
    <div class="tools">${CL_TOOLS.map((x) => `<button class="tool${CL.kind === x.k ? ' on' : ''}" data-k="${x.k}"><span style="font-size:30px">${x.i}</span><b>${x.t}</b><small>${x.d}</small></button>`).join('')}</div>
    <div class="cl-opts">
      <label class="sel"><span>찾을 곳</span><select id="clScope"><option value="">📚 서재 전체</option>${roots.map((r) => `<option value="${esc(r.path)}" ${CL.scope === r.path ? 'selected' : ''}>📁 ${esc(folderLabel(r.path))}</option>`).join('')}</select></label>
      ${CL.kind === 'big' ? `<label class="sel"><span>크기</span><select id="clMB">${[10, 50, 100, 500].map((v) => `<option value="${v}" ${CL.minMB === v ? 'selected' : ''}>${v}MB 넘는</option>`).join('')}</select></label>` : ''}
      ${CL.kind === 'old' ? `<label class="sel"><span>기간</span><select id="clYears">${[1, 2, 3, 5].map((v) => `<option value="${v}" ${CL.years === v ? 'selected' : ''}>${v}년 넘게 안 고친</option>`).join('')}</select></label>` : ''}
      <span class="sp"></span>
      <button class="btn primary" id="clGo">${CL.running ? '⏳ 찾는 중…' : '🔍 찾기'}</button>
    </div>
    <div id="clRes"></div>`;
  paintArt(el);
  $$('.tool', el).forEach((b) => (b.onclick = () => { if (CL.kind !== b.dataset.k) { CL.kind = b.dataset.k; CL.result = null; CL.sel.clear(); } VIEWS.cleanup(); }));
  $('#clScope').onchange = (e) => { CL.scope = e.target.value; };
  const mb = $('#clMB'); if (mb) mb.onchange = (e) => { CL.minMB = +e.target.value; };
  const yr = $('#clYears'); if (yr) yr.onchange = (e) => { CL.years = +e.target.value; };
  $('#clGo').onclick = startCleanup;
  $('#clGo').disabled = CL.running;
  renderCleanup();
};
async function startCleanup() {
  CL.result = null; CL.sel.clear(); CL.running = true;
  try {
    const r = await api('/api/cleanup/start', { kind: CL.kind, scope: CL.scope, minMB: CL.minMB, years: CL.years });
    CL.job = r.job; CL.runKind = CL.kind;
  } catch (e) { CL.running = false; toast('⚠️ ' + e.message); }
  VIEWS.cleanup(); poll();
}
window.refreshCleanup = () => { if (CL.result && !CL.running) startCleanup(); };
window.onCleanupDone = async (j) => {
  if (j.id !== CL.job) return;
  CL.running = false;
  const r = await api('/api/job/result', { id: j.id }).catch(() => null);
  CL.result = r?.result || { error: j.err || '찾지 못했어요' };
  CL.sel.clear();
  // 미리 골라 두기: 똑같은 문서 → 첫 번째(원본)만 남기고, 버전 → 최신만 남기고, 빈 폴더 → 모두
  if (CL.runKind === 'dupes') for (const g of CL.result.groups || []) g.files.slice(1).forEach((f) => CL.sel.add(f.path));
  if (CL.runKind === 'versions') for (const g of CL.result.groups || []) g.files.slice(1).forEach((f) => CL.sel.add(f.path));
  if (CL.runKind === 'empty') for (const d of CL.result.dirs || []) CL.sel.add(d.path);
  if (S.view === 'cleanup') VIEWS.cleanup();
};
function renderCleanup() {
  const box = $('#clRes');
  if (CL.running) {
    box.innerHTML = `<div class="empty"><div data-art="cleanup"></div><p>찾고 있어요…</p><small>아래 진행 막대에서 남은 시간을 볼 수 있어요</small></div>`;
    paintArt(box); return;
  }
  const r = CL.result;
  if (!r) { box.innerHTML = `<div class="empty"><div data-art="cleanup"></div><p>무엇을 정리할지 고르고 ‘찾기’를 눌러 주세요</p><small>찾기만 해서는 아무것도 바뀌지 않아요</small></div>`; paintArt(box); return; }
  if (r.error) { box.innerHTML = `<div class="upd-err">${esc(r.error)}</div>`; return; }
  const kind = CL.runKind;
  let html = '';
  const fileRow = (f, extra = '', tag = '') => `<div class="grow" data-p="${esc(f.path)}"><label class="ck"><input type="checkbox" ${CL.sel.has(f.path) ? 'checked' : ''}><span></span></label>
    <div class="gn"><b>${esc(f.name || f.path)} ${tag}</b><small>📁 ${esc(shortPath(f.dir || f.parent || ''))}</small></div><span class="gm">${extra}</span><span class="gm">${f.size !== undefined ? fmtSize(f.size) : ''}</span></div>`;
  if (kind === 'dupes') {
    const groups = r.groups || [];
    html = groups.length ? `<div class="note" style="margin-bottom:8px">똑같은 문서 <b>${groups.length}</b>묶음 · 지우면 <b>${fmtSize(r.wasted)}</b>를 비울 수 있어요. 묶음마다 맨 위(먼저 만든 것)를 남기도록 골라 뒀어요.</div>` +
      groups.map((g) => `<div class="group"><div class="group-h">👯 똑같은 문서 ${g.files.length}개 <small>· ${fmtSize(g.size)}씩</small></div>${g.files.map((f, i) => fileRow(f, fmtDate(f.mod), i === 0 ? '<span class="tag">원본으로 보여요</span>' : '')).join('')}</div>`).join('')
      : emptyResult('똑같은 문서가 없어요 👍');
  } else if (kind === 'versions') {
    const groups = r.groups || [];
    html = groups.length ? `<div class="note" style="margin-bottom:8px">이름만 조금 다른 버전 묶음 <b>${groups.length}</b>개. 가장 최근에 고친 것만 남기도록 골라 뒀어요. 지우기 대신 ‘이전 버전’ 폴더로 모아 둘 수도 있어요.</div>` +
      groups.map((g) => `<div class="group"><div class="group-h">🗂️ ‘${esc(g.stem)}’ <small>· ${esc(kindOf(g.kind).label)} · ${g.files.length}개</small></div>${g.files.map((f, i) => fileRow(f, fmtDate(f.mod), i === 0 ? '<span class="tag">최신</span>' : '<span class="tag old">이전</span>')).join('')}</div>`).join('')
      : emptyResult('여러 버전으로 나뉜 문서가 없어요 👍');
  } else if (kind === 'big' || kind === 'old') {
    const files = r.files || [];
    html = files.length ? `<div class="note" style="margin-bottom:8px">${comma(r.count)}개 · 모두 ${fmtSize(r.bytes)}${r.count > files.length ? ` (앞의 ${files.length}개만 보여요)` : ''}</div><div class="group">${files.map((f) => fileRow(f, kind === 'old' ? fmtDate(f.mod, false) : fmtDate(f.mod))).join('')}</div>`
      : emptyResult(kind === 'big' ? '큰 파일이 없어요 👍' : '오래된 문서가 없어요 👍');
  } else if (kind === 'empty') {
    const dirs = r.dirs || [];
    html = dirs.length ? `<div class="note" style="margin-bottom:8px">빈 폴더 <b>${dirs.length}</b>개 (안쪽 빈 폴더는 함께 정리돼요)</div><div class="group">${dirs.map((d) => fileRow({ ...d, dir: d.parent }, fmtDate(d.mod))).join('')}</div>`
      : emptyResult('빈 폴더가 없어요 👍');
  }
  const hasItems = (r.groups?.length || r.files?.length || r.dirs?.length);
  box.innerHTML = html + (hasItems ? `<div class="actbar" id="clBar"></div>` : '');
  paintArt(box);
  $$('.grow', box).forEach((row) => {
    const p = row.dataset.p;
    row.querySelector('input').onchange = (e) => { e.target.checked ? CL.sel.add(p) : CL.sel.delete(p); paintClBar(); };
    row.ondblclick = () => openDoc(p, kind === 'empty' ? 'open' : 'reveal');
  });
  if (hasItems) paintClBar();
}
function emptyResult(msg) { return `<div class="empty"><div data-art="empty"></div><p>${msg}</p></div>`; }
function paintClBar() {
  const bar = $('#clBar'); if (!bar) return;
  const n = CL.sel.size;
  const all = (CL.result.groups || []).flatMap((g) => g.files).concat(CL.result.files || []);
  const bytes = all.filter((f) => CL.sel.has(f.path)).reduce((s, f) => s + (f.size || 0), 0);
  bar.innerHTML = `<b style="font-size:13px">${comma(n)}개 고름</b>${bytes ? `<span class="note">${fmtSize(bytes)}</span>` : ''}<span class="sp"></span>
    <button class="link" id="clNone">선택 해제</button>
    ${CL.runKind === 'versions' ? '<button class="btn ghost sm" id="clOld">📦 ‘이전 버전’ 폴더로</button>' : ''}
    ${CL.runKind !== 'empty' ? '<button class="btn ghost sm" id="clBasket">🧺 바구니에</button><button class="btn ghost sm" id="clMove">📂 옮기기</button>' : ''}
    <button class="btn danger sm" id="clTrash" ${n ? '' : 'disabled'}>🗑️ 휴지통으로</button>`;
  $('#clNone').onclick = () => { CL.sel.clear(); renderCleanup(); };
  $('#clTrash').onclick = () => doAction('trash', [...CL.sel]);
  const mv = $('#clMove'); if (mv) mv.onclick = () => doAction('move', [...CL.sel]);
  const bk = $('#clBasket'); if (bk) bk.onclick = () => doAction('basket', [...CL.sel]);
  const old = $('#clOld'); if (old) old.onclick = async () => {
    const pairs = [...CL.sel].map((p) => ({ src: p, dir: dirName(p) + '/이전 버전' }));
    if (!pairs.length) return;
    let plan; try { plan = (await api('/api/ops/plan', { op: 'move', pairs })).plan; } catch (e) { return toast('⚠️ ' + e.message); }
    const body = document.createElement('div'); const pv = planView(plan); body.append(pv);
    const acts = () => setModalActions([{ label: '취소', onClick: closeModal }, { label: `${comma(pv.count())}개 옮기기`, primary: true, disabled: !pv.count(), onClick: () => runPlan(plan, pv.skips()) }]);
    pv.onchange = acts;
    openModal({ title: '📦 이전 버전 모아 두기', sub: '같은 폴더 안에 ‘이전 버전’ 폴더를 만들어 옮겨요', body, wide: true }); acts();
  };
}

// ════════════════════════════════════════════════ 바구니 (일괄 처리)
const BK = { items: [], sel: new Set(), missing: 0 };
window.basketSelected = () => (BK.sel.size ? [...BK.sel] : BK.items.map((d) => d.path));
VIEWS.basket = async () => {
  const el = $('#view-basket');
  const r = await api('/api/basket', {}).catch(() => null);
  if (r) { BK.items = r.items; BK.missing = r.missing; }
  const have = new Set(BK.items.map((d) => d.path));
  for (const p of [...BK.sel]) if (!have.has(p)) BK.sel.delete(p);
  const n = BK.sel.size, all = BK.items.length;
  const target = n ? `고른 ${n}개` : `모두 ${all}개`;
  el.innerHTML = `
    ${titleHTML('basket', '바구니', '여러 번 찾아서 고른 문서를 모아 두었다가, 한꺼번에 옮기고 · 이름 바꾸고 · 분류해요.')}
    <div style="display:flex;gap:8px;align-items:center;margin-bottom:10px">
      <label class="ck" title="모두 선택"><input type="checkbox" id="bkAll" ${n && n === all ? 'checked' : ''}><span></span></label>
      <span class="note"><b style="color:var(--ink)">${comma(all)}</b>개 담김${BK.missing ? ` · 사라진 파일 ${BK.missing}개는 뺐어요` : ''}</span>
      <span class="sp"></span>
      <button class="btn ghost sm" id="bkPick">📄 파일 고르기…</button>
      <button class="btn ghost sm" id="bkClear" ${all ? '' : 'disabled'}>비우기</button>
    </div>
    <div class="res-list" id="bkList">${BK.items.map((d, i) => docRowHTML(d, i, BK.sel.has(d.path))).join('')}</div>
    ${all ? '' : `<div class="empty"><div data-art="basket"></div><p>바구니가 비어 있어요</p><small>찾기 화면에서 문서를 고르고 ‘🧺 바구니에 담기’를 누르거나,<br>‘파일 고르기’로 하나 또는 여러 파일을 직접 고를 수 있어요.</small></div>`}
    ${all ? `<div class="actbar"><b style="font-size:13px">${target}</b><span class="sp"></span>
      <button class="btn ghost sm" data-b="move">📂 옮기기</button><button class="btn ghost sm" data-b="copy">📑 복사</button>
      <button class="btn ghost sm" data-b="rename">✏️ 이름 바꾸기</button><button class="btn ghost sm" data-b="organize">🗂️ 분류</button>
      <button class="btn ghost sm" data-b="out">바구니에서 빼기</button><button class="btn danger sm" data-b="trash">🗑️ 휴지통</button></div>` : ''}`;
  paintArt(el);
  $('#bkAll').onchange = (e) => { BK.sel = e.target.checked ? new Set(BK.items.map((d) => d.path)) : new Set(); VIEWS.basket(); };
  $('#bkPick').onclick = async () => {
    const j = await api('/api/choose-files', { start: S.settings.documents }).catch((e) => { toast('⚠️ ' + e.message); return null; });
    if (j?.paths?.length) { await api('/api/basket', { action: 'add', paths: j.paths }); toast(`🧺 ${j.paths.length}개를 담았어요`); VIEWS.basket(); poll(); }
  };
  $('#bkClear').onclick = async () => { if (await confirmBox('바구니 비우기', '바구니만 비워요. 파일은 그대로 있어요.', '비우기')) { await api('/api/basket', { action: 'clear' }); BK.sel.clear(); VIEWS.basket(); poll(); } };
  $$('#bkList .row').forEach((row) => {
    const d = BK.items[+row.dataset.i];
    row.onclick = (e) => {
      if (e.target.closest('.ck')) { if (e.target.tagName === 'INPUT') { e.target.checked ? BK.sel.add(d.path) : BK.sel.delete(d.path); VIEWS.basket(); } return; }
      if (e.metaKey || e.ctrlKey) { BK.sel.has(d.path) ? BK.sel.delete(d.path) : BK.sel.add(d.path); VIEWS.basket(); }
      showPreview(d.path);
    };
    row.ondblclick = () => openDoc(d.path, 'open');
  });
  $$('[data-b]', el).forEach((b) => (b.onclick = async () => {
    const paths = window.basketSelected();
    if (b.dataset.b === 'out') { await api('/api/basket', { action: 'remove', paths }); BK.sel.clear(); VIEWS.basket(); poll(); return; }
    doAction(b.dataset.b, paths);
  }));
};

// ════════════════════════════════════════════════ 작업 기록
const OP_ICON = { move: '📂', copy: '📑', trash: '🗑️', rename: '✏️' };
VIEWS.history = async () => {
  const el = $('#view-history');
  const r = await api('/api/history').catch(() => null);
  const items = r?.items || [];
  el.innerHTML = `
    ${titleHTML('history', '작업 기록', 'LIBROAD가 옮기고 · 바꾸고 · 휴지통으로 보낸 일을 모두 적어 둬요. 잘못했다면 되돌리세요.')}
    <div class="hist">${items.map((h) => {
      const isRename = h.items.length && h.items.every((x) => x.act === 'move' && dirName(x.from) === dirName(x.to));
      const icon = h.auto ? '🤖' : isRename ? '✏️' : OP_ICON[h.op] || '📦';
      return `<div class="hitem${h.undone ? ' undone' : ''}">
        <div class="hi">${icon}</div>
        <div class="ht"><b>${esc(h.title)}</b>${h.auto ? ' <span class="tag">자동</span>' : ''}
          <small>${fmtWhen(h.time * 1000)} · ${comma(h.count)}개${h.failed ? ` · <span style="color:var(--err)">${h.failed}개 실패</span>` : ''}${h.undone ? ` · ↩︎ 되돌림 (${fmtWhen(h.undoneAt * 1000)})` : ''}${h.undoNote ? ` · ${esc(h.undoNote)}` : ''}</small>
          <details><summary>자세히 보기</summary>${h.items.map((x) => `<div class="hl"><span>${esc(baseName(x.from))}</span><span class="ar">→</span><span>${x.act === 'trash' ? '🗑️ 휴지통' : esc(shortPath(x.act === 'move' && dirName(x.from) === dirName(x.to) ? baseName(x.to) : dirName(x.to)))}</span></div>`).join('')}${h.count > h.items.length ? `<div class="hl">… 그리고 ${h.count - h.items.length}개 더</div>` : ''}${(h.errors || []).map((e) => `<div class="hl" style="color:var(--err)">⚠️ ${esc(e)}</div>`).join('')}</details>
        </div>
        <div>${h.undone ? '' : `<button class="btn ghost sm" data-u="${h.id}">↩︎ 되돌리기</button>`}</div>
      </div>`;
    }).join('') || `<div class="empty"><div data-art="history"></div><p>아직 기록이 없어요</p><small>옮기기 · 이름 바꾸기 · 분류 · 휴지통을 쓰면 여기에 남아요</small></div>`}</div>`;
  paintArt(el);
  $$('[data-u]', el).forEach((b) => (b.onclick = async () => {
    const h = items.find((x) => x.id === b.dataset.u);
    if (!(await confirmBox('↩︎ 되돌리기', `‘${esc(h.title)}’에서 한 일 ${h.count}개를 원래대로 되돌릴까요?<br><span class="note">그 뒤에 다시 옮긴 파일이나, 휴지통을 비워서 사라진 파일은 되돌릴 수 없어요.</span>`, '되돌리기'))) return;
    undoHistory(h.id);
  }));
};
VIEWS.search = () => {};
