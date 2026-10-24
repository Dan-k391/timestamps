// Tag manager: every tag combination with totals, colour, billable, rename, delete.
import { h, clear, button, toggle, empty, promptDialog, confirmDialog, toast, icon } from '../ui.js';
import { t } from '../i18n.js';
import { dur, hex, fmtDate, dateKey } from '../fmt.js';
import * as api from '../api.js';
import { loadTags, state } from '../state.js';
import { go } from '../router.js';

export function mount(el) {
  el.className = 'view tags-view';
  const search = h('input', { class: 'input', placeholder: t('Search tags…'), spellcheck: 'false' });
  const body = h('section', { class: 'card' });
  el.append(h('header', { class: 'toolbar' }, h('h2', null, t('Tags')), h('div', { class: 'spacer' }), h('div', { class: 'filter-wrap' }, icon('search', 14), search)), body);
  search.addEventListener('input', draw);

  async function load() { await loadTags(); draw(); }

  function draw() {
    clear(body);
    const q = search.value.trim().toLowerCase();
    const rows = state.tagList.filter((x) => !q || x.tags.toLowerCase().includes(q));
    if (!state.tagList.length) {
      body.append(empty('tag', t('No tags yet'), t('Select time on the timeline and press E, start a stopwatch from the tray, or create autotag rules.')));
      return;
    }
    const table = h('table', { class: 'table' },
      h('thead', null, h('tr', null, [t('Tag'), t('Total'), t('Entries'), t('Last used'), t('Billable'), ''].map((x, i) => h('th', { class: i === 1 || i === 2 ? 'num' : '' }, x)))),
      h('tbody', null, rows.map((r) => {
        const color = h('input', { type: 'color', class: 'color', value: hex(r.color), title: t('Colour') });
        color.addEventListener('change', async () => { await api.post('tags/meta', { tag: r.tags, color: color.value, billable: r.billable }); load(); });
        const parts = r.tags.split(', ');
        return h('tr', null,
          h('td', null, h('div', { class: 'tag-cell' }, color, h('span', { class: 'crumbs' }, parts.map((p, i) => [i ? h('span', { class: 'sep' }, '›') : null, h('span', null, p)])))),
          h('td', { class: 'num' }, dur(r.secs)),
          h('td', { class: 'num' }, r.count),
          h('td', null, h('button', { class: 'link', type: 'button', onClick: () => go('day/' + dateKey(new Date(r.last * 1000))) }, fmtDate(dateKey(new Date(r.last * 1000)), { year: 'numeric', month: 'short', day: 'numeric' }))),
          h('td', null, toggle(r.billable, async (v) => { await api.post('tags/meta', { tag: r.tags, color: r.color, billable: v }); load(); })),
          h('td', { class: 'actions' },
            button('', { icon: 'edit', small: true, variant: 'ghost', title: t('Rename'), onClick: () => rename(r.tags) }),
            button('', { icon: 'search', small: true, variant: 'ghost', title: t('Find days'), onClick: () => go('search?q=' + encodeURIComponent(r.tags)) }),
            button('', { icon: 'trash', small: true, variant: 'ghost', title: t('Delete everywhere'), onClick: () => purge(r) })));
      })));
    body.append(h('div', { class: 'table-wrap' }, table));
    const names = state.tagNames;
    if (names.length) {
      body.append(h('div', { class: 'tag-names' }, h('h4', null, t('Individual tags')), h('p', { class: 'muted' }, t('Renaming an individual tag updates every combination that contains it.')),
        h('div', { class: 'chips' }, names.map((n) => h('button', { class: 'chip', type: 'button', onClick: () => rename(n) }, n, icon('edit', 12))))));
    }
  }

  async function rename(from) {
    const to = await promptDialog(t('Rename tag'), from, { label: t('New name'), ok: t('Rename') });
    if (!to || to === from) return;
    await api.post('tags/rename', { from, to });
    toast(t('Tag renamed'), { type: 'ok' });
    load();
  }

  async function purge(r) {
    if (!(await confirmDialog(t('Delete all {0} entries tagged "{1}" ({2})? This cannot be undone.', r.count, r.tags, dur(r.secs)), { danger: true, ok: t('Delete') }))) return;
    await api.post('tags/purge', { tags: r.tags });
    toast(t('Tag deleted'));
    load();
  }

  load();
  return { refresh: (ev) => { if (ev && ev.type === 'tags') load(); } };
}
