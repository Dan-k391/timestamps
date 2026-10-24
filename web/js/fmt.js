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

// Deterministic colours for keys without a stored colour.
const colorCache = new Map();
export function hashColor(key) {
  let c = colorCache.get(key);
  if (c) return c;
  let h = 2166136261;
  for (let i = 0; i < key.length; i++) { h ^= key.charCodeAt(i); h = Math.imul(h, 16777619); }
  h >>>= 0;
  c = `hsl(${h % 360} ${55 + (h >> 9) % 20}% ${50 + (h >> 17) % 12}%)`;
  colorCache.set(key, c);
  return c;
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
