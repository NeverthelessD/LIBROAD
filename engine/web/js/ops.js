// LIBROAD — 옮기기 · 복사 · 이름 바꾸기 · 분류 · 휴지통 (미리 보기 → 실행 → 되돌리기)
// made by. Nevertheless_D
'use strict';

const OP_WORD = { move: '옮기기', copy: '복사', rename: '이름 바꾸기', trash: '휴지통으로', organize: '분류' };

async function doAction(act, paths) {
  paths = [...new Set(paths)].filter(Boolean);
  if (!paths.length) return toast('먼저 문서를 골라 주세요');
  switch (act) {
    case 'basket': {
      const r = await api('/api/basket', { action: 'add', paths }).catch((e) => { toast('⚠️ ' + e.message); return null; });
      if (r) { toast(`🧺 바구니에 ${paths.length}개를 담았어요 (모두 ${r.items.length}개)`); poll(); }
      return;
    }
    case 'move': case 'copy': return destDialog(act, paths);
    case 'rename': return renameDialog(paths);
    case 'organize': return organizeDialog(paths);
    case 'trash': return trashDialog(paths);
  }
}

// ════════════════════════════════════════════════ 계획 미리 보기
function planView(plan, { group = false } = {}) {
  const wrap = document.createElement('div');
  const excluded = new Set();
  const sum = () => {
    const n = plan.items.filter((it, i) => !it.skip && !excluded.has(i)).length;
    return n;
  };
  const paint = () => {
    const n = sum();
    const items = plan.items.map((it, i) => ({ it, i }));
    let rows = '';
    let lastG = null;
    const ordered = group ? [...items].sort((a, b) => (a.it.group || '').localeCompare(b.it.group || '', 'ko') || (a.it.skip - b.it.skip)) : items;
    for (const { it, i } of ordered) {
      if (group && (it.group || '') !== lastG && !it.skip) {
        lastG = it.group || '';
        const cnt = plan.items.filter((x) => (x.group || '') === lastG && !x.skip).length;
        rows += `<div class="plan-grp">📁 ${esc(lastG || '그대로')} <span style="color:var(--muted);font-weight:700">· ${cnt}개</span></div>`;
      }
      const off = it.skip || excluded.has(i);
      const to = plan.op === 'trash' ? '<span class="to">🗑️ 휴지통</span>'
        : plan.op === 'rename' ? `<span class="to">→ ${esc(baseName(it.dst || '') || it.newName || '')}</span>`
        : it.dst ? `<span class="to">→ ${esc(shortPath(dirName(it.dst)))}</span>${baseName(it.dst) !== it.name ? ` <span class="pn">(${esc(baseName(it.dst))})</span>` : ''}` : '';
      rows += `<div class="prow${off ? ' skip' : ''}"><label class="ck"><input type="checkbox" data-i="${i}" ${off ? '' : 'checked'} ${it.skip ? 'disabled' : ''}><span></span></label>
        <div class="pf"><b>${esc(it.name)}</b><small>${it.skip ? `<span class="pn">${esc(it.note || '건너뛰어요')}</span>` : `${to}${it.note && plan.op !== 'move' && plan.op !== 'copy' ? ` · <span class="pn">${esc(it.note)}</span>` : ''}`}</small></div></div>`;
    }
    wrap.innerHTML = `
      <div class="plan-sum">
        <span class="g">✔ ${comma(n)}개 ${OP_WORD[plan.op] || ''}</span>
        ${plan.bytes ? `<span>${fmtSize(plan.bytes)}</span>` : ''}
        ${plan.renamed ? `<span class="b">같은 이름 ${plan.renamed}개는 ‘(2)’를 붙여요</span>` : ''}
        ${plan.newDirs?.length ? `<span class="b">새 폴더 ${plan.newDirs.length}개를 만들어요</span>` : ''}
        ${plan.skipped ? `<span>건너뜀 ${plan.skipped}개</span>` : ''}
      </div>
      <div class="plan-list">${rows || '<div class="fp-empty">할 일이 없어요</div>'}</div>
      <div class="note" style="margin-top:8px">${plan.op === 'trash' ? '🗑️ 바로 지우지 않고 휴지통으로 옮겨요. ‘작업 기록’에서 되돌릴 수 있어요.' : '↩️ 같은 이름의 파일을 덮어쓰지 않아요. 끝난 뒤 ‘작업 기록’에서 되돌릴 수 있어요.'}</div>`;
    $$('input[data-i]', wrap).forEach((c) => (c.onchange = () => { c.checked ? excluded.delete(+c.dataset.i) : excluded.add(+c.dataset.i); paint(); wrap.onchange && wrap.onchange(sum()); }));
  };
  paint();
  wrap.count = sum;
  wrap.skips = () => [...excluded];
  return wrap;
}

async function runPlan(plan, skips) {
  try {
    await api('/api/ops/run', { id: plan.id, skip: skips || [] });
    closeModal();
    toast(`⏳ ${plan.title} — 시작했어요`);
    poll();
  } catch (e) { toast('😢 ' + e.message); }
}

// 끝난 작업 → 알림 + 되돌리기 + 화면 새로 고침
window.onJobDone = async (j) => {
  if (j.kind === 'cleanup') { window.onCleanupDone && onCleanupDone(j); return; }
  if (j.kind !== 'ops' && j.kind !== 'auto') return;
  let res = null;
  if (j.hasResult) res = (await api('/api/job/result', { id: j.id }).catch(() => null))?.result;
  const ok = j.state === 'done';
  const msg = `${ok ? '✅' : j.state === 'canceled' ? '⏹️' : '⚠️'} ${j.title} · ${j.err || j.msg}`;
  if (res?.history && res.count > 0) toast(msg, 8000, { label: '↩︎ 되돌리기', fn: () => undoHistory(res.history) });
  else toast(msg, 5000);
  if (res?.errors?.length) console.warn(res.errors);
  S.recent = null;
  if (S.view === 'search') { if (S.res) runSearch(); else renderBrowse(true); }
  if (S.view === 'basket') VIEWS.basket();
  if (S.view === 'history') VIEWS.history();
  if (S.view === 'cleanup' && window.refreshCleanup) refreshCleanup();
  if (S.cur && !(await api('/api/doc', { path: S.cur }).catch(() => null))) { S.cur = ''; $('#preview').hidden = true; }
};
async function undoHistory(id) {
  try { await api('/api/history/undo', { id }); toast('↩︎ 되돌리는 중이에요'); poll(); } catch (e) { toast('⚠️ ' + e.message); }
}

// ════════════════════════════════════════════════ 옮기기 · 복사
function destDialog(op, paths) {
  const body = document.createElement('div');
  const st = S.settings;
  let dest = st.lastDest || '';
  const places = [
    ...(st.lastDest ? [{ p: st.lastDest, n: '최근에 쓴 폴더', i: '🕘' }] : []),
    { p: st.documents, n: '문서', i: '📄' }, { p: st.home + '/Desktop', n: '바탕 화면', i: '🖥️' }, { p: st.downloads, n: '다운로드', i: '⬇️' },
    ...(st.roots || []).filter((r) => ![st.documents, st.home + '/Desktop', st.downloads].includes(r)).map((r) => ({ p: r, n: folderLabel(r), i: '📚' })),
  ];
  let planEl = null, plan = null;
  const render = () => {
    body.innerHTML = `
      <div class="field"><span class="fl">어디로 ${op === 'copy' ? '복사할까요' : '옮길까요'}?</span>
        <div class="folder-pick"><span class="fi" data-art="folder"></span><div class="fx"><b>${dest ? esc(folderLabel(dest)) : '폴더를 골라 주세요'}</b><small>${dest ? '‎' + esc(shortPath(dest)) + '‎' : ''}</small></div>
        <button class="btn ghost sm" id="ddPick">다른 폴더…</button></div>
        <div class="chips" style="margin-top:6px">${places.map((x) => `<button class="chip${x.p === dest ? ' on' : ''}" data-p="${esc(x.p)}" style="--c:var(--green)">${x.i} ${esc(x.n)}</button>`).join('')}</div>
      </div>
      <div id="ddPlan"></div>`;
    paintArt(body);
    $('#ddPick', body).onclick = async () => { const p = await chooseFolder(op === 'copy' ? '복사할 폴더를 고르세요' : '옮길 폴더를 고르세요', dest || st.documents); if (p) { dest = p; render(); } };
    $$('.chip[data-p]', body).forEach((b) => (b.onclick = () => { dest = b.dataset.p; render(); }));
    if (dest) makePlan();
    else setModalActions([{ label: '취소', onClick: closeModal }, { label: `${OP_WORD[op]}`, primary: true, disabled: true }]);
  };
  const makePlan = async () => {
    try { plan = (await api('/api/ops/plan', { op, paths, dest })).plan; }
    catch (e) { $('#ddPlan', body).innerHTML = `<div class="upd-err">${esc(e.message)}</div>`; return; }
    planEl = planView(plan);
    $('#ddPlan', body).innerHTML = ''; $('#ddPlan', body).append(planEl);
    const acts = () => setModalActions([{ label: '취소', onClick: closeModal }, {
      label: `${comma(planEl.count())}개 ${OP_WORD[op]}`, primary: true, disabled: !planEl.count(),
      onClick: () => { api('/api/settings', { lastDest: dest }).then((s) => (S.settings = s)).catch(() => {}); runPlan(plan, planEl.skips()); },
    }]);
    planEl.onchange = acts; acts();
  };
  openModal({ title: op === 'copy' ? '📑 복사하기' : '📂 옮기기', sub: `고른 문서 ${paths.length}개`, body, wide: true });
  render();
}

// ════════════════════════════════════════════════ 이름 한꺼번에 바꾸기
function renameDialog(paths) {
  const body = document.createElement('div');
  const o = { find: '', replace: '', base: '', prefix: '', suffix: '', number: paths.length > 1, numStart: 1, numDigits: paths.length >= 100 ? 3 : 2, numPos: 'end', date: '', case: '', spaces: '' };
  if (paths.length === 1) o.base = baseName(paths[0]).replace(/\.[^.]+$/, '');
  body.innerHTML = `
    <div class="rn-grid">
      <div class="field"><label>새 이름 <span class="note">({이름} = 원래 이름)</span></label><input class="input" data-k="base" placeholder="비우면 원래 이름 그대로" value="${esc(o.base)}"></div>
      <div class="field"><label>바꿀 글자 → 새 글자</label><div style="display:flex;gap:6px"><input class="input" data-k="find" placeholder="찾을 글자" style="flex:1;min-width:0"><input class="input" data-k="replace" placeholder="바꿀 글자 (비우면 지움)" style="flex:1;min-width:0"></div></div>
      <div class="field"><label>앞에 붙이기</label><input class="input" data-k="prefix" placeholder="예: [회사]_"></div>
      <div class="field"><label>뒤에 붙이기</label><input class="input" data-k="suffix" placeholder="예: _검토"></div>
      <div class="field"><label>번호 붙이기</label><div style="display:flex;gap:8px;align-items:center">
        <label class="switch-row"><input type="checkbox" data-k="number" ${o.number ? 'checked' : ''}><span class="switch"></span></label>
        <select class="input" data-k="numPos"><option value="end">뒤에 (_01)</option><option value="start">앞에 (01_)</option></select>
        <input class="input" type="number" min="0" data-k="numStart" value="1" style="width:70px" title="시작 번호">
        <select class="input" data-k="numDigits"><option value="1">1</option><option value="2" ${o.numDigits === 2 ? 'selected' : ''}>01</option><option value="3" ${o.numDigits === 3 ? 'selected' : ''}>001</option></select></div></div>
      <div class="field"><label>날짜 · 모양</label><div style="display:flex;gap:6px;flex-wrap:wrap">
        <select class="input" data-k="date"><option value="">날짜 안 붙임</option><option value="ymd">앞에 고친 날짜 (20240301_)</option><option value="y-m-d">앞에 고친 날짜 (2024-03-01_)</option></select>
        <select class="input" data-k="spaces"><option value="">띄어쓰기 그대로</option><option value="underscore">띄어쓰기 → _</option><option value="remove">띄어쓰기 없애기</option></select>
        <select class="input" data-k="case"><option value="">영문 그대로</option><option value="lower">영문 소문자</option><option value="upper">영문 대문자</option></select></div></div>
    </div>
    <div id="rnPlan"></div>`;
  let plan = null, planEl = null, timer = null, seq = 0;
  const refresh = async () => {
    const my = ++seq;
    let r;
    try { r = await api('/api/ops/plan', { op: 'rename', paths, rename: o }); } catch (e) { $('#rnPlan', body).innerHTML = `<div class="upd-err">${esc(e.message)}</div>`; return; }
    if (my !== seq) return;
    plan = r.plan; planEl = planView(plan);
    $('#rnPlan', body).innerHTML = ''; $('#rnPlan', body).append(planEl);
    const acts = () => setModalActions([{ label: '취소', onClick: closeModal }, { label: `${comma(planEl.count())}개 이름 바꾸기`, primary: true, disabled: !planEl.count(), onClick: () => runPlan(plan, planEl.skips()) }]);
    planEl.onchange = acts; acts();
  };
  $$('[data-k]', body).forEach((el) => {
    const k = el.dataset.k;
    if (el.tagName === 'SELECT' && o[k] !== undefined) el.value = String(o[k]);
    const upd = () => {
      o[k] = el.type === 'checkbox' ? el.checked : el.type === 'number' || k === 'numDigits' ? +el.value : el.value;
      clearTimeout(timer); timer = setTimeout(refresh, 180);
    };
    el.addEventListener('input', upd); el.addEventListener('change', upd);
  });
  openModal({ title: '✏️ 이름 한꺼번에 바꾸기', sub: `고른 문서 ${paths.length}개 · 확장자(.pdf 등)는 그대로 둬요`, body, wide: true });
  refresh();
}

// ════════════════════════════════════════════════ 분류 (고른 문서)
const MODES = [
  { v: 'kind', i: '📚', t: '종류별', d: 'PDF · 한글 · 엑셀…' },
  { v: 'kinddate', i: '🗂️', t: '종류 + 연도', d: '한글/2024년' },
  { v: 'date', i: '📅', t: '날짜별', d: '2024년/03월' },
  { v: 'rules', i: '📏', t: '나만의 규칙', d: '이름 · 종류로' },
];
function modesHTML(cur) {
  return `<div class="modes">${MODES.map((m) => `<button class="mode${m.v === cur ? ' on' : ''}" data-m="${m.v}"><span class="mi">${m.i}</span>${m.t}<small>${m.d}</small></button>`).join('')}</div>`;
}
function organizeDialog(paths, opts = {}) {
  const body = document.createElement('div');
  const st = S.settings;
  const o = { mode: opts.mode || 'kind', dest: opts.dest || st.defaultDest, rest: 'kind', copy: false, source: opts.source || '', recursive: !!opts.recursive };
  let plan = null, planEl = null;
  const render = () => {
    body.innerHTML = `
      <div class="field"><span class="fl">어떻게 나눌까요?</span>${modesHTML(o.mode)}</div>
      ${o.mode === 'rules' ? `<div class="field"><label class="switch-row"><input type="checkbox" id="orRest" ${o.rest === 'kind' ? 'checked' : ''}><span class="switch"></span>규칙에 맞지 않는 문서는 종류별 폴더로</label><div class="note">규칙은 ‘분류하기’ 화면에서 고칠 수 있어요.</div></div>` : ''}
      <div class="field"><span class="fl">정리한 문서를 둘 곳</span>
        <div class="folder-pick"><span class="fi" data-art="folder"></span><div class="fx"><b>${esc(folderLabel(o.dest))}</b><small>‎${esc(shortPath(o.dest))}‎</small></div><button class="btn ghost sm" id="orPick">바꾸기…</button></div></div>
      <div class="field"><label class="switch-row"><input type="checkbox" id="orCopy" ${o.copy ? 'checked' : ''}><span class="switch"></span>원래 자리에 두고 복사본으로 정리하기</label></div>
      <div id="orPlan"></div>`;
    paintArt(body);
    $$('.mode', body).forEach((b) => (b.onclick = () => { o.mode = b.dataset.m; render(); }));
    $('#orPick', body).onclick = async () => { const p = await chooseFolder('정리한 문서를 둘 폴더를 고르세요', o.dest); if (p) { o.dest = p; render(); } };
    $('#orCopy', body).onchange = (e) => { o.copy = e.target.checked; makePlan(); };
    const rest = $('#orRest', body); if (rest) rest.onchange = (e) => { o.rest = e.target.checked ? 'kind' : 'stay'; makePlan(); };
    makePlan();
  };
  const makePlan = async () => {
    try { plan = (await api('/api/ops/plan', { op: 'organize', paths, organize: o })).plan; }
    catch (e) { $('#orPlan', body).innerHTML = `<div class="upd-err">${esc(e.message)}</div>`; setModalActions([{ label: '닫기', onClick: closeModal }]); return; }
    planEl = planView(plan, { group: true });
    $('#orPlan', body).innerHTML = ''; $('#orPlan', body).append(planEl);
    const acts = () => setModalActions([{ label: '취소', onClick: closeModal }, { label: `${comma(planEl.count())}개 ${o.copy ? '복사해서 ' : ''}분류하기`, primary: true, disabled: !planEl.count(), onClick: () => runPlan(plan, planEl.skips()) }]);
    planEl.onchange = acts; acts();
  };
  openModal({ title: '🗂️ 분류하기', sub: paths.length ? `고른 문서 ${paths.length}개` : `‘${folderLabel(o.source)}’ 폴더의 문서`, body, wide: true });
  render();
}

// ════════════════════════════════════════════════ 휴지통
async function trashDialog(paths) {
  let plan;
  try { plan = (await api('/api/ops/plan', { op: 'trash', paths })).plan; } catch (e) { return toast('⚠️ ' + e.message); }
  const body = document.createElement('div');
  const el = planView(plan);
  body.append(el);
  const acts = () => setModalActions([{ label: '취소', onClick: closeModal }, { label: `🗑️ ${comma(el.count())}개 휴지통으로`, danger: true, disabled: !el.count(), onClick: () => runPlan(plan, el.skips()) }]);
  el.onchange = acts;
  openModal({ title: '🗑️ 휴지통으로 옮기기', sub: '바로 지우지 않아요. 휴지통에서 다시 꺼낼 수 있어요.', body });
  acts();
}
