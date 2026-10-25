// Formatting helpers for durations, times, dates and colours.
import { locale } from './i18n.js';

const cfg = { dur: 'hm', clock24: true, weekStart: 1 };

export function configure(s) {
  cfg.dur = s.durationFormat || 'hm';
  cfg.clock24 = s.clock24 !== false;
  cfg.weekStart = s.weekStart ?? 1;
}

export const weekStart = () => cfg.weekStart;

export function dur(secs, fmt = cfg.dur) {
  secs = Math.max(0, Math.round(secs || 0));
  const h = Math.floor(secs / 3600), m = Math.floor(secs / 60) % 60, s = secs % 60;
  if (fmt === 'clock') return `${h}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`;
  if (fmt === 'decimal') return (secs / 3600).toFixed(2) + ' h';
  if (h) return `${h}h ${String(m).padStart(2, '0')}m`;
  if (m) return `${m}m${s && m < 10 ? ' ' + String(s).padStart(2, '0') + 's' : ''}`;
  return `${s}s`;
}

/** Long form used in tooltips: always includes seconds. */
export function durLong(secs) {
  secs = Math.max(0, Math.round(secs || 0));
  const h = Math.floor(secs / 3600), m = Math.floor(secs / 60) % 60, s = secs % 60;
  if (h) return `${h}h ${String(m).padStart(2, '0')}m ${String(s).padStart(2, '0')}s`;
  if (m) return `${m}m ${String(s).padStart(2, '0')}s`;
  return `${s}s`;
}

export function hoursDecimal(secs) { return (secs / 3600).toFixed(2); }

const pad = (n) => String(n).padStart(2, '0');

export function clock(ts, seconds = false) {
  const d = new Date(ts * 1000);
  let h = d.getHours();
  const tail = seconds ? ':' + pad(d.getSeconds()) : '';
  if (cfg.clock24) return `${pad(h)}:${pad(d.getMinutes())}${tail}`;
  const ap = h >= 12 ? 'PM' : 'AM';
  h = h % 12 || 12;
  return `${h}:${pad(d.getMinutes())}${tail} ${ap}`;
}

export function hourLabel(h) {
  if (cfg.clock24) return pad(h % 24) + ':00';
  const hh = h % 24;
  return `${hh % 12 || 12} ${hh >= 12 ? 'PM' : 'AM'}`;
}

export function dateKey(d = new Date()) {
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

export function parseKey(k) {
  const [y, m, d] = k.split('-').map(Number);
  return new Date(y, m - 1, d);
}

export function addDays(k, n) {
  const d = parseKey(k);
  d.setDate(d.getDate() + n);
  return dateKey(d);
}

export function today() { return dateKey(new Date()); }

export function startOfWeek(k) {
  const d = parseKey(k);
  const diff = (d.getDay() - cfg.weekStart + 7) % 7;
  d.setDate(d.getDate() - diff);
  return dateKey(d);
}

export function startOfMonth(k) { const d = parseKey(k); d.setDate(1); return dateKey(d); }
export function endOfMonth(k) { const d = parseKey(k); d.setMonth(d.getMonth() + 1, 0); return dateKey(d); }

export function daysBetween(a, b) {
  return Math.round((parseKey(b) - parseKey(a)) / 86400000);
}

export function fmtDate(k, opts = { weekday: 'short', month: 'short', day: 'numeric' }) {
  return parseKey(k).toLocaleDateString(locale(), opts);
}

export function fmtDateLong(k) {
  return parseKey(k).toLocaleDateString(locale(), { weekday: 'long', year: 'numeric', month: 'long', day: 'numeric' });
}

export function fmtRange(a, b) {
  if (a === b) return fmtDate(a, { year: 'numeric', month: 'short', day: 'numeric', weekday: 'short' });
  const sameYear = a.slice(0, 4) === b.slice(0, 4);
  const o = { month: 'short', day: 'numeric' };
  return `${fmtDate(a, sameYear ? o : { ...o, year: 'numeric' })} – ${fmtDate(b, { ...o, year: 'numeric' })}`;
}

export function weekdayShort(k) { return fmtDate(k, { weekday: 'short' }); }

export function bytes(n) {
  if (n < 1024) return n + ' B';
  if (n < 1024 * 1024) return (n / 1024).toFixed(0) + ' KB';
  if (n < 1024 ** 3) return (n / 1024 / 1024).toFixed(1) + ' MB';
  return (n / 1024 ** 3).toFixed(2) + ' GB';
}

export function pct(part, total) {
  if (!total) return '0%';
  const p = (part / total) * 100;
  return (p < 1 && p > 0 ? p.toFixed(1) : Math.round(p)) + '%';
}

// Deterministic colours for keys without a stored colour: one of the color
// theme's ten (--cat-0..9), or a hue from the key in the classic look.
const colorCache = new Map();
let themeColors = null;
export function resetColors() { colorCache.clear(); themeColors = null; }
export function hashColor(key) {
  let c = colorCache.get(key);
  if (c) return c;
  let h = 2166136261;
  for (let i = 0; i < key.length; i++) { h ^= key.charCodeAt(i); h = Math.imul(h, 16777619); }
  h >>>= 0;
  if (!themeColors) {
    const cs = getComputedStyle(document.documentElement);
    themeColors = Array.from({ length: 10 }, (_, i) => cs.getPropertyValue('--cat-' + i).trim()).filter(Boolean);
  }
  c = themeColors.length ? themeColors[h % themeColors.length] : `hsl(${h % 360} ${55 + (h >> 9) % 20}% ${50 + (h >> 17) % 12}%)`;
  colorCache.set(key, c);
  return c;
}

/** A quiet color for things without an icon to take a color from. */
export function mutedColor(key) {
  let h = 2166136261;
  for (let i = 0; i < key.length; i++) { h ^= key.charCodeAt(i); h = Math.imul(h, 16777619); }
  return `hsl(${(h >>> 0) % 360} 16% 58%)`;
}

export const hex = (c) => (c ? (c.startsWith('#') || c.startsWith('hsl') ? c : '#' + c) : '#8b93a7');

export function escapeHTML(s) {
  return String(s ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
}

export function shortURL(u) {
  return u.replace(/^https?:\/\/(www\.)?/, '').replace(/\/$/, '');
}

const BROWSER_SUFFIX = /\s+[-—–]\s+(Google Chrome|Microsoft Edge|Brave|Opera|Vivaldi|Chromium|Mozilla Firefox|Firefox|Waterfox|LibreWolf|Floorp|Zen Browser|360安全浏览器|360极速浏览器|QQ浏览器|搜狗高速浏览器|Yandex|Arc|Thorium)(\s+(Beta|Dev|Canary|Nightly|Developer Edition))?$/;
const MORE_PAGES = /\s+(and \d+ more pages?|和另外\s*\d+\s*个页面|和其他\s*\d+\s*个页面|及其他\s*\d+\s*个页面)(\s+[-—–]\s+.*)?$/;

// Two-label public suffixes and shared hosting domains; kept in sync with
// multiSuffixes in internal/store/domain.go.
const BASE_SUFFIXES = new Set(`com.cn net.cn org.cn gov.cn edu.cn ac.cn mil.cn com.hk net.hk org.hk edu.hk gov.hk com.tw net.tw org.tw
  edu.tw gov.tw com.mo co.uk org.uk ac.uk gov.uk me.uk ltd.uk plc.uk net.uk sch.uk nhs.uk com.au net.au org.au edu.au gov.au co.nz net.nz
  org.nz ac.nz govt.nz co.jp ne.jp or.jp ac.jp go.jp co.kr ne.kr or.kr ac.kr go.kr com.sg edu.sg gov.sg com.my edu.my gov.my co.id ac.id
  go.id or.id co.th ac.th go.th in.th com.vn edu.vn gov.vn com.ph edu.ph gov.ph co.in net.in org.in ac.in gov.in com.br net.br org.br
  gov.br edu.br com.mx org.mx gob.mx edu.mx com.ar gob.ar com.co gov.co co.za org.za ac.za gov.za com.tr edu.tr gov.tr com.ua org.ua
  com.ru org.ru com.sa edu.sa gov.sa co.il ac.il gov.il com.eg edu.eg gov.eg com.pk edu.pk github.io gitlab.io gitee.io vercel.app
  netlify.app pages.dev workers.dev web.app firebaseapp.com herokuapp.com appspot.com blogspot.com azurewebsites.net cloudfront.net
  onrender.com fly.dev repl.co glitch.me surge.sh ngrok.io ngrok-free.app trycloudflare.com`.split(/\s+/));

/** Registrable domain: "t.bilibili.com" -> "bilibili.com", "news.bbc.co.uk" -> "bbc.co.uk". */
export function baseDomain(host) {
  const h = (host || '').toLowerCase().replace(/\.$/, '');
  if (!h.includes('.') || /^[\d.]+$/.test(h) || h.includes(':')) return h;
  const p = h.split('.');
  if (p.length <= 2) return h;
  return BASE_SUFFIXES.has(p.slice(-2).join('.')) ? p.slice(-3).join('.') : p.slice(-2).join('.');
}

/** Window title without the browser name (profile name still attached). */
export function stripBrowser(title) {
  return title.replace(/\u200b/g, '').replace(BROWSER_SUFFIX, '').trim();
}

/** Page (tab) title from a browser window title. */
export function pageTitle(title) {
  return stripBrowser(title).replace(MORE_PAGES, '').trim();
}

export function baseName(p) {
  const i = Math.max(p.lastIndexOf('\\'), p.lastIndexOf('/'));
  return i >= 0 ? p.slice(i + 1) : p;
}

export function dirName(p) {
  const i = Math.max(p.lastIndexOf('\\'), p.lastIndexOf('/'));
  return i > 0 ? p.slice(0, i) : '';
}
