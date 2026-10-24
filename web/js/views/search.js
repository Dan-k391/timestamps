// Search across window titles, apps, files, websites, tags and notes.
import { h, clear, icon, empty, select, toast, kindIcon } from '../ui.js';
import { t } from '../i18n.js';
import { dur, clock, fmtDate, today, addDays } from '../fmt.js';
import * as api from '../api.js';
import { go } from '../router.js';

export function mount(el, params) {
  el.className = 'view search-view';
  let range = params.r || 'all';
  const input = h('input', { class: 'input big', placeholder: t('Search window titles, apps, files, websites, tags and notes…'), value: params.q || '', autofocus: true, spellcheck: 'false' });
  const results = h('div', { class: 'search-results' });
  const summary = h('div', { class: 'muted search-summary' });
  el.append(h('header', { class: 'search-head' },
    h('div', { class: 'search-box' }, icon('search', 18), input),
    select([['7', t('Last 7 days')], ['30', t('Last 30 days')], ['365', t('Last 12 months')], ['all', t('All time')]], range, (v) => { range = v; run(); })),
  summary, results);

  let seq = 0;
  async function run() {
    const q = input.value.trim();
    history.replaceState(null, '', '#/search?' + new URLSearchParams({ q, r: range }).toString());
    clear(results);
    summary.textContent = '';
    if (!q) {
      results.append(empty('search', t('Find when you worked on something'), t('Search finds every day where a window title, app, file, website, tag or note matches.')));
      return;
    }
    const my = ++seq;
    const from = range === 'all' ? '2000-01-01' : addDays(today(), -(+range - 1));
    summary.textContent = t('Searching…');
    let r;
    try { r = await api.get('search', { q, from, to: today() }); } catch (err) { toast(err.message, { type: 'error' }); return; }
    if (my !== seq) return;
    summary.textContent = r.days ? t('{0} days · {1} total', r.days, dur(r.total)) : '';
    if (!r.hits.length) { results.append(empty('search', t('No matches'), t('Try a shorter or different word.'))); return; }
    const max = Math.max(...r.hits.map((x) => x.secs));
    for (const hit of r.hits.slice(0, 500)) {
      results.append(h('button', { class: 'hit', type: 'button', onClick: () => go(`day/${hit.date}?filter=${encodeURIComponent(q)}`) },
        h('div', { class: 'hit-date' }, h('b', null, fmtDate(hit.date, { day: 'numeric', month: 'short' })), h('span', { class: 'muted' }, fmtDate(hit.date, { weekday: 'short', year: 'numeric' }))),
        h('div', { class: 'hit-main' },
          h('div', { class: 'hit-items' }, hit.items.slice(0, 5).map((it) => h('span', { class: 'hit-item', title: it.name },
            it.kind === 3 ? icon('tag', 13) : it.kind === 2 ? kindIcon(/^https?:/.test(it.name) ? 2 : 1, it.name, 14) : icon('app', 13),
            h('span', null, it.name), h('small', null, dur(it.secs))))),
          h('div', { class: 'bar' }, h('div', { class: 'bar-fill', style: { width: Math.max(2, (hit.secs / max) * 100) + '%' } }))),
        h('div', { class: 'hit-dur' }, h('b', null, dur(hit.secs)), h('span', { class: 'muted' }, t('from {0}', clock(hit.first))))));
    }
  }
  let tm;
  input.addEventListener('input', () => { clearTimeout(tm); tm = setTimeout(run, 300); });
  input.addEventListener('keydown', (e) => { if (e.key === 'Enter') { clearTimeout(tm); run(); } });
  run();
  return {};
}
