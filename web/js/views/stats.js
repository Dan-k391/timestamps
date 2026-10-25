// Statistics: totals, activity by day, heatmap, top lists and drill-down.
import { h, clear, icon, button, appIcon, kindIcon, empty, dropdown, toast } from '../ui.js';
import { t } from '../i18n.js';
import { dur, clock, hex, hashColor, mutedColor, fmtDate, fmtDateLong, pct, weekStart, hourLabel, today, shortURL, dirName, daysBetween, parseKey, dateKey } from '../fmt.js';
import * as api from '../api.js';
import { columns, heatmap } from '../charts.js';
import { rangeBar, resolve } from '../range.js';
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
    body.classList.add('loading');
    try {
      data = await api.get('stats', { from, to });
    } catch (err) { toast(err.message, { type: 'error' }); return; } finally { body.classList.remove('loading'); }
    draw(from, to);
  }

  function draw(from, to) {
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

    const tile = (label, value, sub, cls = '') => h('div', { class: 'tile ' + cls }, h('div', { class: 'tile-label' }, label), h('div', { class: 'tile-value' }, value), sub ? h('div', { class: 'tile-sub' }, sub) : null);
    body.append(h('div', { class: 'tiles' },
      tile(t('Active'), dur(d.active), t('{0} days with activity', activeDays.length), 't-active'),
      tile(t('Average per day'), dur(d.active / n), t('on active days')),
      tile(t('Away'), dur(d.away), null, 't-away'),
      tile(t('Tagged'), dur(d.tagged), d.active ? pct(d.tagged, d.active) + ' ' + t('of active time') : null, 't-tag'),
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

  function bucketize(days) {
    if (days.length <= 62) {
      return days.map((x) => ({ label: days.length > 14 ? String(parseKey(x.date).getDate()) : fmtDate(x.date, { weekday: 'short', day: 'numeric' }), tip: fmtDateLong(x.date), date: x.date, active: x.active, away: x.away }));
    }
    const byMonth = days.length > 400;
    const out = [];
    const map = new Map();
    for (const x of days) {
      const k = byMonth ? x.date.slice(0, 7) : weekKey(x.date);
      let b = map.get(k);
      if (!b) {
        b = { label: byMonth ? fmtDate(x.date, { month: 'short', year: '2-digit' }) : fmtDate(x.date, { month: 'short', day: 'numeric' }), tip: byMonth ? fmtDate(x.date, { month: 'long', year: 'numeric' }) : t('Week of {0}', fmtDate(x.date, { month: 'short', day: 'numeric', year: 'numeric' })), active: 0, away: 0 };
        map.set(k, b);
        out.push(b);
      }
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

  async function detail(kind, title, q, from, to, lead) {
    drawer.hidden = false;
    clear(drawer);
    drawer.append(h('div', { class: 'drawer-head' }, lead, h('h3', null, title), h('div', { class: 'spacer' }), button('', { icon: 'x', variant: 'ghost', onClick: () => (drawer.hidden = true) })),
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

function fmtSec(s) {
  if (s >= 86400 - 30) return '24:00';
  const d = new Date();
  d.setHours(0, 0, 0, 0);
  return clock(d.getTime() / 1000 + s);
}
