// Activity list under the timeline (like ManicTime's detail grid): every
// segment of a lane in the visible part of the timeline, sortable,
// filterable and checkable for bulk tagging. Consecutive activities of the
// same app or website can be merged into one row. Rows are virtualised.
import { h, clear, icon, button, appIcon, kindIcon, segmented } from './ui.js';
import { t } from './i18n.js';
import { dur, durLong, clock, shortURL, baseName, dirName } from './fmt.js';

const ROW = 34;

// Times are clipped to the day shown (bounds set by setLanes).
const day = { from: 0, to: Infinity };
const cs = (s) => Math.max(s.s, day.from);
const ce = (s) => Math.min(s.e, day.to);
const secs = (r) => (r.segs ? r.secs : ce(r) - cs(r));

const pageName = (s) => s.page || (s.doc.name && s.doc.name !== s.doc.key ? s.doc.name : shortURL(s.doc.key));

const COLUMNS = {
  apps: [
    { key: 'title', label: 'Title', grow: 3, get: (s) => (s.segs ? t('{0} windows', s.titles) : s.title || s.app.name) },
    { key: 'app', label: 'Application', grow: 1.2, get: (s) => s.app.name },
  ],
  docs: [
    { key: 'name', label: 'Name', grow: 2.2, get: (s) => (s.doc.kind === 2 ? (s.segs ? s.site || s.doc.grp : pageName(s)) : baseName(s.doc.key)) },
    { key: 'where', label: 'Location', grow: 2, get: (s) => (s.doc.kind === 2 ? (s.segs ? t('{0} pages', s.pages) : shortURL(s.doc.key)) : dirName(s.doc.key) || s.doc.key) },
  ],
  tags: [
    { key: 'tags', label: 'Tags', grow: 2, get: (s) => s.tag.tags },
    { key: 'notes', label: 'Notes', grow: 2, get: (s) => s.tag.notes },
  ],
  usage: [{ key: 'state', label: 'State', grow: 3, get: (s) => (s.state ? t('Active') : t('Away')) }],
};
const TIME_COLS = [
  { key: 's', label: 'Start', w: 92, get: cs, fmt: (v) => clock(v, true) },
  { key: 'e', label: 'End', w: 92, get: ce, fmt: (v) => (v >= day.to ? '24:00:00' : clock(v, true)) },
  { key: 'd', label: 'Duration', w: 96, get: secs, fmt: (v) => durLong(v) },
];

// Key that decides which consecutive rows merge.
const MERGE_KEY = {
  apps: (s) => 'a' + s.app.id,
  docs: (s) => (s.doc.kind === 2 ? 'w' + (s.site || s.doc.grp) : 'f' + s.doc.key),
};

function loadPref(k, def) {
  try { const v = localStorage.getItem('ts.al.' + k); return v === null ? def : v === '1'; } catch (_) { return def; }
}
function savePref(k, v) {
  try { localStorage.setItem('ts.al.' + k, v ? '1' : '0'); } catch (_) { /* ignore */ }
}

export class ActivityList {
  /**
   * opts: { onPick(row), onHover(row|null), onOpen(row), onContext(row, e), onTag(segs), onDeleteTags(segs), onFilter(text) }
   * A row is a timeline segment, or a merged group { segs, s, e, secs, ... }.
   */
  constructor(host, opts) {
    this.opts = opts;
    this.tab = 'apps';
    this.lanes = {};
    this.range = [0, 0];
    this.sel = null;
    this.follow = true;
    this.merge = { apps: loadPref('mergeApps', false), docs: loadPref('mergeDocs', true) };
    this.sort = { key: 's', dir: 1 };
    this.filter = '';
    this.checked = new Set(); // raw segments
    this.picked = null;
    this.rows = [];

    this.tabs = this.makeTabs();
    this.info = h('span', { class: 'al-info muted' });
    this.mergeBox = h('input', { type: 'checkbox', onChange: (e) => {
      this.merge[this.tab] = e.target.checked;
      savePref(this.tab === 'apps' ? 'mergeApps' : 'mergeDocs', e.target.checked);
      this.refresh(true);
    } });
    this.mergeLabel = h('span');
    this.mergeBtn = h('label', { class: 'al-follow' }, this.mergeBox, this.mergeLabel);
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
        this.mergeBtn, this.followBtn, h('div', { class: 'filter-wrap' }, icon('filter', 14), this.filterInput)),
      this.bulk, this.head, this.body);
    host.append(this.el);
    this.ro = new ResizeObserver(() => this.paint());
    this.ro.observe(this.body);
  }

  makeTabs() {
    const n = (id) => (this.lanes[id] ? this.lanes[id].length : 0);
    return segmented([['apps', `${t('Applications')} ${n('apps') || ''}`, 'app'], ['docs', `${t('Documents')} ${n('docs') || ''}`, 'file'],
      ['tags', `${t('Tags')} ${n('tags') || ''}`, 'tag'], ['usage', t('Computer'), 'monitor']], this.tab, (v) => this.switchTab(v));
  }

  switchTab(tab) {
    this.tab = tab;
    this.checked.clear();
    this.sort = { key: 's', dir: 1 };
    this.refresh(true);
  }

  destroy() { this.ro.disconnect(); }

  setLanes(lanes, from, to) {
    day.from = from;
    day.to = to;
    this.lanes = {};
    for (const l of lanes) this.lanes[l.id] = l.segs;
    const tabs = this.makeTabs();
    this.tabs.replaceWith(tabs);
    this.tabs = tabs;
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
    this.tabs.set(tab);
    this.switchTab(tab);
  }

  rowOf(seg) {
    if (!seg) return null;
    return this.rows.find((r) => r === seg || (r.segs && r.segs.includes(seg))) || null;
  }

  /** Marks a segment's row and scrolls it into view. */
  pick(seg, scroll = true) {
    const row = this.rowOf(seg);
    this.picked = row;
    const i = this.rows.indexOf(row);
    if (i >= 0 && scroll) {
      const top = i * ROW, b = this.body;
      if (top < b.scrollTop || top + ROW > b.scrollTop + b.clientHeight) b.scrollTop = Math.max(0, top - b.clientHeight / 2);
    }
    this.paint(true);
  }

  hoverSeg(seg) {
    const row = this.rowOf(seg);
    if (row === this.hovered) return;
    this.hovered = row;
    this.paint();
  }

  columns() { return [...COLUMNS[this.tab], ...TIME_COLS]; }

  isChecked(r) { return r.segs ? r.segs.every((s) => this.checked.has(s)) : this.checked.has(r); }

  setChecked(r, on) {
    for (const s of r.segs || [r]) {
      if (on) this.checked.add(s); else this.checked.delete(s);
    }
  }

  /** Merges runs of consecutive segments with the same app / website. */
  merged(segs) {
    const keyOf = MERGE_KEY[this.tab];
    const out = [];
    let cur = null;
    for (const s of segs) {
      const k = keyOf(s);
      if (cur && cur.key === k) {
        cur.segs.push(s);
        cur.e = Math.max(cur.e, s.e);
        cur.secs += ce(s) - cs(s);
        cur.names.add(this.tab === 'apps' ? s.title : s.doc.key);
        continue;
      }
      if (cur) out.push(cur.segs.length > 1 ? cur : cur.segs[0]);
      cur = { ...s, key: k, segs: [s], secs: ce(s) - cs(s), names: new Set([this.tab === 'apps' ? s.title : s.doc.key]) };
    }
    if (cur) out.push(cur.segs.length > 1 ? cur : cur.segs[0]);
    for (const r of out) if (r.segs) { r.titles = r.names.size; r.pages = r.names.size; }
    return out;
  }

  refresh(resetScroll = false) {
    const segs = this.lanes[this.tab] || [];
    const [rs, re] = this.range;
    const cols = this.columns();
    const f = this.filter;
    let rows = segs.filter((s) => (!this.follow || (s.e > rs && s.s < re)) && (!f || (s.search || s.label || '').toLowerCase().includes(f) || cols.some((c) => !c.fmt && String(c.get(s) || '').toLowerCase().includes(f))));
    const canMerge = !!MERGE_KEY[this.tab];
    this.mergeBtn.hidden = !canMerge;
    if (canMerge) {
      this.mergeLabel.textContent = this.tab === 'apps' ? t('Merge same app') : t('Merge same website');
      this.mergeBox.checked = this.merge[this.tab];
      if (this.merge[this.tab]) rows = this.merged(rows);
    }
    const col = cols.find((c) => c.key === this.sort.key) || TIME_COLS[0];
    const dir = this.sort.dir;
    rows = rows.slice().sort((a, b) => {
      const x = col.get(a), y = col.get(b);
      if (typeof x === 'number') return (x - y) * dir || a.s - b.s;
      return String(x).localeCompare(String(y)) * dir || a.s - b.s;
    });
    this.rows = rows;
    if (this.picked && !rows.includes(this.picked)) this.picked = this.picked.segs ? this.rowOf(this.picked.segs[0]) : this.rowOf(this.picked);
    const clipTotal = (r) => (r.segs || [r]).reduce((n, s) => n + Math.max(0, Math.min(s.e, this.follow ? re : s.e) - Math.max(s.s, this.follow ? rs : s.s)), 0);
    const total = rows.reduce((n, r) => n + clipTotal(r), 0);
    this.info.textContent = `${rows.length} · ${dur(total)}` + (this.follow ? ` · ${clock(rs)}–${re >= day.to ? '24:00' : clock(re)}` : '');
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
    const allChecked = this.rows.length > 0 && this.rows.every((r) => this.isChecked(r));
    const box = h('input', { type: 'checkbox', checked: allChecked, title: t('Check all'), onChange: (e) => {
      if (e.target.checked) this.rows.forEach((r) => this.setChecked(r, true)); else this.checked.clear();
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
    const total = segs.reduce((x, s) => x + (ce(s) - cs(s)), 0);
    this.bulk.append(icon('check', 15), h('span', null, t('{0} selected · {1}', n, dur(total))),
      this.tab !== 'tags' ? button(t('Tag…'), { icon: 'tag', small: true, variant: 'primary', onClick: () => this.opts.onTag && this.opts.onTag(segs) }) : null,
      this.tab === 'tags' ? button(t('Delete tags'), { icon: 'trash', small: true, variant: 'danger', onClick: () => this.opts.onDeleteTags && this.opts.onDeleteTags(segs) }) : null,
      button(t('Clear'), { small: true, variant: 'ghost', onClick: () => { this.checked.clear(); this.renderBulk(); this.renderHead(); this.paint(true); } }));
  }

  lead(s) {
    switch (this.tab) {
      case 'apps': return appIcon(s.app, 18);
      case 'docs': return kindIcon(s.doc.kind, s.site || s.doc.grp || s.doc.key, 18);
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
      const r = this.rows[i];
      const inSel = sel && r.s < sel.e && r.e > sel.s;
      const checked = this.isChecked(r);
      const row = h('div', {
        class: 'al-row' + (r === this.picked ? ' picked' : '') + (r === this.hovered ? ' hover' : '') + (inSel ? ' in-sel' : '') + (checked ? ' checked' : ''),
        style: { gridTemplateColumns: tpl },
        onClick: (e) => { if (e.target.tagName !== 'INPUT') { this.picked = r; this.paint(true); this.opts.onPick && this.opts.onPick(r); } },
        onDblclick: () => this.opts.onOpen && this.opts.onOpen(r),
        onMouseenter: () => this.opts.onHover && this.opts.onHover(r),
        onContextmenu: (e) => { e.preventDefault(); this.opts.onContext && this.opts.onContext(r.segs ? r.segs[0] : r, e); },
      });
      const box = h('input', { type: 'checkbox', checked, onChange: (e) => {
        this.setChecked(r, e.target.checked);
        row.classList.toggle('checked', e.target.checked);
        this.renderBulk();
      } });
      row.append(h('span', { class: 'al-check' }, box), h('span', { class: 'al-lead' }, this.lead(r)));
      for (const c of cols) {
        const v = c.get(r);
        const text = c.fmt ? c.fmt(v) : v || '';
        const cell = h('span', { class: 'al-td' + (c.w ? ' num' : '') + (c.key === 'where' ? ' mono' : ''), title: c.fmt ? null : text }, text);
        if (r.segs && c === cols[0]) cell.append(h('span', { class: 'pill al-count' }, '×' + r.segs.length));
        row.append(cell);
      }
      this.rowsEl.append(row);
    }
  }
}
