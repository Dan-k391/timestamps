// Statistics: totals (against the period before), activity by day, heatmap,
// focus sessions, weekly rhythm, top applications over time, top lists and
// drill-down.
import { h, clear, icon, button, appIcon, kindIcon, empty, dropdown, toast } from '../ui.js';
import { t } from '../i18n.js';
import { dur, clock, hex, hashColor, mutedColor, fmtDate, fmtDateLong, fmtRange, pct, weekStart, hourLabel, today, addDays, shortURL, dirName, daysBetween, parseKey, dateKey } from '../fmt.js';
import * as api from '../api.js';
import { columns, heatmap } from '../charts.js';
import { rangeBar, resolve, shift } from '../range.js';
import { go } from '../router.js';

function cssVar(n) { return getComputedStyle(document.documentElement).getPropertyValue(n).trim(); }

export function mount(el, params) {
  el.className = 'view stats-view';
  let st = { p: params.p || 'week', d: params.d || today(), from: params.from, to: params.to };
  let data = null;
  const head = h('header', { class: 'toolbar' });
  const body = h('div', { class: 'stats-body' });
  const drawer = h('aside', { class: 'drawer', hidden: true });
  el.append(head, body, drawer);

  function setState(next) {
    st = next;
    const q = new URLSearchParams({ p: st.p, d: st.d || '' });
    if (st.p === 'custom') { q.set('from', st.from || ''); q.set('to', st.to || ''); }
    history.replaceState(null, '', '#/stats?' + q.toString());
    render();
  }

  async function render() {
    clear(head);
    head.append(rangeBar(st, setState, ['day', 'week', 'month', 'year', 'last30', 'all', 'custom']), h('div', { class: 'spacer' }),
      button(t('Export'), { icon: 'download', onClick: (e) => exportMenu(e.currentTarget) }));
    const [from, to] = resolve(st.p, st.d, st.from, st.to);
    const [pfrom, pto] = previous(st, from, to);
    body.classList.add('loading');
    try {
      data = await api.get('stats', pfrom ? { from, to, pfrom, pto } : { from, to });
    } catch (err) { toast(err.message, { type: 'error' }); return; } finally { body.classList.remove('loading'); }
    draw(from, to, pfrom && fmtRange(pfrom, pto));
  }

  function draw(from, to, prevLabel) {
    clear(body);
    const d = data;
    let days = d.days;
    if (st.p === 'all') {
      const firstIdx = days.findIndex((x) => x.active > 0);
      days = firstIdx >= 0 ? days.slice(firstIdx) : [];
    }
    days = days.filter((x) => x.date <= today());
    const activeDays = days.filter((x) => x.active > 0);
    const n = activeDays.length || 1;
    const dayStart = (k) => parseKey(k).getTime() / 1000;
    const avgStart = activeDays.length ? activeDays.reduce((s, x) => s + (x.first - dayStart(x.date)), 0) / activeDays.length : 0;
    const avgEnd = activeDays.length ? activeDays.reduce((s, x) => s + (x.last - dayStart(x.date)), 0) / activeDays.length : 0;

    const tile = (label, value, sub, cls = '', change = null) => h('div', { class: 'tile ' + cls }, h('div', { class: 'tile-label' }, label), h('div', { class: 'tile-value' }, value, change), sub ? h('div', { class: 'tile-sub' }, sub) : null);
    // The change from the period before; its range and value in the tooltip.
    const p = d.prev;
    const delta = (cur, old) => {
      if (!p || !old) return null;
      const c = Math.round(((cur - old) / old) * 100);
      return h('span', { class: 'delta' + (c > 0 ? ' up' : c < 0 ? ' down' : ''), title: prevLabel + ' · ' + dur(old) }, icon(c >= 0 ? 'up' : 'down', 11), Math.abs(c) + '%');
    };
    body.append(h('div', { class: 'tiles' },
      tile(t('Active'), dur(d.active), t('{0} days with activity', activeDays.length), 't-active', delta(d.active, p && p.active)),
      tile(t('Average per day'), dur(d.active / n), t('on active days'), '', delta(d.active / n, p && p.days && p.active / p.days)),
      tile(t('Away'), dur(d.away), null, 't-away'),
      tile(t('Tagged'), dur(d.tagged), d.active ? pct(d.tagged, d.active) + ' ' + t('of active time') : null, 't-tag', delta(d.tagged, p && p.tagged)),
      tile(t('Average start'), activeDays.length ? fmtSec(avgStart) : '—', activeDays.length ? t('Average end') + ' ' + fmtSec(avgEnd) : null)));

    if (!d.active && !d.away) {
      body.append(h('section', { class: 'card' }, empty('stats', t('No data for this period'), t('Pick another range, or import your ManicTime history in Settings → Data.'))));
      return;
    }

    // Activity chart: by day, or by week/month for long ranges.
    const buckets = bucketize(days);
    const C = { active: cssVar('--active'), away: cssVar('--away-strong'), tag: cssVar('--accent') };
    const chart = columns({
      labels: buckets.map((b) => b.label),
      series: [
        { name: t('Active'), color: C.active, values: buckets.map((b) => b.active) },
        { name: t('Away'), color: C.away, values: buckets.map((b) => b.away) },
      ],
      fmt: (v) => dur(v, 'hm'),
      tipLabel: (i) => buckets[i].tip,
      onClick: (i) => { if (buckets[i].date) go('day/' + buckets[i].date); },
      height: 200,
    });
    const dayNames = [];
    for (let i = 0; i < 7; i++) dayNames.push(fmtDate(dateKey(new Date(2024, 0, 1 + i)), { weekday: 'short' }));
    body.append(h('div', { class: 'grid-2-1' },
      h('section', { class: 'card' }, h('div', { class: 'card-head' }, icon('activity', 16), h('h3', null, t('Activity')), h('div', { class: 'spacer' }),
        h('span', { class: 'legend' }, h('span', { class: 'dot', style: { background: C.active } }), t('Active'), h('span', { class: 'dot', style: { background: C.away } }), t('Away'))), chart),
      h('section', { class: 'card' }, h('div', { class: 'card-head' }, icon('clock', 16), h('h3', null, t('When you work'))),
        heatmap(d.heat, { days: dayNames, fmt: (v) => dur(v), weekStart: weekStart(), hourLabel }))));

    body.append(h('div', { class: 'grid-2' }, focusCard(d), rhythmCard(d, days, dayNames)));
    const top = topApps(d, buckets);
    if (top) body.append(top);

    // Top lists.
    const listCard = (title, ic, items, render, kind) => {
      const total = items.reduce((s, x) => s + x.secs, 0);
      const max = items.length ? items[0].secs : 1;
      const limit = 25;
      const list = h('div', { class: 'list' });
      const fill = (all) => {
        clear(list);
        if (!items.length) list.append(empty(ic, t('Nothing here')));
        (all ? items : items.slice(0, limit)).forEach((it) => list.append(render(it, max, total)));
        if (!all && items.length > limit) list.append(h('button', { class: 'more-btn', type: 'button', onClick: () => fill(true) }, t('Show all ({0})', items.length)));
      };
      fill(false);
      return h('section', { class: 'card list-card' }, h('div', { class: 'card-head' }, icon(ic, 16), h('h3', null, title), h('div', { class: 'spacer' }), h('span', { class: 'muted' }, dur(total))), list);
    };
    const row = (lead, name, sub, secs, max, total, color, onClick) => h('div', { class: 'row' },
      h('button', { class: 'row-main', type: 'button', onClick },
        lead, h('span', { class: 'row-name' }, h('span', null, name), sub ? h('small', { class: 'mono' }, sub) : null),
        h('div', { class: 'bar' }, h('div', { class: 'bar-fill', style: { width: Math.max(1.5, (secs / max) * 100) + '%', background: color } })),
        h('span', { class: 'row-pct' }, pct(secs, total)), h('span', { class: 'row-dur' }, dur(secs))));

    body.append(h('div', { class: 'grid-2' },
      listCard(t('Applications'), 'app', d.apps, (a, max, total) => row(appIcon({ id: a.id, icon: a.icon, iconV: a.iconV, name: a.name, color: a.color }, 18), a.name, null, a.secs, max, total, hex(a.color),
        () => detail('app', a.name, { id: a.id }, from, to, appIcon({ id: a.id, icon: a.icon, iconV: a.iconV, name: a.name, color: a.color }, 22)))),
      listCard(t('Websites'), 'globe', d.sites, (s, max, total) => row(kindIcon(2, s.key, 18), s.key, null, s.secs, max, total, s.color ? hex(s.color) : mutedColor(s.key),
        () => detail('site', s.key, { key: s.key }, from, to, kindIcon(2, s.key, 22)))),
      listCard(t('Files'), 'file', d.files, (f, max, total) => row(kindIcon(1, f.key, 18), f.name, dirName(f.key), f.secs, max, total, hashColor(f.name),
        () => detail('file', f.name, { id: f.id, path: f.key }, from, to, kindIcon(1, f.key, 22)))),
      listCard(t('Tags'), 'tag', d.tags, (tg, max, total) => row(h('span', { class: 'dot lg', style: { background: hex(tg.color) } }), tg.name, null, tg.secs, max, total, hex(tg.color),
        () => detail('tag', tg.name, { key: tg.key }, from, to, h('span', { class: 'dot lg', style: { background: hex(tg.color) } }))))));

    // Attendance.
    const att = h('table', { class: 'table' },
      h('thead', null, h('tr', null, [t('Date'), t('Start'), t('End'), t('Day length'), t('Active'), t('Away'), t('Tagged')].map((x) => h('th', null, x)))),
      h('tbody', null, days.slice().reverse().filter((x) => x.active || x.away).slice(0, 400).map((x) => h('tr', { class: 'click', onClick: () => go('day/' + x.date) },
        h('td', null, fmtDate(x.date, { weekday: 'short', year: 'numeric', month: 'short', day: 'numeric' })),
        h('td', null, x.first ? clock(x.first) : '—'), h('td', null, x.last ? fmtSec(x.last - parseKey(x.date).getTime() / 1000) : '—'),
        h('td', null, x.first ? dur(x.last - x.first) : '—'), h('td', null, dur(x.active)), h('td', null, dur(x.away)), h('td', null, dur(x.tagged))))));
    body.append(h('section', { class: 'card' }, h('div', { class: 'card-head' }, icon('day', 16), h('h3', null, t('Attendance'))), h('div', { class: 'table-wrap' }, att)));
  }

  // Focus: how the active time breaks into sessions, and how often the
  // application changes.
  function focusCard(d) {
    const f = d.focus;
    const hours = d.active / 3600;
    const [ls, le] = f.longest;
    const lsDay = ls ? dateKey(new Date(ls * 1000)) : '';
    const names = ['< 15m', '15–30m', '30m–1h', '1–2h', '2h+'];
    const most = Math.max(1, ...f.lengths);
    return h('section', { class: 'card' }, h('div', { class: 'card-head' }, icon('timer', 16), h('h3', null, t('Focus'))),
      h('div', { class: 'figs' },
        fig(t('Sessions'), String(f.sessions)),
        fig(t('Average session'), f.sessions ? dur(d.active / f.sessions) : '—'),
        fig(t('Longest session'), ls ? dur(le - ls) : '—', ls ? { title: fmtDateLong(lsDay) + ' · ' + clock(ls) + '–' + clock(le), onClick: () => go('day/' + lsDay) } : {}),
        fig(t('Switches per hour'), hours >= 0.1 ? (f.switches / hours).toFixed(1) : '—')),
      h('div', { class: 'lengths' }, f.lengths.map((c, i) => h('div', { class: 'len-row' },
        h('span', { class: 'len-name' }, names[i]),
        h('div', { class: 'bar' }, h('div', { class: 'bar-fill', style: { width: (c ? Math.max(2, (c / most) * 100) : 0) + '%', background: 'var(--accent)', opacity: 0.35 + i * 0.16 } })),
        h('span', { class: 'len-count' }, String(c))))));
  }

  // Rhythm: the average day of the week (or, for less than a week, the hours
  // of the day), and when activity peaks.
  function rhythmCard(d, days, dayNames) {
    const C = cssVar('--accent');
    const byHour = Array.from({ length: 24 }, (_, hr) => d.heat.reduce((n, row) => n + row[hr], 0));
    const peak = byHour.indexOf(Math.max(...byHour));
    const figs = [fig(t('Peak hour'), d.active ? hourLabel(peak) + '–' + hourLabel(peak + 1) : '—')];
    let chart;
    if (days.length >= 7) {
      const order = weekStart() === 0 ? [6, 0, 1, 2, 3, 4, 5] : [0, 1, 2, 3, 4, 5, 6];
      const sum = Array(7).fill(0), count = Array(7).fill(0);
      for (const x of days) {
        const wd = (parseKey(x.date).getDay() + 6) % 7; // Monday = 0
        sum[wd] += x.active;
        count[wd]++;
      }
      const avg = sum.map((v, i) => (count[i] ? v / count[i] : 0));
      figs.push(fig(t('Busiest day'), d.active ? dayNames[avg.indexOf(Math.max(...avg))] : '—'), fig(t('Weekend'), pct(sum[5] + sum[6], d.active)));
      chart = columns({ labels: order.map((i) => dayNames[i]), series: [{ name: t('Average per day'), color: C, values: order.map((i) => avg[i]) }], fmt: (v) => dur(v, 'hm'), height: 150 });
    } else {
      chart = columns({ labels: byHour.map((_, hr) => String(hr)), series: [{ name: t('Active'), color: C, values: byHour }], fmt: (v) => dur(v, 'hm'), tipLabel: (i) => hourLabel(i) + '–' + hourLabel(i + 1), height: 150 });
    }
    return h('section', { class: 'card' }, h('div', { class: 'card-head' }, icon('activity', 16), h('h3', null, t('Rhythm'))), h('div', { class: 'figs' }, figs), chart);
  }

  // The top applications over the range, by day (or week, or month).
  function topApps(d, buckets) {
    if (!d.trend.length || buckets.length < 2) return null;
    const at = new Map(d.days.map((x, i) => [x.date, i]));
    const apps = d.trend.map((s) => ({ s, app: d.apps.find((a) => a.id === s.id) })).filter((x) => x.app);
    const sumOf = (vals, b) => b.dates.reduce((n, k) => n + (vals[at.get(k)] || 0), 0);
    const series = apps.map(({ s, app }) => ({ name: app.name, color: hex(app.color), values: buckets.map((b) => sumOf(s.days, b)) }));
    const other = cssVar('--border-2');
    series.push({ name: t('Other'), color: other, values: buckets.map((b, i) => Math.max(0, b.active - series.reduce((n, x) => n + x.values[i], 0))) });
    const chart = columns({ labels: buckets.map((b) => b.label), series, fmt: (v) => dur(v, 'hm'), tipLabel: (i) => buckets[i].tip, onClick: (i) => { if (buckets[i].date) go('day/' + buckets[i].date); }, height: 200 });
    const legend = h('span', { class: 'legend' }, apps.map(({ app }) => h('span', { class: 'legend-app' }, appIcon({ id: app.id, icon: app.icon, iconV: app.iconV, name: app.name, color: app.color }, 14), app.name)),
      h('span', { class: 'legend-app' }, h('span', { class: 'dot', style: { background: other } }), t('Other')));
    return h('section', { class: 'card' }, h('div', { class: 'card-head' }, icon('layers', 16), h('h3', null, t('Top applications')), h('div', { class: 'spacer' }), legend), chart);
  }

  function bucketize(days) {
    if (days.length <= 62) {
      return days.map((x) => ({ label: days.length > 14 ? String(parseKey(x.date).getDate()) : fmtDate(x.date, { weekday: 'short', day: 'numeric' }), tip: fmtDateLong(x.date), date: x.date, dates: [x.date], active: x.active, away: x.away }));
    }
    const byMonth = days.length > 400;
    const out = [];
    const map = new Map();
    for (const x of days) {
      const k = byMonth ? x.date.slice(0, 7) : weekKey(x.date);
      let b = map.get(k);
      if (!b) {
        b = { label: byMonth ? fmtDate(x.date, { month: 'short', year: '2-digit' }) : fmtDate(x.date, { month: 'short', day: 'numeric' }), tip: byMonth ? fmtDate(x.date, { month: 'long', year: 'numeric' }) : t('Week of {0}', fmtDate(x.date, { month: 'short', day: 'numeric', year: 'numeric' })), dates: [], active: 0, away: 0 };
        map.set(k, b);
        out.push(b);
      }
      b.dates.push(x.date);
      b.active += x.active;
      b.away += x.away;
    }
    return out;
  }

  function weekKey(k) {
    const d = parseKey(k);
    const diff = (d.getDay() - weekStart() + 7) % 7;
    d.setDate(d.getDate() - diff);
    return dateKey(d);
  }

  // The drawer slides out before it hides.
  let drawerTimer = 0;
  function closeDrawer() {
    drawer.classList.add('closing');
    drawerTimer = setTimeout(() => { drawer.hidden = true; drawer.classList.remove('closing'); }, 160);
  }

  async function detail(kind, title, q, from, to, lead) {
    clearTimeout(drawerTimer);
    drawer.classList.remove('closing');
    drawer.hidden = false;
    clear(drawer);
    drawer.append(h('div', { class: 'drawer-head' }, lead, h('h3', null, title), h('div', { class: 'spacer' }), button('', { icon: 'x', variant: 'ghost', onClick: closeDrawer })),
      q.path ? h('div', { class: 'drawer-path mono' }, q.path, button('', { icon: 'folder', small: true, variant: 'ghost', title: t('Show in folder'), onClick: () => api.post('reveal', { path: q.path }).catch(() => toast(t('File not found'), { type: 'error' })) })) : null,
      h('div', { class: 'drawer-body muted' }, t('Loading…')));
    const r = await api.get('detail', { kind, from, to, id: q.id, key: q.key });
    const b = drawer.querySelector('.drawer-body');
    clear(b);
    b.classList.remove('muted');
    const days = r.days.filter((x) => x.active > 0);
    b.append(h('div', { class: 'drawer-total' }, h('b', null, dur(r.total)), h('span', { class: 'muted' }, t('{0} days', days.length))));
    if (r.days.length > 1 && r.days.length <= 62) {
      b.append(columns({ labels: r.days.map((x) => String(parseKey(x.date).getDate())), series: [{ name: title, color: cssVar('--accent'), values: r.days.map((x) => x.active) }], fmt: (v) => dur(v, 'hm'), tipLabel: (i) => fmtDateLong(r.days[i].date), onClick: (i) => go('day/' + r.days[i].date), height: 120 }));
    }
    const itemsTitle = { app: t('Window titles'), site: t('Pages'), file: t('Applications'), tag: t('Notes') }[kind];
    b.append(h('h4', null, itemsTitle));
    const list = h('div', { class: 'list' });
    const max = r.items.length ? r.items[0].secs : 1;
    r.items.slice(0, 300).forEach((it) => list.append(h('div', { class: 'sub-row', title: it.name },
      h('span', { class: 'row-name' }, it.name ? (kind === 'site' ? shortURL(it.name) : it.name) : t('(none)')),
      kind === 'site' ? button('', { icon: 'external', small: true, variant: 'ghost', onClick: () => api.post('open', { target: it.name }) }) : null,
      h('div', { class: 'bar sm' }, h('div', { class: 'bar-fill', style: { width: (it.secs / max) * 100 + '%' } })),
      h('span', { class: 'row-dur' }, dur(it.secs)))));
    b.append(list);
  }

  function exportMenu(anchor) {
    const [from, to] = resolve(st.p, st.d, st.from, st.to);
    const table = (name, header, rows) => (fmt) => api.download('export/table', null, { name: `timestamps-${name}-${from}_${to}`, format: fmt, rows: [header, ...rows] });
    const hrs = (s) => (s / 3600).toFixed(2);
    const sets = {
      [t('Applications')]: table('applications', ['Application', 'Process', 'Hours', 'Duration'], data.apps.map((a) => [a.name, a.key, hrs(a.secs), dur(a.secs, 'clock')])),
      [t('Websites')]: table('websites', ['Website', 'Hours', 'Duration'], data.sites.map((a) => [a.key, hrs(a.secs), dur(a.secs, 'clock')])),
      [t('Files')]: table('files', ['File', 'Path', 'Hours', 'Duration'], data.files.map((a) => [a.name, a.key, hrs(a.secs), dur(a.secs, 'clock')])),
      [t('Tags')]: table('tags', ['Tags', 'Hours', 'Duration'], data.tags.map((a) => [a.name, hrs(a.secs), dur(a.secs, 'clock')])),
      [t('Attendance')]: table('attendance', ['Date', 'Start', 'End', 'Active hours', 'Away hours', 'Tagged hours'], data.days.filter((x) => x.active).map((x) => [x.date, clock(x.first), clock(x.last), hrs(x.active), hrs(x.away), hrs(x.tagged)])),
    };
    const items = [];
    for (const [name, fn] of Object.entries(sets)) {
      items.push({ label: name + ' · CSV', icon: 'download', onClick: () => fn('csv') }, { label: name + ' · Excel', icon: 'sheet', onClick: () => fn('xlsx') });
    }
    dropdown(anchor, items);
  }

  render();
  return { refresh: (ev) => { if (ev && (ev.type === 'tags' || ev.type === 'settings')) render(); } };
}

// A figure: a number with its name under it; with onClick, a button.
function fig(label, value, opts = {}) {
  return h(opts.onClick ? 'button' : 'div', { class: 'fig' + (opts.onClick ? ' click' : ''), type: opts.onClick ? 'button' : null, title: opts.title, onClick: opts.onClick },
    h('span', { class: 'fig-value' }, value), h('span', { class: 'fig-label' }, label));
}

// The period to compare with: the one before, cut to as many days as this
// one has had so far (this week so far against the same days last week).
function previous(s, from, to) {
  if (s.p === 'all') return [];
  if (['day', 'week', 'month', 'year'].includes(s.p)) {
    const [pf, pt] = resolve(s.p, shift(s.p, from, -1));
    const cut = addDays(pf, daysBetween(from, to > today() ? today() : to));
    return [pf, cut < pt ? cut : pt];
  }
  const span = daysBetween(from, to) + 1;
  return [addDays(from, -span), addDays(from, -1)];
}

function fmtSec(s) {
  if (s >= 86400 - 30) return '24:00';
  const d = new Date();
  d.setHours(0, 0, 0, 0);
  return clock(d.getTime() / 1000 + s);
}
