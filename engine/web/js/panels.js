// LIBROAD — 설정 · 업데이트 · 처음 안내 · 시작
// made by. Nevertheless_D
'use strict';

// ════════════════════════════════════════════════ 설정
let settingsRender = null;
function openSettings() {
  const body = document.createElement('div');
  body.style.cssText = 'display:flex;flex-direction:column;gap:12px';
  const render = () => {
    const st = S.settings, u = S.update || {}, l = S.lib || {};
    const ocrNote = { yes: '이 Mac의 글자 인식(Vision)으로 한국어 · 영어를 읽어요.', noko: '이 macOS에서는 영어만 읽을 수 있어요. 한국어는 macOS 13(Ventura) 이상에서 돼요.', no: '이 Mac에서는 글자 인식을 쓸 수 없어요.', checking: '글자 인식을 쓸 수 있는지 확인하는 중이에요…' }[st.ocrSupport] || '';
    body.innerHTML = `
      <div class="card"><div class="card-title">🌓 화면 모드</div>
        <div class="seg" id="stTheme"><button data-v="system">시스템 설정</button><button data-v="light">라이트</button><button data-v="dark">다크</button></div>
        <div class="note" style="margin-top:6px">‘시스템 설정’을 고르면 ${L.os}의 라이트/다크 모드를 그대로 따라가요.</div></div>
      <div class="card"><div class="card-title">📚 내 서재 (찾을 폴더)</div>
        <div class="note" style="margin-bottom:8px">여기 넣은 폴더 안의 문서를 살펴서 찾기 · 정리해요. 외장 디스크의 폴더도 넣을 수 있어요.</div>
        ${(st.roots || []).map((r) => `<div class="folder-pick" style="margin-bottom:6px"><span class="fi" data-art="folder"></span><div class="fx"><b>${esc(folderLabel(r))}</b><small>‎${esc(shortPath(r))}‎</small></div><button class="btn ghost sm" data-rm="${esc(r)}">빼기</button></div>`).join('') || '<div class="note">비어 있어요</div>'}
        <div style="display:flex;gap:6px;margin-top:4px"><button class="btn ghost sm" id="stAddRoot">＋ 폴더 넣기</button><button class="btn ghost sm" id="stResetRoots">기본으로 (문서 · 바탕 화면 · 다운로드)</button><span class="sp"></span><button class="btn ghost sm" id="stRescan">🔄 지금 다시 살피기</button></div></div>
      <div class="card"><div class="card-title">🔤 글자 인식 (OCR)</div>
        <label class="switch-row"><input type="checkbox" id="stOcr" ${st.ocr ? 'checked' : ''} ${st.ocrSupport === 'no' ? 'disabled' : ''}><span class="switch"></span>스캔 문서 · 사진 속 글자도 읽어 두기</label>
        <div class="note" style="margin-top:6px">${ocrNote}<br>처음에는 시간이 걸려요(한 쪽에 1초쯤). 한 번 읽은 글자는 기억해 두고, 글자가 이미 있는 PDF는 건너뛰어요. 손글씨나 흐린 사진은 잘 못 읽을 수 있어요.${l.pending?.ocr ? `<br>기다리는 문서: <b>${comma(l.pending.ocr)}</b>개` : ''}</div></div>
      <div class="card"><div class="card-title">🔔 알림</div>
        <label class="switch-row"><input type="checkbox" id="stNotify" ${st.notify ? 'checked' : ''}><span class="switch"></span>자동 정리가 끝나면 ${L.os} 알림 보내기</label></div>
      <div class="card"><div class="card-title">ℹ️ 정보</div>
        <div class="kv"><span>버전</span><b>LIBROAD ${esc(st.build)}</b></div>
        <div class="kv"><span>업데이트</span><b>${u.autoUpdate === false ? '자동 업데이트 꺼짐' : '새 버전이 나오면 앱이 알아서 받아요'}</b></div>
        <div class="kv"><span>색인 · 설정 저장 위치</span><b style="font-weight:600;font-size:11.5px">~/Library/Application Support/LIBROAD</b></div>
        <div style="display:flex;gap:6px;margin-top:6px"><button class="btn ghost sm" id="stUpdate" style="flex:1">🔄 업데이트 확인…</button><button class="btn ghost sm" id="stWelcome" style="flex:1">👋 처음 안내 다시 보기</button><button class="btn ghost sm" id="stQuit" style="flex:1">⏻ LIBROAD 종료</button></div></div>
      <div class="note" style="text-align:center">🔒 모든 작업은 이 Mac 안에서만 해요. 문서 내용은 어디에도 보내지 않아요.<br><span style="font:italic 13px var(--serif);color:var(--brass-ink)">made by. Nevertheless_D</span></div>`;
    paintArt(body);
    $$('#stTheme button', body).forEach((b) => {
      b.classList.toggle('on', b.dataset.v === (st.appearance || 'system'));
      b.onclick = async () => { applyTheme(b.dataset.v); S.settings = await api('/api/settings', { appearance: b.dataset.v }); render(); };
    });
    $$('[data-rm]', body).forEach((b) => (b.onclick = async () => {
      if (!(await confirmBox('서재에서 빼기', `‘${esc(folderLabel(b.dataset.rm))}’ 폴더를 서재에서 뺄까요?<br><span class="note">폴더와 파일은 그대로 있어요. 찾기에서만 빠져요.</span>`, '빼기'))) { openSettings(); return; }
      S.settings = await api('/api/library/roots', { action: 'remove', path: b.dataset.rm });
      if (S.q.scope && S.q.scope.startsWith(b.dataset.rm)) setScope('');
      openSettings(); poll();
    }));
    $('#stAddRoot', body).onclick = async () => { await addRoot(); render(); };
    $('#stResetRoots', body).onclick = async () => { S.settings = await api('/api/library/roots', { action: 'reset' }); render(); poll(); };
    $('#stRescan', body).onclick = async () => { await api('/api/library/rescan', {}); toast('🔄 서재를 다시 살펴볼게요'); poll(); };
    $('#stOcr', body).onchange = async (e) => {
      S.settings = await api('/api/settings', { ocr: e.target.checked });
      toast(e.target.checked ? '🔤 글자 인식을 켰어요. 스캔 문서 · 사진을 차례로 읽어요' : '글자 인식을 껐어요. 이미 읽은 글자는 그대로 찾을 수 있어요');
      render(); poll();
    };
    $('#stNotify', body).onchange = async (e) => { S.settings = await api('/api/settings', { notify: e.target.checked }); };
    $('#stUpdate', body).onclick = () => { closeModal(); openUpdate(true); };
    $('#stWelcome', body).onclick = () => { closeModal(); showWelcome(); };
    $('#stQuit', body).onclick = quitApp;
  };
  render();
  settingsRender = render;
  openModal({ title: '⚙️ LIBROAD 설정', body, onClose: () => { settingsRender = null; } });
}
$('#settingsBtn').onclick = openSettings;

// ════════════════════════════════════════════════ 버전 · 업데이트
// 새 버전 정보는 인터넷의 업데이트 정보 파일에서 받아요. (문서 내용은 보내지 않아요)
const U = { open: false, sig: '', restarting: false, toldReady: '' };
window.U = U;
const updBody = document.createElement('div');
updBody.style.cssText = 'display:flex;flex-direction:column;gap:12px';

function onUpdateState(st) {
  S.update = st;
  renderVersion();
  const sig = JSON.stringify(st);
  if (U.open && sig !== U.sig) renderUpdate();
  U.sig = sig;
  if (st.status === 'ready' && st.installed && U.toldReady !== st.installed && !U.open) {
    U.toldReady = st.installed;
    toast(`✨ LIBROAD ${st.installed} 준비 완료! 오른쪽 위 배지를 눌러 다시 시작하세요.`, 6000);
  }
}
window.onUpdateState = onUpdateState;

function renderVersion() {
  const st = S.update; if (!st) return;
  let txt = 'v' + st.current, isNew = false;
  if (st.status === 'available') { txt = `새 버전 ${st.latest.version}`; isNew = true; }
  else if (st.status === 'downloading') { txt = `받는 중 ${Math.round(st.progress * 100)}%`; isNew = true; }
  else if (st.status === 'installing') { txt = '설치 중…'; isNew = true; }
  else if (st.status === 'ready') { txt = '다시 시작해서 업데이트'; isNew = true; }
  $('#verText').textContent = txt;
  $('#verBadge').classList.toggle('new', isNew);
}

function renderUpdate() {
  const st = S.update; if (!st) return;
  const Lr = st.latest;
  let t1 = '', t2 = '', showVers = false, acts = [];
  switch (st.status) {
    case 'checking': t1 = '새 버전을 확인하고 있어요…'; t2 = '잠시만 기다려 주세요.'; break;
    case 'available':
      t1 = '새 버전이 나왔어요! 🎉'; t2 = '받는 데 몇 초면 끝나요. 하던 작업은 그대로 둬도 돼요.'; showVers = true;
      acts = [{ label: '⬇︎ 지금 업데이트', primary: true, onClick: installUpdate }]; break;
    case 'downloading': t1 = '새 버전을 받고 있어요…'; t2 = `${Math.round(st.progress * 100)}%`; showVers = true; break;
    case 'installing': t1 = '설치하고 있어요…'; t2 = '잠시만 기다려 주세요.'; showVers = true; break;
    case 'ready':
      t1 = `LIBROAD ${st.installed} 준비 완료 ✨`; t2 = '다시 시작하면 새 버전으로 바뀌어요. (몇 초 걸려요)';
      acts = [{ label: '↻ 지금 다시 시작', primary: true, onClick: restartApp }]; break;
    case 'uptodate': t1 = '최신 버전을 쓰고 있어요 👍'; t2 = `LIBROAD ${st.current}`; break;
    case 'error': t1 = '업데이트를 확인하지 못했어요'; t2 = '인터넷 연결을 확인한 뒤 다시 시도해 주세요.'; break;
    default: t1 = `LIBROAD ${st.current}`; t2 = '버튼을 눌러 새 버전이 있는지 확인해 보세요.';
  }
  const busy = ['checking', 'downloading', 'installing'].includes(st.status);
  if (!['available', 'ready'].includes(st.status)) acts = [{ label: '🔄 업데이트 확인', primary: true, disabled: busy, onClick: checkUpdate }];
  const notes = ['available', 'downloading', 'installing', 'ready'].includes(st.status) && Lr?.notes?.length
    ? `<div class="card"><div class="card-title">📝 ${esc(Lr.version)}에서 바뀐 점</div><ul class="upd-notes">${Lr.notes.map((n) => `<li>${esc(n)}</li>`).join('')}</ul></div>` : '';
  updBody.innerHTML = `
    <div class="upd-hero"><img src="/icon.png" alt=""><div><div class="t1">${t1}</div><div class="t2">${esc(t2)}</div></div></div>
    ${showVers && Lr ? `<div class="upd-vers"><div class="upd-ver"><small>지금 버전</small><b>${esc(st.current)}</b></div><span class="upd-arrow">➜</span><div class="upd-ver new"><small>새 버전</small><b>${esc(Lr.version)}</b></div></div>` : ''}
    ${st.status === 'downloading' || st.status === 'installing' ? `<div class="upd-bar"><i style="width:${st.status === 'installing' ? 100 : Math.round(st.progress * 100)}%"></i></div>` : ''}
    ${st.status === 'error' && st.error ? `<div class="upd-err">${esc(st.error)}</div>` : ''}
    ${notes}
    <div class="card">
      <label class="switch-row"><input type="checkbox" id="updAuto" ${st.autoUpdate ? 'checked' : ''}><span class="switch"></span>자동 업데이트 (새 버전이 나오면 앱이 알아서 받아요)</label>
      <div class="kv" style="margin-top:6px"><span>지금 버전</span><b>LIBROAD ${esc(st.current)}</b></div>
      <div class="kv"><span>마지막 확인</span><b>${fmtWhen(st.lastCheck)}</b></div>
      ${st.canRollback ? `<div class="kv"><span class="note">문제가 생겼다면 이전 버전(${esc(st.backupVersion)})으로 되돌릴 수 있어요.</span><button class="link" id="updRollback">되돌리기</button></div>` : ''}
    </div>`;
  $('#updAuto', updBody).onchange = async (e) => {
    S.settings = await api('/api/settings', { autoUpdate: e.target.checked });
    S.update.autoUpdate = e.target.checked;
    toast(e.target.checked ? '✅ 자동 업데이트를 켰어요' : '자동 업데이트를 껐어요. 필요할 때 직접 확인하세요.');
  };
  const rb = $('#updRollback', updBody); if (rb) rb.onclick = rollbackUpdate;
  setModalActions(acts);
}
function openUpdate(check = false) {
  U.open = true;
  openModal({ title: '🔄 업데이트', body: updBody, onClose: () => { U.open = false; } });
  renderUpdate();
  if (check && !['available', 'ready', 'downloading', 'installing'].includes(S.update?.status)) checkUpdate();
}
$('#verBadge').onclick = () => openUpdate(true);
async function checkUpdate() {
  if (S.update) { S.update.status = 'checking'; renderUpdate(); renderVersion(); }
  try { onUpdateState(await api('/api/update/check')); } catch (e) { toast('⚠️ ' + e.message); }
}
async function installUpdate() {
  try { await api('/api/update/install', {}); } catch (e) { toast('⚠️ ' + e.message); }
  if (S.update) { S.update.status = 'downloading'; S.update.progress = 0; renderUpdate(); }
}
async function rollbackUpdate() {
  if (!(await confirmBox('이전 버전으로', `이전 버전(${esc(S.update.backupVersion)})으로 되돌릴까요?<br><span class="note">되돌리면 자동 업데이트는 꺼져요. (설정에서 다시 켤 수 있어요)</span>`, '되돌리기'))) { openUpdate(); return; }
  try { await api('/api/update/rollback', {}); } catch (e) { toast('⚠️ ' + e.message); return; }
  toast('↩︎ 이전 버전을 준비했어요. 다시 시작하면 바뀌어요.');
  poll();
}
async function restartApp() {
  const busy = S.jobs.some((j) => j.state === 'running' && (j.kind === 'ops' || j.kind === 'auto'));
  if (busy) { toast('📦 파일을 옮기는 중이에요. 끝난 뒤에 다시 시작해 주세요.'); return; }
  const from = S.update?.current;
  U.restarting = true;
  closeModal();
  const veil = document.createElement('div');
  veil.className = 'restart-veil';
  veil.innerHTML = '<img src="/icon.png" alt="">새 버전으로 다시 시작하고 있어요…<span>창을 닫지 말고 잠시만 기다려 주세요.</span>';
  document.body.append(veil);
  try { await api('/api/update/restart', {}); }
  catch (e) { veil.remove(); U.restarting = false; toast('⚠️ ' + e.message); return; }
  const t0 = Date.now();
  const tick = async () => {
    try { const j = await api('/api/ping'); if (j.build && j.build !== from) { location.reload(); return; } } catch { /* 다시 켜지는 중 */ }
    if (Date.now() - t0 > 60000) { showEnded('다시 시작에 시간이 걸리고 있어요. LIBROAD 앱을 다시 실행해 주세요.'); return; }
    setTimeout(tick, 600);
  };
  setTimeout(tick, 1200);
}

// ════════════════════════════════════════════════ 처음 안내
function showWelcome() {
  const body = document.createElement('div');
  body.innerHTML = `<div class="welcome"><div class="wa" data-art="welcome"></div>
    <ul class="wlist">
      <li><span class="e">📚</span><div><b>문서 · 바탕 화면 · 다운로드 폴더를 서재로 살펴요</b><span class="note">처음에 “LIBROAD가 … 폴더에 접근하려고 합니다” 창이 뜨면 <b>허용</b>을 눌러 주세요.</span></div></li>
      <li><span class="e">🔎</span><div><b>이름 · 본문 · 초성 · 스캔 문서의 글자까지 찾아요</b><span class="note">처음 한 번은 문서를 읽느라 시간이 걸려요. 아래 진행 막대에서 남은 시간을 볼 수 있어요.</span></div></li>
      <li><span class="e">↩️</span><div><b>옮기기 · 지우기는 미리 보여 드리고, 되돌릴 수 있어요</b><span class="note">같은 이름은 ‘(2)’를 붙이고, 지울 때는 휴지통으로 보내요.</span></div></li>
      <li><span class="e">🔒</span><div><b>모든 작업은 이 Mac 안에서만 해요</b><span class="note">문서 내용은 어디에도 보내지 않아요.</span></div></li>
    </ul></div>`;
  openModal({ title: '👋 LIBROAD에 오신 걸 환영해요', sub: '내 문서 도서관 — 찾고 · 분류하고 · 옮기고 · 정리해요', body, wide: true,
    actions: [{ label: '시작하기', primary: true, onClick: closeModal }],
    onClose: () => { if (!S.settings.seen) api('/api/settings', { seen: true }).then((s) => (S.settings = s)).catch(() => {}); } });
}

// ════════════════════════════════════════════════ 종료
async function quitApp() {
  const busy = S.jobs.some((j) => j.state === 'running' && (j.kind === 'ops' || j.kind === 'auto'));
  if (!(await confirmBox('LIBROAD 종료', busy ? '파일을 옮기는 중이에요. 지금 끄면 남은 파일은 옮기지 않아요. 그래도 종료할까요?' : 'LIBROAD를 종료할까요?' + (S.settings.auto?.enabled ? '<br><span class="note">종료하면 자동 정리도 멈춰요.</span>' : ''), '종료'))) return;
  await api('/api/quit', {}).catch(() => {});
  showEnded('LIBROAD를 종료했어요 👋');
  setTimeout(() => window.close(), 400);
}
$('#quitBtn').onclick = quitApp;

// ════════════════════════════════════════════════ 시작
(async function init() {
  paintArt();
  if (!T) { showEnded('LIBROAD 앱에서 열어 주세요'); return; }
  try { S.settings = await api('/api/settings'); }
  catch { showEnded('앱 엔진에 연결하지 못했어요'); return; }
  setPlatform(S.settings.os);
  applyTheme(S.settings.appearance);
  S.kinds = S.settings.kinds;
  for (const k of S.kinds) window.KINDS[k.id] = k;
  try { const p = JSON.parse(localStorage.getItem('libroad-q') || '{}'); S.q.noBody = !!p.noBody; S.q.noOcr = !!p.noOcr; } catch { /* */ }
  try { const v = localStorage.getItem('libroad-viewmode'); if (v === 'grid' || v === 'list') { S.viewMode = v; $$('#viewMode button').forEach((x) => x.classList.toggle('on', x.dataset.v === v)); } } catch { /* */ }
  $('#verText').textContent = 'v' + S.settings.build;
  paintToggles(); paintScope(); renderSaved(); renderChips();
  await poll();
  renderBrowse();
  setInterval(poll, 800);
  qIn.focus();
  if (!S.settings.seen) showWelcome();
})();
