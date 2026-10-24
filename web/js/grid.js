// Grid layout in the style of react-grid-layout, with little freedom: blocks
// are 1/3, 1/2, 2/3 or full width (4/6/8/12 of 12 columns) and sit in the
// few column slots that line up for that width; heights come from a short
// per-block list. Dragging a block moves it to the nearest slot and pushes
// the blocks it overlaps down; everything floats up to fill gaps. Vertically
// a dragged block only snaps to the top or bottom edge of another block.
// A block is picked up by pressing on any empty part of it and resized by
// dragging its left, right or bottom border, which jumps between the allowed
// sizes; a border shared with the block beside it moves for both. The
// timeline (autoHeight) takes the height of its content.
import { h } from './ui.js';

export const COLS = 12;
const ROW = 30; // px per grid row
const GAP = 14; // px between blocks
const NARROW = 860; // below this width blocks simply stack

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

// Column slots per width so blocks line up (e.g. a half is left or right).
const SLOTS = { 4: [0, 4, 8], 6: [0, 6], 8: [0, 4], 12: [0] };
const nearest = (list, v) => list.reduce((best, x) => (Math.abs(x - v) < Math.abs(best - v) ? x : best), list[0]);
const slotX = (w, x) => nearest(SLOTS[w], x);

const overlap = (a, b) => a.x < b.x + b.w && a.x + a.w > b.x && a.y < b.y + b.h && a.y + a.h > b.y;

/**
 * Places `fixed` exactly where it is and packs every other block as high as
 * it can go without overlapping what is already placed (top to bottom).
 * A block that `fixed` lands on first steps sideways into a free slot
 * (preferring the side `fixed` came from, vacatedX), otherwise it is pushed
 * down; gaps close up.
 */
export function compact(items, fixed, vacatedX) {
  const placed = [];
  const out = items.map((i) => ({ ...i }));
  if (fixed) {
    const f = out.find((i) => i.id === fixed.id);
    Object.assign(f, fixed);
    placed.push(f);
  }
  const rest = out.filter((i) => !fixed || i.id !== fixed.id).sort((a, b) => a.y - b.y || a.x - b.x);
  const f0 = fixed && out.find((i) => i.id === fixed.id);
  for (const it of rest) {
    it.x = Math.max(0, Math.min(COLS - it.w, it.x));
    // Float up. A block may pass behind the dragged block (so the two swap
    // places), but never through any other block.
    let top = it.y;
    for (let y = it.y - 1; y >= 0; y--) {
      const blockers = placed.filter((p) => overlap(p, { ...it, y }));
      if (!blockers.length) top = y;
      else if (!blockers.every((b) => b === f0)) break;
    }
    it.y = top;
    if (f0 && overlap(f0, it) && SLOTS[it.w]) {
      const to = vacatedX ?? it.x;
      const xs = [...SLOTS[it.w]].sort((a, b) => Math.abs(a - to) - Math.abs(b - to));
      const x = xs.find((cx) => !placed.some((p) => overlap(p, { ...it, x: cx })));
      if (x !== undefined) it.x = x;
    }
    for (let c = placed.find((p) => overlap(p, it)); c; c = placed.find((p) => overlap(p, it))) it.y = c.y + c.h;
    placed.push(it);
  }
  return out;
}

/** Layout after putting `fixed` somewhere: the others make room, then
 *  everything (the moved block too) settles upwards, as it will on drop. */
export function settle(items, fixed, vacatedX) {
  return compact(compact(items, fixed, vacatedX));
}

export class Board {
  /**
   * blocks: [{ id, title, el, x, w, h, widths: [8, 12], heights: [8, 12], autoHeight }]
   * layout: saved [{ id, x, y, w, h }] (may be empty or stale)
   * onChange(layout) is called after every drag or resize.
   */
  constructor(host, blocks, layout, onChange) {
    this.host = host;
    this.onChange = onChange;
    this.blocks = new Map(blocks.map((b) => [b.id, b]));
    this.defaults = compact(blocks.map((b, i) => ({ id: b.id, x: b.x, y: b.y ?? i * 100, w: b.w, h: b.h })));
    this.hidden = new Set();
    this.items = new Map();
    this.fill = new Map(); // id -> px added under a fit-to-content block
    host.classList.add('grid');
    this.holder = h('div', { class: 'gi-holder', hidden: true });
    host.append(this.holder);
    for (const b of blocks) {
      const el = h('div', { class: 'gi' + (b.autoHeight ? ' auto' : ''), dataset: { id: b.id } }, b.el);
      const dirs = [];
      if (b.widths.length > 1) dirs.push('w', 'e');
      if (!b.autoHeight && b.heights.length > 1) dirs.push('s');
      if (dirs.includes('e') && dirs.includes('s')) dirs.push('se', 'sw');
      for (const dir of dirs) {
        const edge = h('div', { class: 'gi-edge ' + dir });
        edge.addEventListener('pointerdown', (e) => this.startResize(e, b.id, edge, dir));
        el.append(edge);
      }
      el.addEventListener('pointerdown', (e) => this.press(e, b.id));
      host.append(el);
      this.items.set(b.id, el);
      if (b.autoHeight) new ResizeObserver(() => this.fitAuto(b.id)).observe(b.el);
    }
    this.layout = this.sanitize(layout);
    this.ro = new ResizeObserver(() => this.render());
    this.ro.observe(host);
    this.render();
    // Animate moves only after the first placement.
    setTimeout(() => host.classList.add('ready'), 60);
  }

  /** Snaps saved positions and sizes to what each block allows. */
  sanitize(layout) {
    const out = [];
    const seen = new Set();
    for (const it of layout || []) {
      const b = this.blocks.get(it.id);
      if (!b || seen.has(it.id) || ![it.x, it.y, it.w, it.h].every(Number.isFinite)) continue;
      seen.add(it.id);
      const w = nearest(b.widths, it.w);
      const hh = b.autoHeight ? it.h : nearest(b.heights, it.h);
      out.push({ id: it.id, x: slotX(w, it.x), y: Math.max(0, Math.round(it.y)), w, h: hh });
    }
    if (!out.length) return this.defaults.map((x) => ({ ...x }));
    let bottom = Math.max(0, ...out.map((i) => i.y + i.h));
    for (const d of this.defaults) if (!seen.has(d.id)) { out.push({ ...d, y: bottom }); bottom += d.h; }
    return compact(out);
  }

  reset() {
    this.layout = this.defaults.map((x) => ({ ...x }));
    for (const id of this.blocks.keys()) if (this.blocks.get(id).autoHeight) this.fitAuto(id, true);
    this.render();
    this.onChange(this.current());
  }

  current() { return this.layout.map(({ id, x, y, w, h: hh }) => ({ id, x, y, w, h: hh })); }

  /** Layout without hidden blocks, compacted. */
  visible(layout = this.layout) {
    return compact(layout.filter((i) => !this.hidden.has(i.id)));
  }

  setHidden(id, hidden) {
    if (hidden === this.hidden.has(id)) return;
    if (hidden) this.hidden.add(id); else this.hidden.delete(id);
    this.render();
  }

  /** Grows or shrinks an autoHeight block to its content. */
  fitAuto(id, quiet) {
    if (this.busy) return;
    const b = this.blocks.get(id);
    // The padding is hidden while the blocks are stacked (narrow).
    const fill = this.host.classList.contains('narrow') ? 0 : this.fill.get(id) || 0;
    const px = b.el.offsetHeight - fill;
    if (px <= 0) return;
    const rows = Math.max(1, Math.ceil((px + GAP) / (ROW + GAP)));
    // Pad it to the bottom of its last row so the gap below is the usual one.
    const want = rows * ROW + (rows - 1) * GAP - px;
    if (want !== fill) {
      this.fill.set(id, want);
      b.el.style.setProperty('--fill', want + 'px');
    }
    const it = this.layout.find((i) => i.id === id);
    if (!it || it.h === rows) return;
    it.h = rows;
    this.layout = compact(this.layout);
    if (!quiet) this.render();
  }

  get colW() { return (this.host.clientWidth - GAP * (COLS - 1)) / COLS; }
  px(it) {
    const cw = this.colW;
    return { left: it.x * (cw + GAP), top: it.y * (ROW + GAP), width: it.w * cw + (it.w - 1) * GAP, height: it.h * ROW + (it.h - 1) * GAP };
  }

  render(layout = this.visible(), skip) {
    const narrow = this.host.clientWidth < NARROW;
    this.host.classList.toggle('narrow', narrow);
    if (narrow) {
      // Stack in reading order; no dragging.
      let top = 0;
      for (const it of [...layout].sort((a, b) => a.y - b.y || a.x - b.x)) {
        const el = this.items.get(it.id);
        const auto = this.blocks.get(it.id).autoHeight;
        el.hidden = false;
        el.style.transform = `translate(0px, ${top}px)`;
        el.style.width = '100%';
        el.style.height = auto ? '' : this.px(it).height + 'px';
        top += (auto ? el.offsetHeight : this.px(it).height) + GAP;
      }
      for (const id of this.hidden) this.items.get(id).hidden = true;
      this.host.style.height = Math.max(0, top - GAP) + 'px';
      return;
    }
    let bottom = 0;
    for (const it of layout) {
      const el = this.items.get(it.id);
      el.hidden = false;
      const p = this.px(it);
      bottom = Math.max(bottom, p.top + p.height);
      if (it.id === skip) continue;
      el.style.transform = `translate(${p.left}px, ${p.top}px)`;
      el.style.width = p.width + 'px';
      if (!this.blocks.get(it.id).autoHeight) el.style.height = p.height + 'px';
    }
    for (const id of this.hidden) this.items.get(id).hidden = true;
    this.host.style.height = bottom + 'px';
  }

  showHolder(it) {
    const p = this.px(it);
    const hd = this.holder;
    hd.hidden = false;
    hd.style.transform = `translate(${p.left}px, ${p.top}px)`;
    hd.style.width = p.width + 'px';
    hd.style.height = p.height + 'px';
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
    this.busy = true;
    const base = this.visible();
    const me = base.find((i) => i.id === id);
    const start = this.px(me);
    const hr0 = this.host.getBoundingClientRect();
    const offX = e.clientX - hr0.left - start.left, offY = e.clientY - hr0.top - start.top;
    const sc = this.host.closest('.main');
    // Vertical places a block can go: the top or bottom edge of another block.
    const others = base.filter((i) => i.id !== id);
    const ys = [...new Set([0, ...others.map((o) => o.y), ...others.map((o) => o.y + o.h)])];
    let px = firstMove.clientX, py = firstMove.clientY;
    let cell = { x: me.x, y: me.y };
    let preview = base;
    el.classList.add('dragging');
    document.body.classList.add('grid-dragging');
    this.showHolder(me);

    const cellPx = (c) => ({ left: c.x * (this.colW + GAP), top: c.y * (ROW + GAP) });
    const place = () => {
      const hr = this.host.getBoundingClientRect();
      const left = px - hr.left - offX, top = py - hr.top - offY;
      el.style.transform = `translate(${left}px, ${top}px) scale(1.01)`;
      const x = slotX(me.w, left / (this.colW + GAP));
      const y = nearest(ys, top / (ROW + GAP));
      if (x === cell.x && y === cell.y) return;
      // Dead zone: only switch when clearly closer to the new place.
      const cur = cellPx(cell), nxt = cellPx({ x, y });
      const dCur = Math.hypot(left - cur.left, top - cur.top), dNew = Math.hypot(left - nxt.left, top - nxt.top);
      if (dNew > dCur - 40) return;
      cell = { x, y };
      preview = settle(base, { ...me, x, y }, me.x);
      this.render(preview, id);
      this.showHolder(preview.find((i) => i.id === id));
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
      this.holder.hidden = true;
      this.busy = false;
      this.commit(preview);
    };
    el.addEventListener('pointermove', move);
    el.addEventListener('pointerup', end);
    el.addEventListener('pointercancel', end);
  }

  /**
   * Border resize: the block jumps between its allowed widths/heights
   * (nearest to the pointer), keeping the opposite edge in place.
   */
  startResize(e, id, handle, dir) {
    if (e.button !== 0 || this.host.classList.contains('narrow')) return;
    e.preventDefault();
    e.stopPropagation();
    try { handle.setPointerCapture(e.pointerId); } catch (_) { /* best effort */ }
    const b = this.blocks.get(id);
    const el = this.items.get(id);
    this.busy = true;
    const base = this.visible();
    const me = base.find((i) => i.id === id);
    const start = this.px(me);
    const sx = e.clientX, sy = e.clientY;
    let preview = base;
    let size = { x: me.x, w: me.w, h: me.h };
    el.classList.add('resizing');
    document.body.classList.add('grid-resizing', 'resize-' + dir);
    const unit = () => this.colW + GAP;
    const move = (ev) => {
      const dx = ev.clientX - sx, dy = ev.clientY - sy;
      let next = { ...size };
      if (dir.includes('e') || dir.includes('w')) {
        const west = dir.includes('w');
        // Where the moving edge is, in columns.
        const edge = west ? (start.left + dx) / unit() : (start.left + start.width + GAP + dx) / unit();
        const right = me.x + me.w;
        let best = null;
        for (const w of b.widths) {
          const x = west ? right - w : me.x;
          if (x < 0 || x + w > COLS || !SLOTS[w].includes(x)) continue;
          if (!this.shareBorder(base, me, { x, w }, dir)) continue;
          const d = Math.abs((west ? x : x + w) - edge);
          if (!best || d < best.d) best = { d, x, w };
        }
        if (best) next = { ...next, x: best.x, w: best.w };
      }
      if (dir.includes('s')) {
        const rows = (start.height + GAP + dy) / (ROW + GAP);
        next.h = nearest(b.heights, rows);
      }
      if (next.x === size.x && next.w === size.w && next.h === size.h) return;
      size = next;
      preview = settle(this.shareBorder(base, me, size, dir) || base, { ...me, ...size }, me.x);
      this.render(preview);
    };
    const end = () => {
      handle.removeEventListener('pointermove', move);
      handle.removeEventListener('pointerup', end);
      handle.removeEventListener('pointercancel', end);
      el.classList.remove('resizing');
      document.body.classList.remove('grid-resizing', 'resize-' + dir);
      this.busy = false;
      this.commit(preview);
    };
    handle.addEventListener('pointermove', move);
    handle.addEventListener('pointerup', end);
    handle.addEventListener('pointercancel', end);
  }

  /**
   * A border shared with a block beside it moves for both, so the row stays
   * filled: that block gives (or takes up) the columns this one takes (or
   * gives). Taking all of it pushes it down instead. Returns null when the
   * block beside can't have the width that would leave it.
   */
  shareBorder(base, me, size, dir) {
    if (size.w === me.w) return base;
    const east = dir.includes('e');
    const edge = east ? me.x + me.w : me.x;
    const out = [];
    for (const n of base) {
      if (n.id === me.id || n.y >= me.y + me.h || n.y + n.h <= me.y || (east ? n.x !== edge : n.x + n.w !== edge)) {
        out.push(n);
        continue;
      }
      const x = east ? size.x + size.w : n.x;
      const w = east ? n.x + n.w - x : size.x - n.x;
      if (w <= 0) { out.push(n); continue; }
      if (!this.blocks.get(n.id).widths.includes(w) || !SLOTS[w].includes(x)) return null;
      out.push({ ...n, x, w });
    }
    return out;
  }

  /** Stores a visible layout (keeping hidden blocks' saved places). */
  commit(visible) {
    const byId = new Map(visible.map((i) => [i.id, i]));
    const changed = this.layout.some((i) => {
      const v = byId.get(i.id);
      return v && (v.x !== i.x || v.y !== i.y || v.w !== i.w || v.h !== i.h);
    });
    this.layout = compact(this.layout.map((i) => byId.get(i.id) || i));
    this.render();
    for (const id of this.blocks.keys()) if (this.blocks.get(id).autoHeight) this.fitAuto(id);
    if (changed) this.onChange(this.current());
  }
}
