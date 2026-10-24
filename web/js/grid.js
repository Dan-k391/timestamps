// Grid layout in the style of react-grid-layout: blocks have x/y/w/h in
// grid units on a 12-column grid. Dragging a block moves it to the cell
// under the pointer and pushes the blocks it overlaps down; everything then
// floats up to fill gaps (vertical compaction). Resizing snaps to whole
// cells within per-block limits. Blocks with autoHeight (the timeline) take
// the height of their content. A block is picked up by pressing on any empty
// part of it (not on buttons, rows, inputs or the timeline) and resized by
// dragging its left, right or bottom border.
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

const overlap = (a, b) => a.x < b.x + b.w && a.x + a.w > b.x && a.y < b.y + b.h && a.y + a.h > b.y;

/**
 * Places `fixed` exactly where it is and packs every other block as high as
 * it can go without overlapping what is already placed (top to bottom), so
 * blocks under `fixed` are pushed down and gaps close up.
 */
export function compact(items, fixed) {
  const placed = [];
  const out = items.map((i) => ({ ...i }));
  if (fixed) {
    const f = out.find((i) => i.id === fixed.id);
    Object.assign(f, fixed);
    placed.push(f);
  }
  const rest = out.filter((i) => !fixed || i.id !== fixed.id).sort((a, b) => a.y - b.y || a.x - b.x);
  for (const it of rest) {
    it.x = Math.max(0, Math.min(COLS - it.w, it.x));
    while (it.y > 0 && !placed.some((p) => overlap(p, { ...it, y: it.y - 1 }))) it.y--;
    for (let c = placed.find((p) => overlap(p, it)); c; c = placed.find((p) => overlap(p, it))) it.y = c.y + c.h;
    placed.push(it);
  }
  if (fixed) {
    // The moved block floats up into free space too, like the others.
    const f = out.find((i) => i.id === fixed.id);
    let y = f.y;
    while (y > 0 && !out.some((o) => o !== f && overlap(o, { ...f, y: y - 1 }))) y--;
    if (y !== f.y) return compact(items, { ...fixed, y });
  }
  return out;
}

export class Board {
  /**
   * blocks: [{ id, title, el, x, y, w, h, minW, minH, autoHeight }]
   * layout: saved [{ id, x, y, w, h }] (may be empty or stale)
   * onChange(layout) is called after every drag or resize.
   */
  constructor(host, blocks, layout, onChange) {
    this.host = host;
    this.onChange = onChange;
    this.blocks = new Map(blocks.map((b) => [b.id, b]));
    this.defaults = blocks.map((b) => ({ id: b.id, x: b.x, y: b.y, w: b.w, h: b.h }));
    this.hidden = new Set();
    this.items = new Map();
    host.classList.add('grid');
    this.holder = h('div', { class: 'gi-holder', hidden: true });
    host.append(this.holder);
    for (const b of blocks) {
      const el = h('div', { class: 'gi' + (b.autoHeight ? ' auto' : ''), dataset: { id: b.id } }, b.el);
      for (const dir of b.autoHeight ? ['w', 'e'] : ['w', 'e', 's', 'se', 'sw']) {
        const edge = h('div', { class: 'gi-edge ' + dir });
        edge.addEventListener('pointerdown', (e) => this.startResize(e, b.id, edge, dir));
        el.append(edge);
      }
      el.addEventListener('pointerdown', (e) => this.press(e, b.id));
      host.append(el);
      this.items.set(b.id, el);
      if (b.autoHeight) {
        new ResizeObserver(() => this.fitAuto(b.id)).observe(b.el);
      }
    }
    this.layout = this.sanitize(layout);
    this.ro = new ResizeObserver(() => this.render());
    this.ro.observe(host);
    this.render();
    // Animate moves only after the first placement.
    setTimeout(() => host.classList.add('ready'), 60);
  }

  sanitize(layout) {
    const out = [];
    const seen = new Set();
    for (const it of layout || []) {
      const b = this.blocks.get(it.id);
      if (!b || seen.has(it.id) || ![it.x, it.y, it.w, it.h].every(Number.isFinite)) continue;
      seen.add(it.id);
      const w = Math.max(b.minW || 1, Math.min(COLS, Math.round(it.w)));
      out.push({ id: it.id, x: Math.max(0, Math.min(COLS - w, Math.round(it.x))), y: Math.max(0, Math.round(it.y)), w, h: Math.max(b.minH || 1, Math.round(it.h)) });
    }
    if (!out.length) return this.defaults.map((x) => ({ ...x }));
    let bottom = Math.max(0, ...out.map((i) => i.y + i.h));
    for (const d of this.defaults) if (!seen.has(d.id)) { out.push({ ...d, y: bottom }); bottom += d.h; }
    return compact(out);
  }

  reset() {
    this.layout = compact(this.defaults.map((x) => ({ ...x })));
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
    const b = this.blocks.get(id);
    const px = b.el.offsetHeight;
    if (!px) return;
    const rows = Math.max(1, Math.ceil((px + GAP) / (ROW + GAP)));
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
        el.hidden = false;
        el.style.transform = `translate(0px, ${top}px)`;
        el.style.width = '100%';
        el.style.height = this.blocks.get(it.id).autoHeight ? '' : this.px(it).height + 'px';
        top += (this.blocks.get(it.id).autoHeight ? el.offsetHeight : this.px(it).height) + GAP;
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
    const grip = el;
    try { grip.setPointerCapture(e.pointerId); } catch (_) { /* best effort */ }
    window.getSelection && window.getSelection().removeAllRanges();
    const base = this.visible();
    const me = base.find((i) => i.id === id);
    const start = this.px(me);
    const hr0 = this.host.getBoundingClientRect();
    const offX = e.clientX - hr0.left - start.left, offY = e.clientY - hr0.top - start.top;
    const sc = this.host.closest('.main');
    let px = firstMove.clientX, py = firstMove.clientY;
    let cell = { x: me.x, y: me.y };
    let preview = base;
    el.classList.add('dragging');
    document.body.classList.add('grid-dragging');
    this.showHolder(me);

    const place = () => {
      const hr = this.host.getBoundingClientRect();
      const left = px - hr.left - offX, top = py - hr.top - offY;
      el.style.transform = `translate(${left}px, ${top}px)`;
      const x = Math.max(0, Math.min(COLS - me.w, Math.round(left / (this.colW + GAP))));
      const y = Math.max(0, Math.round(top / (ROW + GAP)));
      if (x === cell.x && y === cell.y) return;
      cell = { x, y };
      preview = compact(base, { ...me, x, y });
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
      grip.removeEventListener('pointermove', move);
      grip.removeEventListener('pointerup', end);
      grip.removeEventListener('pointercancel', end);
      clearTimeout(timer);
      el.classList.remove('dragging');
      document.body.classList.remove('grid-dragging');
      this.holder.hidden = true;
      this.commit(preview);
    };
    grip.addEventListener('pointermove', move);
    grip.addEventListener('pointerup', end);
    grip.addEventListener('pointercancel', end);
  }

  startResize(e, id, handle, dir) {
    if (e.button !== 0 || this.host.classList.contains('narrow')) return;
    e.preventDefault();
    e.stopPropagation();
    try { handle.setPointerCapture(e.pointerId); } catch (_) { /* best effort */ }
    const b = this.blocks.get(id);
    const el = this.items.get(id);
    const base = this.visible();
    const me = base.find((i) => i.id === id);
    const start = this.px(me);
    const sx = e.clientX, sy = e.clientY;
    let preview = base;
    el.classList.add('resizing');
    document.body.classList.add('grid-resizing', 'resize-' + dir);
    this.showHolder(me);
    let size0 = { x: me.x, w: me.w, h: me.h };
    const move = (ev) => {
      const dx = ev.clientX - sx, dy = ev.clientY - sy;
      const west = dir.includes('w'), east = dir.includes('e'), south = dir.includes('s');
      const wpx = Math.max(60, start.width + (east ? dx : west ? -dx : 0));
      const hpx = Math.max(60, start.height + (south ? dy : 0));
      // The block follows the pointer; the placeholder shows the snapped size.
      el.style.width = wpx + 'px';
      if (south) el.style.height = hpx + 'px';
      if (west) el.style.transform = `translate(${start.left + start.width - wpx}px, ${start.top}px)`;
      const right = me.x + me.w;
      let w = Math.max(b.minW || 1, Math.round((wpx + GAP) / (this.colW + GAP)));
      w = Math.min(w, west ? right : COLS - me.x);
      const x = west ? right - w : me.x;
      const hh = south ? Math.max(b.minH || 1, Math.round((hpx + GAP) / (ROW + GAP))) : me.h;
      if (x === size0.x && w === size0.w && hh === size0.h) return;
      size0 = { x, w, h: hh };
      preview = compact(base, { ...me, ...size0 });
      this.render(preview, id);
      this.showHolder(preview.find((i) => i.id === id));
    };
    const end = () => {
      handle.removeEventListener('pointermove', move);
      handle.removeEventListener('pointerup', end);
      handle.removeEventListener('pointercancel', end);
      el.classList.remove('resizing');
      document.body.classList.remove('grid-resizing', 'resize-' + dir);
      this.holder.hidden = true;
      this.commit(preview);
    };
    handle.addEventListener('pointermove', move);
    handle.addEventListener('pointerup', end);
    handle.addEventListener('pointercancel', end);
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
    if (changed) this.onChange(this.current());
  }
}
