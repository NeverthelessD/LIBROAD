// LIBROAD — 그림 (SVG). 색은 CSS 변수를 따라가서 라이트/다크 모드에 맞춰 바뀌어요.
// made by. Nevertheless_D
'use strict';

const V = (n) => `var(--${n})`;
const BOOKC = ['#d4473b', '#2f7fd6', '#1f8a4c', '#d9762b', '#8b5cc4', '#3c5fb8', '#b0822f', '#7a7066', '#c25d8a', '#2aa1a1'];

function bookRow(x0, base, specs) {
  let x = x0, out = '';
  for (const [w, h, c, lean] of specs) {
    const y = base - h;
    if (lean) {
      out += `<g transform="rotate(${lean} ${x + w} ${base})"><rect x="${x}" y="${y}" width="${w}" height="${h}" rx="2" style="fill:${c}"/><rect x="${x + 2}" y="${y + 8}" width="${w - 4}" height="3" style="fill:#fff;opacity:.35"/><rect x="${x + 2}" y="${base - 14}" width="${w - 4}" height="2" style="fill:#fff;opacity:.3"/></g>`;
    } else {
      out += `<rect x="${x}" y="${y}" width="${w}" height="${h}" rx="2" style="fill:${c}"/>
        <rect x="${x}" y="${y}" width="${w * 0.28}" height="${h}" rx="1.5" style="fill:#000;opacity:.12"/>
        <rect x="${x + 2}" y="${y + 9}" width="${w - 4}" height="3" style="fill:#fff;opacity:.38"/>
        <rect x="${x + 2}" y="${y + 14}" width="${w - 4}" height="1.5" style="fill:#fff;opacity:.3"/>
        <rect x="${x + 3}" y="${base - 16}" width="${Math.max(1, w - 6)}" height="5" rx="1" style="fill:#fff;opacity:.25"/>`;
    }
    x += w + 2;
  }
  return out;
}

const ART = {
  hero: () => `<svg viewBox="0 0 330 150" xmlns="http://www.w3.org/2000/svg">
    <defs>
      <radialGradient id="hgGlass" cx="40%" cy="35%" r="70%"><stop offset="0" stop-color="#fff" stop-opacity=".85"/><stop offset=".55" stop-color="#dff1ff" stop-opacity=".35"/><stop offset="1" stop-color="#9cc9e8" stop-opacity=".25"/></radialGradient>
      <linearGradient id="hgShade" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#2f8a63"/><stop offset="1" stop-color="#155a40"/></linearGradient>
      <radialGradient id="hgLight" cx="50%" cy="0%" r="90%"><stop offset="0" stop-color="#ffe9a8" stop-opacity=".75"/><stop offset="1" stop-color="#ffe9a8" stop-opacity="0"/></radialGradient>
    </defs>
    <!-- 램프 불빛 -->
    <path d="M30 52 L-6 132 L92 132 L60 52Z" fill="url(#hgLight)"/>
    <!-- 책장 -->
    <rect x="0" y="128" width="330" height="9" rx="2" style="fill:${V('shelf')}"/>
    <rect x="0" y="137" width="330" height="5" style="fill:${V('wood2')}"/>
    ${bookRow(88, 128, [[13, 70, BOOKC[0]], [17, 82, BOOKC[1]], [11, 64, BOOKC[6]], [15, 76, BOOKC[2]], [12, 58, BOOKC[3]], [19, 86, BOOKC[5]], [13, 66, BOOKC[4]], [11, 72, BOOKC[8]], [14, 60, BOOKC[9], -14]])}
    <!-- 눕혀 쌓은 책 -->
    <rect x="262" y="116" width="58" height="12" rx="2" style="fill:${BOOKC[2]}"/><rect x="262" y="116" width="58" height="3" style="fill:#fff;opacity:.3"/>
    <rect x="267" y="105" width="50" height="11" rx="2" style="fill:${BOOKC[6]}"/><rect x="267" y="105" width="50" height="3" style="fill:#fff;opacity:.3"/>
    <rect x="264" y="95" width="54" height="10" rx="2" style="fill:${BOOKC[0]}"/><rect x="264" y="95" width="54" height="3" style="fill:#fff;opacity:.3"/>
    <!-- 은행원 램프 -->
    <rect x="22" y="122" width="46" height="6" rx="3" style="fill:${V('brass')}"/>
    <rect x="43" y="60" width="4" height="64" style="fill:${V('brass')}"/>
    <path d="M18 62 Q45 30 72 62 Z" fill="url(#hgShade)"/>
    <rect x="16" y="60" width="58" height="5" rx="2.5" style="fill:${V('brass2')}"/>
    <path d="M28 46 Q40 38 52 42" stroke="#fff" stroke-opacity=".4" stroke-width="3" fill="none" stroke-linecap="round"/>
    <circle cx="66" cy="75" r="2.5" style="fill:${V('brass2')}"/><path d="M66 65 v8" style="stroke:${V('brass')}" stroke-width="1.5"/>
    <!-- 돋보기 -->
    <g transform="rotate(-18 236 70)">
      <rect x="230" y="98" width="13" height="44" rx="6" style="fill:${V('wood')}"/>
      <rect x="229" y="94" width="15" height="8" rx="2" style="fill:${V('brass')}"/>
      <circle cx="236" cy="58" r="38" style="fill:${V('brass')}"/>
      <circle cx="236" cy="58" r="32" style="fill:${V('panel')}"/>
      <circle cx="236" cy="58" r="32" fill="url(#hgGlass)"/>
      <g stroke-linecap="round" stroke-width="5"><path d="M214 46h44" style="stroke:${V('faint')}" opacity=".7"/><path d="M210 60h52" style="stroke:${V('mark')}" stroke-width="11" opacity=".95"/><path d="M216 60h38" style="stroke:${V('green-ink')}"/><path d="M214 74h40" style="stroke:${V('faint')}" opacity=".7"/></g>
      <path d="M214 42 A26 26 0 0 1 236 32" stroke="#fff" stroke-width="4" stroke-linecap="round" fill="none" opacity=".9"/>
    </g>
    <!-- 반짝임 -->
    <g style="fill:${V('brass2')}"><path d="M300 30 l2 6 6 2 -6 2 -2 6 -2 -6 -6 -2 6 -2z"/><path d="M190 18 l1.4 4 4 1.4 -4 1.4 -1.4 4 -1.4 -4 -4 -1.4 4 -1.4z" opacity=".8"/></g>
  </svg>`,

  'nav-search': () => `<svg viewBox="0 0 34 34"><rect width="34" height="34" rx="10" style="fill:${V('green-soft')}"/>
    <rect x="7" y="9" width="13" height="17" rx="1.5" style="fill:${V('green')}"/><rect x="9" y="12" width="8" height="1.6" fill="#fff" opacity=".6"/><rect x="9" y="15" width="6" height="1.6" fill="#fff" opacity=".6"/>
    <circle cx="20" cy="18" r="6" style="fill:${V('panel')};stroke:${V('brass')}" stroke-width="2.6"/><path d="M24.5 22.5 L28 26" style="stroke:${V('brass')}" stroke-width="3" stroke-linecap="round"/></svg>`,
  'nav-organize': () => `<svg viewBox="0 0 34 34"><rect width="34" height="34" rx="10" style="fill:${V('brass-soft')}"/>
    <rect x="6" y="8" width="22" height="19" rx="2.5" style="fill:${V('wood')}"/><rect x="8" y="10" width="18" height="7" rx="1.5" style="fill:${V('shelf')}"/><rect x="8" y="18.5" width="18" height="7" rx="1.5" style="fill:${V('shelf')}"/>
    <rect x="14" y="12.5" width="6" height="2" rx="1" style="fill:${V('brass2')}"/><rect x="14" y="21" width="6" height="2" rx="1" style="fill:${V('brass2')}"/></svg>`,
  'nav-cleanup': () => `<svg viewBox="0 0 34 34"><rect width="34" height="34" rx="10" fill="#e4ecf7"/>
    <path d="M23 6 L16 18" stroke="#8a5a2b" stroke-width="2.6" stroke-linecap="round"/>
    <path d="M11 17 L20 21 L17 28 Q12 27 7 23 Z" fill="#3f6fb4"/><path d="M12 22 l-2 4 M15 23 l-1.5 4.5" stroke="#fff" stroke-width="1.2" opacity=".6"/>
    <g fill="#d7ad5f"><path d="M26 18 l1 2.6 2.6 1 -2.6 1 -1 2.6 -1 -2.6 -2.6 -1 2.6 -1z"/><circle cx="9" cy="10" r="1.4"/></g></svg>`,
  'nav-basket': () => `<svg viewBox="0 0 34 34"><rect width="34" height="34" rx="10" fill="#f6e3dc"/>
    <rect x="10" y="7" width="5" height="12" rx="1" fill="#d4473b"/><rect x="15.5" y="9" width="5" height="10" rx="1" fill="#2f7fd6"/><rect x="21" y="8" width="4" height="11" rx="1" transform="rotate(12 23 19)" fill="#1f8a4c"/>
    <path d="M6 17 h22 l-3 11 h-16 z" fill="#b97a3c"/><path d="M8 20.5 h18 M9 24 h16" stroke="#8a5a2b" stroke-width="1.3"/><path d="M12 17 v11 M17 17 v11 M22 17 v11" stroke="#8a5a2b" stroke-width="1" opacity=".6"/></svg>`,
  'nav-convert': () => `<svg viewBox="0 0 34 34"><rect width="34" height="34" rx="10" fill="#fbe3df"/>
    <rect x="6" y="8" width="11" height="15" rx="1.5" fill="#2f7fd6"/><rect x="8" y="11" width="7" height="1.6" fill="#fff" opacity=".7"/><rect x="8" y="14" width="5" height="1.6" fill="#fff" opacity=".7"/>
    <path d="M17.5 17.5 h4" stroke="#b0822f" stroke-width="2" stroke-linecap="round"/><path d="M20.5 15.2 l2.6 2.3 -2.6 2.3" stroke="#b0822f" stroke-width="2" fill="none" stroke-linecap="round" stroke-linejoin="round"/>
    <rect x="18" y="12" width="11" height="15" rx="1.5" fill="#d4473b"/><text x="23.5" y="23.5" text-anchor="middle" font-family="Pretendard, sans-serif" font-weight="900" font-size="5" fill="#fff">PDF</text></svg>`,
  convert: () => `<svg viewBox="0 0 92 70"><ellipse cx="46" cy="64" rx="38" ry="4" style="fill:${V('chip')}"/>
    <g transform="rotate(-8 22 40)"><rect x="8" y="18" width="26" height="36" rx="2" fill="#2f7fd6"/><rect x="12" y="24" width="18" height="2.5" fill="#fff" opacity=".7"/><rect x="12" y="29" width="14" height="2.5" fill="#fff" opacity=".7"/><text x="21" y="49" text-anchor="middle" font-family="Pretendard" font-weight="900" font-size="7" fill="#fff">HWP</text></g>
    <rect x="14" y="22" width="26" height="36" rx="2" fill="#d9762b"/><rect x="18" y="28" width="18" height="2.5" fill="#fff" opacity=".7"/><text x="27" y="53" text-anchor="middle" font-family="Pretendard" font-weight="900" font-size="7" fill="#fff">PPT</text>
    <path d="M44 38 h10" style="stroke:${V('brass')}" stroke-width="3.5" stroke-linecap="round"/><path d="M51 33 l5 5 -5 5" style="stroke:${V('brass')}" stroke-width="3.5" fill="none" stroke-linecap="round" stroke-linejoin="round"/>
    <rect x="60" y="16" width="26" height="40" rx="2" fill="#d4473b"/><rect x="60" y="16" width="5" height="40" fill="#000" opacity=".15"/><rect x="66" y="23" width="16" height="2.5" fill="#fff" opacity=".7"/><rect x="66" y="28" width="12" height="2.5" fill="#fff" opacity=".7"/>
    <text x="74" y="50" text-anchor="middle" font-family="'DM Serif Display', serif" font-size="10" fill="#fff">PDF</text>
    <g style="fill:${V('brass2')}"><path d="M84 8 l1.4 3.4 3.4 1.4 -3.4 1.4 -1.4 3.4 -1.4 -3.4 -3.4 -1.4 3.4 -1.4z"/></g></svg>`,
  'nav-history': () => `<svg viewBox="0 0 34 34"><rect width="34" height="34" rx="10" fill="#ece5f6"/>
    <circle cx="18" cy="17" r="9" fill="#fff" stroke="#6b4fb0" stroke-width="2.4"/><path d="M18 12 v5.5 l3.5 2" stroke="#6b4fb0" stroke-width="2.2" stroke-linecap="round" fill="none"/>
    <path d="M6.5 13 l2.8 3.2 3.4-2.6" stroke="#6b4fb0" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" fill="none"/></svg>`,

  catalog: () => `<svg viewBox="0 0 42 42"><rect x="3" y="8" width="36" height="30" rx="4" style="fill:${V('wood')}"/>
    <rect x="6" y="11" width="14" height="11" rx="2" style="fill:${V('shelf')}"/><rect x="22" y="11" width="14" height="11" rx="2" style="fill:${V('shelf')}"/>
    <rect x="6" y="24" width="14" height="11" rx="2" style="fill:${V('shelf')}"/><rect x="22" y="24" width="14" height="11" rx="2" style="fill:${V('shelf')}"/>
    <rect x="10" y="15" width="6" height="2.5" rx="1" style="fill:${V('brass2')}"/><rect x="26" y="15" width="6" height="2.5" rx="1" style="fill:${V('brass2')}"/>
    <rect x="10" y="28" width="6" height="2.5" rx="1" style="fill:${V('brass2')}"/><rect x="26" y="28" width="6" height="2.5" rx="1" style="fill:${V('brass2')}"/>
    <rect x="24" y="3" width="11" height="10" rx="1.5" fill="#fffaf0" stroke="#d8c8a8"/><path d="M26 6.5h7M26 9h5" stroke="#c8b38a" stroke-width="1.2"/></svg>`,

  empty: () => `<svg viewBox="0 0 170 120"><ellipse cx="85" cy="108" rx="62" ry="7" style="fill:${V('chip')}"/>
    <path d="M85 34 Q60 24 26 28 L26 96 Q60 92 85 102 Z" style="fill:${V('panel')};stroke:${V('stroke2')}" stroke-width="2"/>
    <path d="M85 34 Q110 24 144 28 L144 96 Q110 92 85 102 Z" style="fill:${V('panel')};stroke:${V('stroke2')}" stroke-width="2"/>
    <g style="stroke:${V('faint')}" stroke-width="2" stroke-linecap="round" opacity=".6"><path d="M38 44 Q58 40 74 46M38 56 Q58 52 74 58M38 68 Q56 64 70 70"/><path d="M96 46 Q112 40 132 44M96 58 Q112 52 132 56"/></g>
    <circle cx="122" cy="74" r="13" style="fill:none;stroke:${V('brass')}" stroke-width="4"/><path d="M131 83 l10 10" style="stroke:${V('brass')}" stroke-width="5" stroke-linecap="round"/>
    <text x="118" y="79" style="font: 700 13px Pretendard; fill:${V('brass')}">?</text></svg>`,

  organize: () => `<svg viewBox="0 0 92 70"><ellipse cx="46" cy="64" rx="38" ry="4" style="fill:${V('chip')}"/>
    <rect x="12" y="20" width="62" height="6" rx="2" style="fill:${V('wood')}"/><rect x="12" y="40" width="62" height="6" rx="2" style="fill:${V('wood')}"/>
    ${bookRow(16, 20, [[6, 14, BOOKC[0]], [7, 16, BOOKC[0]], [5, 12, BOOKC[0]], [7, 15, BOOKC[1]], [6, 13, BOOKC[1]]])}
    ${bookRow(16, 40, [[6, 14, BOOKC[2]], [7, 16, BOOKC[2]], [6, 12, BOOKC[3]], [7, 15, BOOKC[3]], [5, 13, BOOKC[3]]])}
    <path d="M14 46 v12 M72 46 v12" style="stroke:${V('wood2')}" stroke-width="3"/><circle cx="18" cy="60" r="4" style="fill:${V('ink')}"/><circle cx="68" cy="60" r="4" style="fill:${V('ink')}"/>
    <path d="M78 10 q8 4 6 14" style="stroke:${V('brass')}" stroke-width="2.5" fill="none" stroke-linecap="round"/><path d="M81 23 l3 2 2-4" style="stroke:${V('brass')}" stroke-width="2.5" fill="none" stroke-linecap="round"/></svg>`,

  cleanup: () => `<svg viewBox="0 0 92 70"><ellipse cx="46" cy="64" rx="38" ry="4" style="fill:${V('chip')}"/>
    <rect x="14" y="50" width="40" height="9" rx="2" fill="#3c5fb8"/><rect x="18" y="41" width="34" height="9" rx="2" fill="#d9762b"/><rect x="16" y="32" width="36" height="9" rx="2" fill="#1f8a4c"/>
    <path d="M78 6 L58 40" stroke="#8a5a2b" stroke-width="3.5" stroke-linecap="round"/>
    <path d="M52 37 L66 45 L58 58 Q48 55 42 48 Z" fill="#c25d8a"/><path d="M52 47 l-4 6 M57 49 l-3 6" stroke="#fff" stroke-width="1.4" opacity=".55"/>
    <g style="fill:${V('brass2')}"><path d="M76 34 l1.6 4 4 1.6 -4 1.6 -1.6 4 -1.6 -4 -4 -1.6 4 -1.6z"/><path d="M28 14 l1.2 3 3 1.2 -3 1.2 -1.2 3 -1.2 -3 -3 -1.2 3 -1.2z"/><circle cx="40" cy="22" r="1.6"/></g></svg>`,

  basket: () => `<svg viewBox="0 0 92 70"><ellipse cx="46" cy="64" rx="36" ry="4" style="fill:${V('chip')}"/>
    <rect x="26" y="10" width="10" height="28" rx="2" fill="#d4473b"/><rect x="37" y="14" width="11" height="24" rx="2" fill="#2f7fd6"/><rect x="50" y="12" width="9" height="26" rx="2" transform="rotate(14 54 38)" fill="#1f8a4c"/>
    <path d="M14 36 h64 l-8 24 h-48 z" fill="#b97a3c"/><path d="M17 43 h58 M20 51 h52" stroke="#8a5a2b" stroke-width="2"/><path d="M28 36 v24 M40 36 v24 M52 36 v24 M64 36 v24" stroke="#8a5a2b" stroke-width="1.4" opacity=".6"/>
    <path d="M22 36 Q46 0 70 36" stroke="#8a5a2b" stroke-width="3" fill="none"/></svg>`,

  history: () => `<svg viewBox="0 0 92 70"><ellipse cx="46" cy="64" rx="36" ry="4" style="fill:${V('chip')}"/>
    <rect x="14" y="26" width="64" height="34" rx="4" style="fill:${V('wood')}"/><rect x="18" y="30" width="56" height="26" rx="3" style="fill:${V('shelf')}"/>
    <rect x="40" y="40" width="12" height="4" rx="2" style="fill:${V('brass2')}"/>
    <g transform="rotate(-6 46 20)"><rect x="24" y="8" width="44" height="26" rx="2" fill="#fffaf0" stroke="#d8c8a8"/><path d="M29 15h26M29 20h34M29 25h20" stroke="#c8b38a" stroke-width="1.6"/><circle cx="62" cy="14" r="3" fill="#d4473b"/></g></svg>`,

  welcome: () => `<svg viewBox="0 0 180 130"><ellipse cx="90" cy="122" rx="80" ry="6" style="fill:${V('chip')}"/>
    <path d="M18 46 L90 14 L162 46 Z" style="fill:${V('green')}"/><rect x="24" y="46" width="132" height="8" style="fill:${V('brass')}"/>
    <g style="fill:${V('panel')};stroke:${V('stroke2')}"><rect x="32" y="54" width="12" height="56"/><rect x="58" y="54" width="12" height="56"/><rect x="110" y="54" width="12" height="56"/><rect x="136" y="54" width="12" height="56"/></g>
    <rect x="76" y="70" width="28" height="40" rx="14" style="fill:${V('wood')}"/><circle cx="98" cy="92" r="1.6" style="fill:${V('brass2')}"/>
    <rect x="18" y="110" width="144" height="6" style="fill:${V('shelf')}"/><rect x="12" y="116" width="156" height="5" style="fill:${V('wood2')}"/>
    <text x="90" y="40" text-anchor="middle" style="font: 13px ${V('serif')}; fill:#fff; letter-spacing:2px">LIBROAD</text>
    <g style="fill:${V('brass2')}"><path d="M160 14 l1.6 4 4 1.6 -4 1.6 -1.6 4 -1.6 -4 -4 -1.6 4 -1.6z"/></g></svg>`,

  folder: () => `<svg viewBox="0 0 30 26"><path d="M2 5 a2 2 0 0 1 2-2 h7 l3 3 h12 a2 2 0 0 1 2 2 v14 a2 2 0 0 1-2 2 h-22 a2 2 0 0 1-2-2z" style="fill:${V('brass')}"/><path d="M2 9 h26 v13 a2 2 0 0 1-2 2 h-22 a2 2 0 0 1-2-2z" style="fill:${V('brass2')}"/></svg>`,
  root: () => `<svg viewBox="0 0 18 18"><rect x="2" y="3" width="3.2" height="12" rx=".8" fill="#2f7fd6"/><rect x="5.8" y="5" width="3.2" height="10" rx=".8" fill="#d4473b"/><rect x="9.6" y="4" width="3.2" height="11" rx=".8" fill="#1f8a4c"/><rect x="13" y="5.5" width="3" height="9.5" rx=".8" transform="rotate(10 14.5 15)" fill="#b0822f"/><rect x="1" y="15" width="16" height="1.6" rx=".8" fill="#8a5a2b"/></svg>`,
};

// 표지 제목 줄 나누기: 한글은 1칸, 영문 · 숫자는 반 칸 정도로 셈
function coverLines(t, cap, maxLines) {
  const w = (ch) => (/[\u1100-\u11ff\u3130-\u318f\uac00-\ud7a3\u4e00-\u9fff]/.test(ch) ? 1 : /[A-Z0-9MW]/.test(ch) ? 0.62 : 0.52);
  const tokens = t.split(/(?<=[\s_\-.·])/);
  const lines = [];
  let cur = '', cw = 0;
  const push = () => { if (cur.trim()) lines.push(cur.trim()); cur = ''; cw = 0; };
  for (const tok of tokens) {
    const tw = [...tok].reduce((a, c) => a + w(c), 0);
    if (cw + tw <= cap) { cur += tok; cw += tw; continue; }
    if (tw <= cap) { push(); cur = tok; cw = tw; continue; }
    for (const ch of tok) { if (cw + w(ch) > cap) push(); cur += ch; cw += w(ch); }
  }
  push();
  if (lines.length > maxLines) { lines.length = maxLines; lines[maxLines - 1] = lines[maxLines - 1].slice(0, -1) + '…'; }
  return lines;
}

// 책 표지 (미리 보기 그림이 없을 때)
function coverSVG(kind, ext, name) {
  const k = (window.KINDS || {})[kind] || { color: '#7a7066', short: (ext || '?').toUpperCase() };
  const c = k.color;
  const label = (ext || k.short || '').toUpperCase().slice(0, 5);
  const lines = coverLines(String(name || '').replace(/\.[^.]+$/, ''), 6.6, 3);
  const esc2 = (s) => s.replace(/[&<>]/g, (m) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;' }[m]));
  return `<svg viewBox="0 0 120 156" preserveAspectRatio="xMidYMid slice" xmlns="http://www.w3.org/2000/svg">
    <defs><linearGradient id="cg${kind}" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="#fff" stop-opacity=".22"/><stop offset="1" stop-color="#000" stop-opacity=".18"/></linearGradient></defs>
    <rect width="120" height="156" fill="${c}"/><rect width="120" height="156" fill="url(#cg${kind})"/>
    <rect x="0" y="0" width="11" height="156" fill="#000" opacity=".18"/><rect x="11" y="0" width="1.5" height="156" fill="#fff" opacity=".25"/>
    <rect x="20" y="12" width="90" height="2" fill="#f3d79a" opacity=".85"/><rect x="20" y="142" width="90" height="2" fill="#f3d79a" opacity=".85"/>
    <rect x="22" y="36" width="86" height="${18 + lines.length * 15}" rx="3" fill="#fffaf0" opacity=".95"/>
    ${lines.map((l, i) => `<text x="65" y="${54 + i * 15}" text-anchor="middle" font-family="Pretendard, sans-serif" font-weight="800" font-size="12.5" fill="#2a241c">${esc2(l)}</text>`).join('')}
    <text x="65" y="128" text-anchor="middle" font-family="'DM Serif Display', Georgia, serif" font-size="20" fill="#fff" letter-spacing="1.5">${esc2(label)}</text>
  </svg>`;
}

function paintArt(root = document) {
  root.querySelectorAll('[data-art]').forEach((el) => {
    const f = ART[el.dataset.art];
    if (f && !el.dataset.painted) { el.innerHTML = f(); el.dataset.painted = '1'; }
  });
}
window.ART = ART;
window.coverSVG = coverSVG;
window.paintArt = paintArt;
