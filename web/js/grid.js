// Day view layout: blocks sit in full-width rows. A row holds one to three
// blocks whose widths (1/3, 1/2, 2/3 or all of 12 columns) always add up to
// the full width, and the blocks of a row share its height, picked from a
// short list. A row with a fit-to-content block (the timeline) takes that
// block's height.
//
// A block is picked up by pressing on any empty part of it. Dropped on the
// middle of another block it takes that block's place and width, and the
// other block moves down into a full-width row of its own (in the same row
// the two swap). Dropped on the left or right edge of a block it joins that
// row beside it (the row's blocks share the width again, the moved one
// keeping its width when it can); near the top or bottom edge of a row it
// gets a row of its own there. The row it left closes up to full width.
// The border between two blocks of a row moves both, the bottom border sets
// the row's height.
import { h } from './ui.js';

export const COLS = 12;
const ROW = 30; // px per height unit
const GAP = 14; // px between blocks
const NARROW = 860; // below this width blocks simply stack
const HEIGHTS = [6, 8, 10, 13, 16, 20]; // row heights, in units
const COMBOS = { 1: [[12]], 2: [[6, 6], [8, 4], [4, 8]], 3: [[4, 4, 4]] };
const EDGE = 48; // px at a row's top/bottom edge that make a new row

// Pressing on these never starts a drag: they have their own interaction.
const NO_DRAG = 'button, a, input, select, textarea, label, [contenteditable], .row, .sub-row, .al-row, .al-head, .al-bulk, ' +
  '.tl-wrap, .tl-mini, canvas, .shot, .chip, .segmented, .suggest, .sel-bar, .banner, .filter-wrap, .gi-edge, .empty .btn';

// A press on a scrollbar targets the scrolling element itself.
function onScrollbar(e) {
  const el = e.target;
  if (!(el instanceof HTMLElement)) return false;
  const r = el.getBoundingClientRect();
  return (el.scrollHeight > el.clientHeight && e.clientX > r.left + el.clientWidth) ||
    (el.scrollWidth > el.clientWidth && e.clientY > r.top + el.clientHeight);
}

const nearest = (list, v) => list.reduce((best, x) => (Math.abs(x - v) < Math.abs(best - v) ? x : best), list[0]);
const unitPx = (u) => u * ROW + (u - 1) * GAP;
const copy = (rows) => rows.map((r) => ({ h: r.h, items: r.items.map((i) => ({ ...i })) }));

/**
 * Widths for the blocks of one row ([{ id, w }], w = wanted width): the
 * allowed combination closest to the wanted widths, or null if there is none.
 * allowed(id) lists the widths a block may have.
 */
export function fitRow(items, allowed) {
  let best = null, bestD = Infinity;
  for (const c of COMBOS[items.length] || []) {
    if (c.some((w, i) => !allowed(items[i].id).includes(w))) continue;
    const d = c.reduce((n, w, i) => n + Math.abs(w - items[i].w), 0);
    if (d < bestD) { best = c; bestD = d; }
  }
  return best;
}

/** Fits every row to the full width, keeping the widths' proportions as far
 *  as it can; a row that can't be fitted is split into more rows. */
export function normalize(rows, allowed) {
  const out = [];
  for (const r of rows) {
    let items = r.items;
    while (items.length) {
      let k = Math.min(3, items.length), fit = null;
      for (; k > 0 && !fit; k--) {
        const part = items.slice(0, k);
        const sum = part.reduce((n, i) => n + i.w, 0) || 1;
        fit = fitRow(part.map((i) => ({ id: i.id, w: (i.w * COLS) / sum })), allowed);
        if (fit) break;
      }
      if (!fit) { k = 1; fit = [COLS]; }
      out.push({ h: r.h, items: items.slice(0, k).map((it, i) => ({ id: it.id, w: fit[i] })) });
      items = items.slice(k);
    }
  }
  return out;
}

export class Board {
  /**
   * blocks: [{ id, el, row, w, h, widths: [6, 8, 12], autoHeight }] in their
   *   default rows (row index) and order; h is the default row height.
   * layout: saved [{ id, x, y, w, h }] (y = row; may be empty or stale)
   * onChange(layout) is called after every drag or resize.
   */
  constructor(host, blocks, layout, onChange) {
    this.host = host;
    this.onChange = onChange;
    this.blocks = new Map(blocks.map((b) => [b.id, b]));
    this.allowed = (id) => this.blocks.get(id).widths;
    const x = {};
    this.defaults = this.fromItems(blocks.map((b) => {
      const it = { id: b.id, x: x[b.row] || 0, y: b.row, w: b.w, h: b.h || 0 };
      x[b.row] = it.x + b.w;
      return it;
    }));
    this.hidden = new Set();
    this.items = new Map();
    this.edges = new Map();
    this.preview = null; // rows shown while dragging or resizing
    this.dragId = null;
    host.classList.add('grid');
    this.holder = h('div', { class: 'gi-holder', hidden: true });
    host.append(this.holder);
    for (const b of blocks) {
      const el = h('div', { class: 'gi' + (b.autoHeight ? ' auto' : ''), dataset: { id: b.id } }, b.el);
      const edges = { e: h('div', { class: 'gi-edge e', hidden: true }), s: h('div', { class: 'gi-edge s', hidden: true }) };
      for (const [dir, edge] of Object.entries(edges)) {
        edge.addEventListener('pointerdown', (e) => this.startResize(e, b.id, edge, dir));
        el.append(edge);
      }
      el.addEventListener('pointerdown', (e) => this.press(e, b.id));
      host.append(el);
      this.items.set(b.id, el);
      this.edges.set(b.id, edges);
      if (b.autoHeight) new ResizeObserver(() => this.render()).observe(b.el);
    }
    const saved = this.fromItems(layout);
    this.rows = saved.length ? saved : copy(this.defaults);
    this.ro = new ResizeObserver(() => this.render());
    this.ro.observe(host);
    this.render();
    // Animate moves only after the first placement.
    setTimeout(() => host.classList.add('ready'), 60);
  }

  /** Rows from saved items: items with the same y form a row. Blocks the
   *  layout doesn't have get a row of their own at the end. */
  fromItems(items) {
    const seen = new Set();
    const ok = [];
    for (const it of items || []) {
      if (!this.blocks.has(it.id) || seen.has(it.id) || ![it.x, it.y, it.w].every(Number.isFinite)) continue;
      seen.add(it.id);
      ok.push(it);
    }
    if (!ok.length) return [];
    ok.sort((a, b) => a.y - b.y || a.x - b.x);
    const rows = [];
    let y = null;
    for (const it of ok) {
      if (y !== it.y) rows.push({ h: it.h, items: [] });
      y = it.y;
      rows[rows.length - 1].items.push({ id: it.id, w: it.w });
    }
    for (const b of this.blocks.values()) if (!seen.has(b.id)) rows.push({ h: b.h, items: [{ id: b.id, w: COLS }] });
    return this.tidy(rows);
  }

  isAuto(row) { return row.items.some((i) => this.blocks.get(i.id).autoHeight); }

  /** Full-width rows with heights from the list (0 for fit-to-content rows). */
  tidy(rows) {
    return normalize(rows.filter((r) => r.items.length), this.allowed).map((r) => {
      if (this.isAuto(r)) return { ...r, h: 0 };
      const def = Math.max(...r.items.map((i) => this.blocks.get(i.id).h || 0));
      return { ...r, h: nearest(HEIGHTS, r.h || def || 10) };
    });
  }

  visibleRows(rows = this.rows) {
    return this.tidy(rows.map((r) => ({ h: r.h, items: r.items.filter((i) => !this.hidden.has(i.id)) })));
  }

  /** Puts hidden blocks back (each in a row of its own where it was). */
  withHidden(rows) {
    const out = copy(rows);
    this.rows.forEach((r, i) => {
      for (const it of r.items) {
        if (this.hidden.has(it.id)) out.splice(Math.min(i, out.length), 0, { h: r.h, items: [{ id: it.id, w: COLS }] });
      }
    });
    return this.tidy(out);
  }

  current() {
    const out = [];
    this.rows.forEach((r, y) => {
      let x = 0;
      for (const it of r.items) { out.push({ id: it.id, x, y, w: it.w, h: r.h }); x += it.w; }
    });
    return out;
  }

  reset() {
    this.rows = copy(this.defaults);
    this.render();
    this.onChange(this.current());
  }

  setHidden(id, hidden) {
    if (hidden === this.hidden.has(id)) return;
    if (hidden) this.hidden.add(id); else this.hidden.delete(id);
    this.render();
  }

  get colW() { return (this.host.clientWidth - GAP * (COLS - 1)) / COLS; }

  /** Pixel boxes of every block and row. */
  geometry(rows) {
    const cw = this.colW;
    const g = { rows: [], rects: new Map(), height: 0 };
    let top = 0;
    for (const r of rows) {
      const auto = r.items.find((i) => this.blocks.get(i.id).autoHeight);
      const hh = auto ? this.blocks.get(auto.id).el.offsetHeight || unitPx(10) : unitPx(r.h);
      let x = 0;
      const items = r.items.map((it) => {
        const rect = { id: it.id, left: x * (cw + GAP), top, width: it.w * cw + (it.w - 1) * GAP, height: hh };
        x += it.w;
        g.rects.set(it.id, rect);
        return rect;
      });
      g.rows.push({ top, bottom: top + hh, items });
      top += hh + GAP;
    }
    g.height = Math.max(0, top - GAP);
    return g;
  }

  /** Whether the border after block i of a row can move. */
  canMove(row, i) {
    const a = row.items[i], b = row.items[i + 1];
    if (!b) return false;
    const sum = a.w + b.w;
    return this.allowed(a.id).some((w) => w !== a.w && w < sum && this.allowed(b.id).includes(sum - w));
  }

  render() {
    const rows = this.preview || this.visibleRows();
    const narrow = this.host.clientWidth < NARROW;
    this.host.classList.toggle('narrow', narrow);
    const shown = new Set(rows.flatMap((r) => r.items.map((i) => i.id)));
    for (const [id, el] of this.items) el.hidden = !shown.has(id);
    if (narrow) {
      // Stack in reading order; no dragging.
      let top = 0;
      for (const r of rows) {
        for (const it of r.items) {
          const el = this.items.get(it.id);
          const auto = this.blocks.get(it.id).autoHeight;
          el.style.transform = `translate(0px, ${top}px)`;
          el.style.width = '100%';
          el.style.height = auto ? '' : unitPx(r.h || this.blocks.get(it.id).h || 10) + 'px';
          top += el.offsetHeight + GAP;
        }
      }
      this.host.style.height = Math.max(0, top - GAP) + 'px';
      this.holder.hidden = true;
      return;
    }
    const g = this.geometry(rows);
    this.geo = g;
    rows.forEach((r, ri) => r.items.forEach((it, i) => {
      const edges = this.edges.get(it.id);
      edges.e.hidden = !!this.dragId || !this.canMove(r, i);
      edges.s.hidden = !!this.dragId || this.isAuto(r);
      if (it.id === this.dragId) return;
      const el = this.items.get(it.id);
      const p = g.rects.get(it.id);
      el.style.transform = `translate(${p.left}px, ${p.top}px)`;
      el.style.width = p.width + 'px';
      el.style.height = this.blocks.get(it.id).autoHeight ? '' : p.height + 'px';
    }));
    const hp = this.dragId && g.rects.get(this.dragId);
    this.holder.hidden = !hp;
    if (hp) {
      this.holder.style.transform = `translate(${hp.left}px, ${hp.top}px)`;
      this.holder.style.width = hp.width + 'px';
      this.holder.style.height = hp.height + 'px';
    }
    this.host.style.height = g.height + 'px';
  }

  // ---- dragging ---------------------------------------------------------

  /** A press on an empty part of a block becomes a drag once it moves. */
  press(e, id) {
    if (e.button !== 0 || this.host.classList.contains('narrow')) return;
    if (e.target.closest(NO_DRAG) || onScrollbar(e)) return;
    const sx = e.clientX, sy = e.clientY;
    const move = (ev) => {
      if (Math.hypot(ev.clientX - sx, ev.clientY - sy) < 5) return;
      stop();
      this.startDrag(e, id, ev);
    };
    const stop = () => {
      window.removeEventListener('pointermove', move);
      window.removeEventListener('pointerup', stop);
    };
    window.addEventListener('pointermove', move);
    window.addEventListener('pointerup', stop);
  }

  startDrag(e, id, firstMove) {
    const el = this.items.get(id);
    try { el.setPointerCapture(e.pointerId); } catch (_) { /* best effort */ }
    window.getSelection && window.getSelection().removeAllRanges();
    const rows = this.visibleRows();
    const ri = rows.findIndex((r) => r.items.some((i) => i.id === id));
    const src = rows[ri];
    const me = { id, w: src.items.find((i) => i.id === id).w, h: src.h || this.blocks.get(id).h || 10 };
    // The layout without the block: its row closes up (or goes away).
    const base = this.tidy(rows.map((r, i) => (i === ri ? { h: r.h, items: r.items.filter((x) => x.id !== id) } : r)));
    const srcRow = src.items.length > 1 ? ri : -1; // the row it left, in `base`
    const srcIdx = src.items.findIndex((i) => i.id === id);
    // Where it goes (rows and indexes in `base`): a new row before row `at`;
    // into row `row` before block `index` (join); or in place of that block
    // (replace), which then gets a row of its own just below.
    let target = srcRow >= 0 ? { type: 'join', row: ri, index: srcIdx } : { type: 'new', at: ri };
    const same = (a, b) => a.type === b.type && a.row === b.row && a.index === b.index && a.at === b.at;
    const fitted = (r, items) => {
      const fit = fitRow(items, this.allowed);
      if (!fit) return false;
      r.items = items.map((x, i) => ({ id: x.id, w: fit[i] }));
      return true;
    };
    const build = (t) => {
      const out = copy(base);
      if (t.type === 'new') {
        out.splice(t.at, 0, { h: me.h, items: [{ id, w: COLS }] });
        return this.tidy(out);
      }
      const r = out[t.row];
      if (!r) return null;
      if (t.type === 'join') {
        if (r.items.length >= 3) return null;
        // The moved block keeps its width if it can; the others share the rest.
        const want = me.w < COLS ? me.w : COLS / (r.items.length + 1);
        const items = r.items.map((x) => ({ id: x.id, w: (x.w * (COLS - want)) / COLS }));
        items.splice(t.index, 0, { id, w: want });
        return fitted(r, items) ? this.tidy(out) : null;
      }
      // Replace: it takes the other block's place and width.
      const other = r.items[t.index];
      if (t.row === srcRow) {
        // Same row: the two swap places (and widths).
        const items = src.items.map((x) => ({ ...x }));
        const oi = items.findIndex((x) => x.id === other.id);
        items[oi] = { id, w: other.w };
        items[srcIdx] = { id: other.id, w: me.w };
        return fitted(r, items) ? this.tidy(out) : null;
      }
      const items = r.items.map((x) => ({ ...x }));
      items[t.index] = { id, w: other.w };
      if (!fitted(r, items)) return null;
      out.splice(t.row + 1, 0, { h: r.h || this.blocks.get(other.id).h || 10, items: [{ id: other.id, w: COLS }] });
      return this.tidy(out);
    };
    const rowOf = (bid) => base.findIndex((r) => r.items.some((x) => x.id === bid));
    // The place under the pointer (host coordinates).
    const targetAt = (lx, ly) => {
      const g = this.geo;
      if (!g.rows.length || ly < g.rows[0].top - GAP / 2) return { type: 'new', at: 0 };
      let p = 0;
      while (p + 1 < g.rows.length && ly >= g.rows[p + 1].top - GAP / 2) p++;
      const r = g.rows[p];
      if (p === g.rows.length - 1 && ly > r.bottom + GAP / 2) return { type: 'new', at: base.length };
      const others = r.items.filter((x) => x.id !== id);
      if (!others.length) return target; // over the new row being shown
      const over = r.items.find((x) => lx >= x.left - GAP / 2 && lx < x.left + x.width + GAP / 2) ||
        (lx < r.items[0].left ? r.items[0] : r.items[r.items.length - 1]);
      // Over its own placeholder nothing changes (it is picked up by its
      // header, at the top edge of its row).
      if (over.id === id && ly >= r.top && ly <= r.bottom) return target;
      // The row a replaced block was pushed into stands for the place below
      // the row it came from.
      const pushed = target.type === 'replace' && target.row !== srcRow && others.length === 1 &&
        others[0].id === base[target.row].items[target.index].id;
      const b = rowOf(others[0].id);
      const edge = Math.min(EDGE, (r.bottom - r.top) / 4);
      if (ly < r.top + edge) return { type: 'new', at: pushed ? b + 1 : b };
      if (ly > r.bottom - edge) return { type: 'new', at: b + 1 };
      if (over.id === id) return target;
      const index = base[b].items.findIndex((x) => x.id === over.id);
      const side = Math.min(70, over.width * 0.2);
      const tries = lx < over.left + side ? ['join', 'replace'] : lx > over.left + over.width - side ? ['join+', 'replace'] : ['replace', 'join'];
      for (const k of tries) {
        const t = k === 'replace' ? { type: 'replace', row: b, index } : { type: 'join', row: b, index: index + (k === 'join+' ? 1 : 0) };
        if (build(t)) return t;
      }
      return { type: 'new', at: ly < (r.top + r.bottom) / 2 ? b : b + 1 };
    };

    const start = this.geo.rects.get(id);
    const hr0 = this.host.getBoundingClientRect();
    const offX = e.clientX - hr0.left - start.left, offY = e.clientY - hr0.top - start.top;
    const sc = this.host.closest('.main');
    let px = firstMove.clientX, py = firstMove.clientY;
    this.dragId = id;
    this.preview = rows;
    el.classList.add('dragging');
    document.body.classList.add('grid-dragging');
    this.render();

    // Where the pointer was at the last change: the blocks moving under a
    // still pointer must not flip the place back.
    let at = null;
    const place = () => {
      const hr = this.host.getBoundingClientRect();
      const lx = px - hr.left, ly = py - hr.top;
      el.style.transform = `translate(${lx - offX}px, ${ly - offY}px) scale(1.01)`;
      if (at && Math.hypot(lx - at.x, ly - at.y) < 10) return;
      const t = targetAt(lx, ly);
      if (same(t, target)) return;
      const next = build(t);
      if (!next) return;
      target = t;
      at = { x: lx, y: ly };
      this.preview = next;
      this.render();
    };
    let timer = 0;
    const tick = () => {
      if (sc && Math.hypot(px - e.clientX, py - e.clientY) > 30) {
        const r = sc.getBoundingClientRect();
        const v = py < r.top + 60 ? -16 : py > r.bottom - 60 ? 16 : 0;
        if (v) { sc.scrollTop += v; place(); }
      }
      timer = setTimeout(tick, 16);
    };
    tick();
    place();
    const move = (ev) => { px = ev.clientX; py = ev.clientY; place(); };
    const end = () => {
      el.removeEventListener('pointermove', move);
      el.removeEventListener('pointerup', end);
      el.removeEventListener('pointercancel', end);
      clearTimeout(timer);
      el.classList.remove('dragging');
      document.body.classList.remove('grid-dragging');
      const done = this.preview;
      this.dragId = null;
      this.preview = null;
      this.commit(done);
    };
    el.addEventListener('pointermove', move);
    el.addEventListener('pointerup', end);
    el.addEventListener('pointercancel', end);
  }

  /**
   * Border resize: the right border moves the border shared with the next
   * block of the row (both change width); the bottom border sets the row
   * height. Both jump between the allowed sizes nearest to the pointer.
   */
  startResize(e, id, handle, dir) {
    if (e.button !== 0 || this.host.classList.contains('narrow')) return;
    e.preventDefault();
    e.stopPropagation();
    try { handle.setPointerCapture(e.pointerId); } catch (_) { /* best effort */ }
    const el = this.items.get(id);
    const rows = this.visibleRows();
    const ri = rows.findIndex((r) => r.items.some((i) => i.id === id));
    const i = rows[ri].items.findIndex((x) => x.id === id);
    const top = this.geo.rows[ri].top;
    this.preview = rows;
    el.classList.add('resizing');
    document.body.classList.add('grid-resizing', 'resize-' + dir);
    const move = (ev) => {
      const hr = this.host.getBoundingClientRect();
      const lx = ev.clientX - hr.left, ly = ev.clientY - hr.top;
      const next = copy(this.preview);
      const row = next[ri];
      if (dir === 'e') {
        const a = row.items[i], b = row.items[i + 1];
        const sum = a.w + b.w;
        const left = row.items.slice(0, i).reduce((n, x) => n + x.w, 0);
        const col = (lx + GAP / 2) / (this.colW + GAP); // pointer, in columns
        let best = a.w;
        for (const w of this.allowed(a.id)) {
          if (w < sum && this.allowed(b.id).includes(sum - w) && Math.abs(left + w - col) < Math.abs(left + best - col)) best = w;
        }
        if (best === a.w) return;
        a.w = best;
        b.w = sum - best;
      } else {
        const hh = nearest(HEIGHTS, (ly - top + GAP) / (ROW + GAP));
        if (hh === row.h) return;
        row.h = hh;
      }
      this.preview = next;
      this.render();
    };
    const end = () => {
      handle.removeEventListener('pointermove', move);
      handle.removeEventListener('pointerup', end);
      handle.removeEventListener('pointercancel', end);
      el.classList.remove('resizing');
      document.body.classList.remove('grid-resizing', 'resize-' + dir);
      const done = this.preview;
      this.preview = null;
      this.commit(done);
    };
    handle.addEventListener('pointermove', move);
    handle.addEventListener('pointerup', end);
    handle.addEventListener('pointercancel', end);
  }

  /** Stores the visible rows (hidden blocks keep a row where they were). */
  commit(rows) {
    const before = JSON.stringify(this.current());
    this.rows = this.withHidden(rows);
    this.render();
    if (JSON.stringify(this.current()) !== before) this.onChange(this.current());
  }
}
