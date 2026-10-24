// Activity list under the timeline (like ManicTime's detail grid): every
// segment of a lane in the visible part of the timeline, sortable,
// filterable and checkable for bulk tagging. Rows are virtualised.
import { h, clear, icon, button, appIcon, kindIcon, segmented } from './ui.js';
import { t } from './i18n.js';
import { dur, durLong, clock, shortURL, baseName, dirName } from './fmt.js';

const ROW = 34;

const COLUMNS = {
  apps: [
    { key: 'title', label: 'Title', grow: 3, get: (s) => s.title || s.app.name },
    { key: 'app', label: 'Application', grow: 1.2, get: (s) => s.app.name },
  ],
  docs: [
    { key: 'name', label: 'Name', grow: 2.2, get: (s) => (s.doc.kind === 2 ? s.doc.name || shortURL(s.doc.key) : baseName(s.doc.key)) },
    { key: 'where', label: 'Location', grow: 2, get: (s) => (s.doc.kind === 2 ? shortURL(s.doc.key) : dirName(s.doc.key) || s.doc.key) },
  ],
  tags: [
    { key: 'tags', label: 'Tags', grow: 2, get: (s) => s.tag.tags },
    { key: 'notes', label: 'Notes', grow: 2, get: (s) => s.tag.notes },
  ],
  usage: [{ key: 'state', label: 'State', grow: 3, get: (s) => (s.state ? t('Active') : t('Away')) }],
};
// Times are clipped to the day shown (bounds set by setLanes).
const day = { from: 0, to: Infinity };
const cs = (s) => Math.max(s.s, day.from);
const ce = (s) => Math.min(s.e, day.to);
const TIME_COLS = [
  { key: 's', label: 'Start', w: 92, get: cs, fmt: (v) => clock(v, true) },
  { key: 'e', label: 'End', w: 92, get: ce, fmt: (v) => (v >= day.to ? '24:00:00' : clock(v, true)) },
  { key: 'd', label: 'Duration', w: 96, get: (s) => ce(s) - cs(s), fmt: (v) => durLong(v) },
];

export class ActivityList {
  /**
   * opts: { onPick(seg), onHover(seg|null), onOpen(seg), onContext(seg, e), onTag(segs), onFilter(text) }
   */
  constructor(host, opts) {
    this.opts = opts;
    this.tab = 'apps';
    this.lanes = {};
    this.range = [0, 0];
    this.sel = null;
    this.follow = true;
    this.sort = { key: 's', dir: 1 };
    this.filter = '';
    this.checked = new Set();
    this.picked = null;
    this.rows = [];

    this.tabs = segmented(this.tabDefs(), this.tab, (v) => { this.tab = v; this.checked.clear(); this.sort = { key: 's', dir: 1 }; this.refresh(true); });
    this.info = h('span', { class: 'al-info muted' });
    this.followBtn = h('label', { class: 'al-follow', title: t('Show only what is visible in the timeline') },
      h('input', { type: 'checkbox', checked: true, onChange: (e) => { this.follow = e.target.checked; this.refresh(true); } }), h('span', null, t('Follow timeline')));
    this.filterInput = h('input', { class: 'input', placeholder: t('Filter…') + ' (Ctrl+F)', spellcheck: 'false' });
    this.filterInput.addEventListener('input', () => { this.filter = this.filterInput.value.trim().toLowerCase(); this.opts.onFilter && this.opts.onFilter(this.filterInput.value.trim()); this.refresh(true); });
    this.filterInput.addEventListener('keydown', (e) => { if (e.key === 'Escape') { this.setFilter(''); this.filterInput.blur(); e.stopPropagation(); } });
    this.bulk = h('div', { class: 'al-bulk', hidden: true });
    this.head = h('div', { class: 'al-head' });
    this.body = h('div', { class: 'al-body', tabindex: '0' });
    this.spacer = h('div', { class: 'al-spacer' });
    this.rowsEl = h('div', { class: 'al-rows' });
    this.body.append(this.spacer, this.rowsEl);
    this.body.addEventListener('scroll', () => this.paint());
    this.body.addEventListener('mouseleave', () => this.opts.onHover && this.opts.onHover(null));
    this.el = h('section', { class: 'card al-card' },
      h('div', { class: 'card-head al-top' }, icon('menu', 16), h('h3', null, t('Activities')), this.tabs, this.info, h('div', { class: 'spacer' }),
        this.followBtn, h('div', { class: 'filter-wrap' }, icon('filter', 14), this.filterInput)),
      this.bulk, this.head, this.body);
    host.append(this.el);
    this.ro = new ResizeObserver(() => this.paint());
    this.ro.observe(this.body);
  }

  tabDefs() {
    const n = (id) => (this.lanes[id] ? this.lanes[id].length : 0);
    return [['apps', `${t('Applications')} ${n('apps') || ''}`, 'app'], ['docs', `${t('Documents')} ${n('docs') || ''}`, 'file'],
      ['tags', `${t('Tags')} ${n('tags') || ''}`, 'tag'], ['usage', t('Computer'), 'monitor']];
  }

  destroy() { this.ro.disconnect(); }

  setLanes(lanes, from, to) {
    day.from = from;
    day.to = to;
    this.lanes = {};
    for (const l of lanes) this.lanes[l.id] = l.segs;
    this.tabs.replaceWith((this.tabs = segmented(this.tabDefs(), this.tab, (v) => { this.tab = v; this.checked.clear(); this.sort = { key: 's', dir: 1 }; this.refresh(true); })));
    // Keep checks that still exist after a refresh.
    const all = new Set(this.lanes[this.tab] || []);
    for (const s of [...this.checked]) if (!all.has(s)) this.checked.delete(s);
    this.refresh();
  }

  setRange(s, e) { this.range = [s, e]; if (this.follow) this.refresh(); }
  setSelection(sel) { this.sel = sel; this.paint(); }
  setFilter(text) { this.filterInput.value = text; this.filter = text.toLowerCase(); this.opts.onFilter && this.opts.onFilter(text); this.refresh(true); }
  focusFilter() { this.filterInput.focus(); this.filterInput.select(); }

  showTab(tab) {
    if (tab === this.tab) return;
    this.tab = tab;
    this.tabs.set(tab);
    this.checked.clear();
    this.sort = { key: 's', dir: 1 };
    this.refresh(true);
  }

  /** Marks a segment's row and scrolls it into view. */
  pick(seg, scroll = true) {
    this.picked = seg;
    const i = this.rows.indexOf(seg);
    if (i >= 0 && scroll) {
      const top = i * ROW, b = this.body;
      if (top < b.scrollTop || top + ROW > b.scrollTop + b.clientHeight) b.scrollTop = Math.max(0, top - b.clientHeight / 2);
    }
    this.paint();
  }

  hoverSeg(seg) {
    if (seg === this.hovered) return;
    this.hovered = seg;
    this.paint();
  }

  columns() { return [...COLUMNS[this.tab], ...TIME_COLS]; }

  refresh(resetScroll = false) {
    const segs = this.lanes[this.tab] || [];
    const [rs, re] = this.range;
    const cols = this.columns();
    const f = this.filter;
    let rows = segs.filter((s) => (!this.follow || (s.e > rs && s.s < re)) && (!f || (s.search || s.label || '').toLowerCase().includes(f) || cols.some((c) => !c.fmt && String(c.get(s) || '').toLowerCase().includes(f))));
    const col = cols.find((c) => c.key === this.sort.key) || TIME_COLS[0];
    const dir = this.sort.dir;
    rows = rows.slice().sort((a, b) => {
      const x = col.get(a), y = col.get(b);
      if (typeof x === 'number') return (x - y) * dir || a.s - b.s;
      return String(x).localeCompare(String(y)) * dir || a.s - b.s;
    });
    this.rows = rows;
    const total = rows.reduce((n, s) => n + Math.max(0, Math.min(s.e, this.follow ? re : s.e) - Math.max(s.s, this.follow ? rs : s.s)), 0);
    this.info.textContent = `${rows.length} · ${dur(total)}` + (this.follow ? ` · ${clock(rs)}–${clock(re)}` : '');
    this.renderHead();
    this.spacer.style.height = rows.length * ROW + 'px';
    if (resetScroll) this.body.scrollTop = 0;
    this.renderBulk();
    this.paint(true);
  }

  gridTemplate() {
    return '34px 26px ' + this.columns().map((c) => (c.w ? c.w + 'px' : `minmax(0, ${c.grow}fr)`)).join(' ');
  }

  renderHead() {
    clear(this.head);
    this.head.style.gridTemplateColumns = this.gridTemplate();
    const allChecked = this.rows.length > 0 && this.rows.every((s) => this.checked.has(s));
    const box = h('input', { type: 'checkbox', checked: allChecked, title: t('Check all'), onChange: (e) => {
      if (e.target.checked) this.rows.forEach((s) => this.checked.add(s)); else this.checked.clear();
      this.renderBulk(); this.paint(true);
    } });
    this.head.append(h('span', { class: 'al-check' }, box), h('span'));
    for (const c of this.columns()) {
      const on = this.sort.key === c.key;
      this.head.append(h('button', {
        type: 'button', class: 'al-th' + (on ? ' on' : '') + (c.w ? ' num' : ''),
        onClick: () => { this.sort = { key: c.key, dir: on ? -this.sort.dir : 1 }; this.refresh(); },
      }, h('span', null, t(c.label)), on ? icon(this.sort.dir > 0 ? 'up' : 'down', 13) : null));
    }
  }

  renderBulk() {
    const n = this.checked.size;
    if (!n) { this.bulk.hidden = true; return; }
    this.bulk.hidden = false;
    clear(this.bulk);
    const segs = [...this.checked];
    const total = segs.reduce((x, s) => x + (s.e - s.s), 0);
    this.bulk.append(icon('check', 15), h('span', null, t('{0} selected · {1}', n, dur(total))),
      this.tab !== 'tags' ? button(t('Tag…'), { icon: 'tag', small: true, variant: 'primary', onClick: () => this.opts.onTag && this.opts.onTag(segs) }) : null,
      this.tab === 'tags' ? button(t('Delete tags'), { icon: 'trash', small: true, variant: 'danger', onClick: () => this.opts.onDeleteTags && this.opts.onDeleteTags(segs) }) : null,
      button(t('Clear'), { small: true, variant: 'ghost', onClick: () => { this.checked.clear(); this.renderBulk(); this.renderHead(); this.paint(true); } }));
  }

  lead(s) {
    switch (this.tab) {
      case 'apps': return appIcon(s.app, 18);
      case 'docs': return kindIcon(s.doc.kind, s.doc.grp || s.doc.key, 18);
      case 'tags': return h('span', { class: 'dot lg', style: { background: s.color } });
      default: return h('span', { class: 'dot lg', style: { background: s.state ? 'var(--active)' : 'var(--away-strong)' } });
    }
  }

  paint(force = false) {
    const b = this.body;
    const first = Math.max(0, Math.floor(b.scrollTop / ROW) - 6);
    const last = Math.min(this.rows.length, Math.ceil((b.scrollTop + b.clientHeight) / ROW) + 6);
    if (!force && this.painted && this.painted[0] === first && this.painted[1] === last && this.painted[2] === this.picked && this.painted[3] === this.hovered && this.painted[4] === this.sel) {
      return;
    }
    this.painted = [first, last, this.picked, this.hovered, this.sel];
    clear(this.rowsEl);
    this.rowsEl.style.transform = `translateY(${first * ROW}px)`;
    if (!this.rows.length) {
      this.rowsEl.style.transform = '';
      this.rowsEl.append(h('div', { class: 'al-empty muted' }, this.filter ? t('No matches') : this.follow ? t('Nothing in the visible part of the timeline.') : t('Nothing recorded.')));
      return;
    }
    const tpl = this.gridTemplate();
    const cols = this.columns();
    const sel = this.sel;
    for (let i = first; i < last; i++) {
      const s = this.rows[i];
      const inSel = sel && s.s < sel.e && s.e > sel.s;
      const row = h('div', {
        class: 'al-row' + (s === this.picked ? ' picked' : '') + (s === this.hovered ? ' hover' : '') + (inSel ? ' in-sel' : '') + (this.checked.has(s) ? ' checked' : ''),
        style: { gridTemplateColumns: tpl },
        onClick: (e) => { if (e.target.tagName !== 'INPUT') { this.picked = s; this.paint(true); this.opts.onPick && this.opts.onPick(s); } },
        onDblclick: () => this.opts.onOpen && this.opts.onOpen(s),
        onMouseenter: () => this.opts.onHover && this.opts.onHover(s),
        onContextmenu: (e) => { e.preventDefault(); this.opts.onContext && this.opts.onContext(s, e); },
      });
      const box = h('input', { type: 'checkbox', checked: this.checked.has(s), onChange: (e) => {
        if (e.target.checked) this.checked.add(s); else this.checked.delete(s);
        row.classList.toggle('checked', e.target.checked);
        this.renderBulk();
      } });
      row.append(h('span', { class: 'al-check' }, box), h('span', { class: 'al-lead' }, this.lead(s)));
      for (const c of cols) {
        const v = c.get(s);
        const text = c.fmt ? c.fmt(v) : v || '';
        row.append(h('span', { class: 'al-td' + (c.w ? ' num' : '') + (c.key === 'where' ? ' mono' : ''), title: c.fmt ? null : text }, text));
      }
      this.rowsEl.append(row);
    }
  }
}

