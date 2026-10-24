// Timesheets: tags × days with grouping and rounding, exportable.
import { h, clear, button, select, toggle, empty, dropdown, toast, icon } from '../ui.js';
import { t } from '../i18n.js';
import { dur, hex, fmtDate, today, hoursDecimal } from '../fmt.js';
import * as api from '../api.js';
import { rangeBar, resolve } from '../range.js';
import { go } from '../router.js';

const saved = () => { try { return JSON.parse(localStorage.getItem('ts.timesheet') || '{}'); } catch (_) { return {}; } };

export function mount(el, params) {
  el.className = 'view sheet-view';
  const prefs = { level: 0, round: 0, mode: 'nearest', neverZero: false, billable: false, decimal: true, hideEmpty: true, ...saved() };
  let st = { p: params.p || 'week', d: params.d || today(), from: params.from, to: params.to };
  let data = null;
  const head = h('header', { class: 'toolbar' });
  const opts = h('div', { class: 'toolbar sub' });
  const body = h('section', { class: 'card sheet-card' });
  el.append(head, opts, body);

  const savePrefs = () => { try { localStorage.setItem('ts.timesheet', JSON.stringify(prefs)); } catch (_) { /* ignore */ } };

  function renderOpts() {
    clear(opts);
    opts.append(
      h('label', { class: 'inline' }, t('Group by'), select([[0, t('Full tag')], [1, t('First tag')], [2, t('First two tags')]], prefs.level, (v) => { prefs.level = +v; savePrefs(); load(); })),
      h('label', { class: 'inline' }, t('Rounding'), select([[0, t('None')], [300, '5 min'], [360, '6 min'], [600, '10 min'], [900, '15 min'], [1800, '30 min'], [3600, '1 h']], prefs.round, (v) => { prefs.round = +v; savePrefs(); load(); })),
      prefs.round ? select([['nearest', t('Nearest')], ['up', t('Up')], ['down', t('Down')]], prefs.mode, (v) => { prefs.mode = v; savePrefs(); load(); }) : null,
      prefs.round ? toggle(prefs.neverZero, (v) => { prefs.neverZero = v; savePrefs(); load(); }, t('Never round to zero')) : null,
      toggle(prefs.billable, (v) => { prefs.billable = v; savePrefs(); load(); }, t('Billable only')),
      toggle(prefs.decimal, (v) => { prefs.decimal = v; savePrefs(); draw(); }, t('Decimal hours')),
      toggle(prefs.hideEmpty, (v) => { prefs.hideEmpty = v; savePrefs(); draw(); }, t('Hide empty days')));
  }

  function setState(next) {
    st = next;
    const q = new URLSearchParams({ p: st.p, d: st.d || '' });
    if (st.p === 'custom') { q.set('from', st.from || ''); q.set('to', st.to || ''); }
    history.replaceState(null, '', '#/timesheet?' + q.toString());
    load();
  }

  async function load() {
    clear(head);
    head.append(rangeBar(st, setState, ['week', 'month', 'year', 'custom']), h('div', { class: 'spacer' }),
      button(t('Export'), { icon: 'download', onClick: (e) => dropdown(e.currentTarget, [
        { label: 'CSV', icon: 'download', onClick: () => exportAs('csv') },
        { label: 'Excel (.xlsx)', icon: 'sheet', onClick: () => exportAs('xlsx') },
      ]) }));
    renderOpts();
    const [from, to] = resolve(st.p, st.d, st.from, st.to);
    try {
      data = await api.get('timesheet', { from, to, level: prefs.level, round: prefs.round, mode: prefs.mode, neverZero: prefs.neverZero ? 1 : 0, billable: prefs.billable ? 1 : 0 });
    } catch (err) { toast(err.message, { type: 'error' }); return; }
    draw();
  }

  const fmtCell = (s) => (s ? (prefs.decimal ? hoursDecimal(s) : dur(s, 'clock').replace(/:\d\d$/, '')) : '');

  function columnsToShow() {
    return data.days.map((d, i) => i).filter((i) => !prefs.hideEmpty || data.totals[i] > 0);
  }

  function draw() {
    clear(body);
    if (!data.rows.length) {
      body.append(empty('sheet', t('No tagged time in this period'), t('Tags you add in the day view (or with the stopwatch and autotag rules) appear here.'),
        button(t('Go to today'), { variant: 'primary', onClick: () => go('day/' + today()) })));
      return;
    }
    const cols = columnsToShow();
    const table = h('table', { class: 'table sheet' },
      h('thead', null, h('tr', null, h('th', { class: 'tag-col' }, t('Tag')), cols.map((i) => h('th', { class: 'num', title: data.days[i] }, fmtDate(data.days[i], cols.length > 10 ? { day: 'numeric', month: 'numeric' } : { weekday: 'short', day: 'numeric' }))), h('th', { class: 'num total' }, t('Total')))),
      h('tbody', null, data.rows.map((r) => h('tr', null,
        h('td', { class: 'tag-col', title: r.notes.join('\n') }, h('span', { class: 'dot', style: { background: hex(r.color) } }), r.key),
        cols.map((i) => h('td', { class: 'num' + (r.cells[i] ? '' : ' zero') }, fmtCell(r.cells[i]))),
        h('td', { class: 'num total' }, fmtCell(r.total))))),
      h('tfoot', null, h('tr', null, h('td', { class: 'tag-col' }, t('Total')), cols.map((i) => h('td', { class: 'num' }, fmtCell(data.totals[i]))), h('td', { class: 'num total' }, fmtCell(data.total)))));
    body.append(h('div', { class: 'table-wrap' }, table),
      h('div', { class: 'sheet-foot muted' }, icon('info', 14), prefs.decimal ? t('Values are in hours.') : t('Values are hours:minutes.'), prefs.round ? ' ' + t('Each cell is rounded separately.') : ''));
  }

  function exportAs(fmt) {
    const cols = columnsToShow();
    const header = ['Tag', ...cols.map((i) => data.days[i]), 'Total'];
    const rows = data.rows.map((r) => [r.key, ...cols.map((i) => hoursDecimal(r.cells[i])), hoursDecimal(r.total)]);
    rows.push(['Total', ...cols.map((i) => hoursDecimal(data.totals[i])), hoursDecimal(data.total)]);
    const [from, to] = resolve(st.p, st.d, st.from, st.to);
    api.download('export/table', null, { name: `timesheet-${from}_${to}`, format: fmt, rows: [header, ...rows] });
  }

  load();
  return { refresh: (ev) => { if (ev && ev.type === 'tags') load(); } };
}
