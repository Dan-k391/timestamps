// Board: blocks on a 6-column grid that the user arranges by dragging.
// Widths are fixed steps (1/3, 1/2, 2/3, full, limited per block); rows
// re-fit automatically when a block is dropped or resized so they stay full.
// Dragging only moves a lifted copy (transforms) and a light placeholder, so
// heavy blocks (timeline, lists) are not re-laid out on every mouse move.
import { h, icon, dropdown } from './ui.js';
import { t } from './i18n.js';

const SPAN = { '1/3': 2, '1/2': 3, '2/3': 4, full: 6 };
const W_OF = { 2: '1/3', 3: '1/2', 4: '2/3', 6: 'full' };
const label = (w) => ({ '1/3': '⅓', '1/2': '½', '2/3': '⅔' }[w] || t('Full'));

/** Splits a flat layout ({ id, w, br }) into rows; br marks a row start. */
function toRows(layout) {
  const rows = [];
  let used = 6;
  for (const it of layout) {
    const span = SPAN[it.w];
    if (it.br || used + span > 6 || !rows.length) {
      rows.push([]);
      used = 0;
    }
    rows[rows.length - 1].push({ id: it.id, w: it.w });
    used += span;
  }
  return rows;
}

function fromRows(rows) {
  const out = [];
  for (const r of rows) r.forEach((it, i) => out.push({ id: it.id, w: it.w, br: i === 0 }));
  return out;
}

export class Board {
  /**
   * blocks: [{ id, title, el, widths: ['1/2', 'full', ...], w: default width }]
   * layout: saved [{ id, w, br }] (may be empty or stale)
   * onChange(layout) is called after every drop or resize.
   */
  constructor(host, blocks, layout, onChange) {
    this.host = host;
    this.onChange = onChange;
    this.blocks = new Map(blocks.map((b) => [b.id, b]));
    this.defaults = this.fitAll(blocks.map((b) => ({ id: b.id, w: b.w })));
    this.wraps = new Map();
    host.classList.add('board');
    for (const b of blocks) {
      const tools = h('div', { class: 'blk-tools' });
      const wrap = h('div', { class: 'blk', dataset: { id: b.id } }, tools, b.el);
      this.wraps.set(b.id, wrap);
    }
    this.apply(this.sanitize(layout));
  }

  // ---- layout model ---------------------------------------------------

  allowed(id) { return this.blocks.get(id).widths.map((w) => SPAN[w]); }

  /** Drops unknown blocks, fixes widths and appends new blocks. */
  sanitize(layout) {
    const seen = new Set();
    const out = [];
    for (const it of layout || []) {
      const b = this.blocks.get(it.id);
      if (!b || seen.has(it.id)) continue;
      seen.add(it.id);
      out.push({ id: it.id, w: b.widths.includes(it.w) ? it.w : b.w, br: it.br });
    }
    if (!out.length) return this.defaults.map((x) => ({ ...x }));
    for (const d of this.defaults) if (!seen.has(d.id)) out.push({ ...d, br: true });
    return fromRows(toRows(out));
  }

  fitAll(layout) {
    return fromRows(toRows(layout).map((r) => this.fit(r, new Set()) || r));
  }

  /**
   * Picks widths for a row so it adds up to the full width, changing as
   * little as possible. locked ids keep their width; weight 0 ids (a block
   * being dropped) adapt first. Returns null when the row cannot fit.
   */
  fit(row, locked, light = new Set()) {
    const opts = row.map((it) => (locked.has(it.id) ? [SPAN[it.w]] : this.allowed(it.id)));
    let best = null;
    const cur = new Array(row.length);
    const walk = (i, sum) => {
      if (sum > 6) return;
      if (i === row.length) {
        let score = (6 - sum) * 100;
        row.forEach((it, k) => { score += Math.abs(cur[k] - SPAN[it.w]) * (light.has(it.id) ? 0.2 : 1); });
        if (!best || score < best.score) best = { score, spans: [...cur] };
        return;
      }
      for (const s of opts[i]) { cur[i] = s; walk(i + 1, sum + s); }
    };
    walk(0, 0);
    if (!best) return null;
    return row.map((it, k) => ({ id: it.id, w: W_OF[best.spans[k]] }));
  }

  /**
   * Layout after dropping block id next to target ('left' | 'right') or in
   * its own row ('above' | 'below').
   */
  placed(base, id, target, side) {
    const rows = toRows(base).map((r) => r.map((x) => ({ ...x })));
    const me = { id, w: base.find((x) => x.id === id).w };
    let ri = rows.findIndex((r) => r.some((x) => x.id === id));
    // Moving within its own row just reorders; widths stay as they are.
    if ((side === 'left' || side === 'right') && rows[ri].some((x) => x.id === target)) {
      const row = rows[ri].filter((x) => x.id !== id);
      row.splice(row.findIndex((x) => x.id === target) + (side === 'right' ? 1 : 0), 0, me);
      rows[ri] = row;
      return fromRows(rows);
    }
    // Take the block out; the row it leaves re-fits to stay full.
    rows[ri] = rows[ri].filter((x) => x.id !== id);
    if (!rows[ri].length) rows.splice(ri, 1);
    else rows[ri] = this.fit(rows[ri], new Set()) || rows[ri];
    const ti = rows.findIndex((r) => r.some((x) => x.id === target));
    if (ti < 0) { rows.push([{ id, w: 'full' }]); return fromRows(rows); }
    if (side === 'left' || side === 'right') {
      const row = [...rows[ti]];
      const k = row.findIndex((x) => x.id === target) + (side === 'right' ? 1 : 0);
      row.splice(k, 0, me);
      const fitted = this.fit(row, new Set(), new Set([id]));
      if (fitted) { rows[ti] = fitted; return fromRows(rows); }
      side = side === 'left' ? 'above' : 'below';
    }
    rows.splice(side === 'above' ? ti : ti + 1, 0, [{ id, w: 'full' }]);
    return fromRows(rows);
  }

  /** Layout after manually setting a block's width; its row mates adapt. */
  resized(base, id, w) {
    const rows = toRows(base).map((r) => r.map((x) => (x.id === id ? { ...x, w } : { ...x })));
    const ri = rows.findIndex((r) => r.some((x) => x.id === id));
    const fitted = this.fit(rows[ri], new Set([id]));
    if (fitted) rows[ri] = fitted;
    else {
      // Too wide to share the row: the others move to a row of their own.
      const others = rows[ri].filter((x) => x.id !== id);
      rows.splice(ri, 1, [{ id, w }], ...(others.length ? [this.fit(others, new Set()) || others] : []));
    }
    return fromRows(rows);
  }

  // ---- rendering --------------------------------------------------------

  apply(layout, holder) {
    this.layout = layout;
    for (const it of layout) {
      const el = holder && holder.id === it.id ? holder.el : this.wraps.get(it.id);
      el.style.gridColumn = `${it.br ? '1 / ' : ''}span ${SPAN[it.w]}`;
      el.dataset.w = it.w;
      this.host.append(el);
    }
    if (!holder) for (const it of layout) this.renderTools(it.id);
  }

  current() { return this.layout.map((x) => ({ ...x })); }

  renderTools(id) {
    const b = this.blocks.get(id);
    const wrap = this.wraps.get(id);
    const tools = wrap.querySelector('.blk-tools');
    tools.replaceChildren();
    const grip = h('button', { type: 'button', class: 'blk-grip', title: t('Drag to move') }, icon('grip', 14), h('span', null, b.title));
    grip.addEventListener('pointerdown', (e) => this.startDrag(e, id, grip));
    const w = wrap.dataset.w || b.w;
    const sizeBtn = h('button', { type: 'button', class: 'blk-size', title: t('Width') }, icon('width', 14), h('span', null, label(w)));
    sizeBtn.addEventListener('click', () => dropdown(sizeBtn, [
      ...b.widths.map((x) => ({ label: (x === w ? '✓ ' : '   ') + t('Width') + ' ' + label(x), onClick: () => this.resize(id, x) })),
      { separator: true },
      { label: t('Reset layout'), icon: 'refresh', onClick: () => this.commit(this.defaults.map((x) => ({ ...x }))) },
    ]));
    tools.append(grip, sizeBtn);
  }

  commit(layout) {
    this.animate(() => this.apply(layout));
    this.onChange(this.current());
  }

  resize(id, w) { this.commit(this.resized(this.layout, id, w)); }

  /** FLIP: runs mutate(), then slides moved blocks from old to new places. */
  animate(mutate, skip) {
    const els = [...this.host.children].filter((el) => el !== skip && !el.hidden);
    const first = new Map(els.map((el) => [el, el.getBoundingClientRect()]));
    mutate();
    const moved = [];
    for (const el of [...this.host.children]) {
      if (el === skip || el.hidden || !first.has(el)) continue;
      const a = first.get(el), b = el.getBoundingClientRect();
      const dx = a.left - b.left, dy = a.top - b.top;
      if (Math.abs(dx) < 1 && Math.abs(dy) < 1) continue;
      el.style.transition = 'none';
      el.style.transform = `translate(${dx}px, ${dy}px)`;
      moved.push(el);
    }
    if (!moved.length) return;
    void this.host.offsetHeight;
    for (const el of moved) {
      el.style.transition = 'transform 200ms cubic-bezier(.2, .8, .2, 1)';
      el.style.transform = '';
    }
    clearTimeout(this.flipTimer);
    this.flipTimer = setTimeout(() => moved.forEach((el) => { el.style.transition = ''; }), 240);
  }

  // ---- dragging ---------------------------------------------------------

  startDrag(e, id, grip) {
    if (e.button !== 0) return;
    e.preventDefault();
    try { grip.setPointerCapture(e.pointerId); } catch (_) { /* best effort */ }
    const wrap = this.wraps.get(id);
    const rect = wrap.getBoundingClientRect();
    const base = this.current();
    const offX = e.clientX - rect.left, offY = e.clientY - rect.top;
    const scale = Math.min(1, 440 / rect.width);

    // Placeholder takes the block's slot; the block itself floats.
    const holder = h('div', { class: 'blk-holder' });
    holder.style.height = rect.height + 'px';
    holder.style.gridColumn = wrap.style.gridColumn;
    this.host.insertBefore(holder, wrap);
    Object.assign(wrap.style, {
      position: 'fixed', left: rect.left + 'px', top: rect.top + 'px', width: rect.width + 'px', height: rect.height + 'px',
      zIndex: 90, transformOrigin: `${offX}px ${offY}px`, transition: 'transform 90ms ease', willChange: 'transform',
    });
    wrap.classList.add('lifted');
    document.body.classList.add('board-dragging');
    const sx = e.clientX, sy = e.clientY;
    let px = e.clientX, py = e.clientY;
    const follow = () => { wrap.style.transform = `translate(${px - sx}px, ${py - sy}px) scale(${scale})`; };
    follow();
    setTimeout(() => { wrap.style.transition = 'none'; }, 100);

    let decision = null, lastChange = 0, lastX = px, lastY = py;
    let rects = this.rectsExcept(wrap);
    const sc = this.host.closest('.main');
    const onScroll = () => { rects = this.rectsExcept(wrap); };
    sc && sc.addEventListener('scroll', onScroll);

    const decide = () => {
      let hit = null;
      for (const [oid, r] of rects) {
        if (px >= r.left && px <= r.right && py >= r.top && py <= r.bottom) { hit = { id: oid, r }; break; }
      }
      if (!hit) {
        // Below everything: last row; above everything: first row.
        const all = [...rects.values()];
        if (!all.length) return null;
        if (py > Math.max(...all.map((r) => r.bottom))) return { target: this.layout.filter((x) => x.id !== id).slice(-1)[0].id, side: 'below' };
        if (py < Math.min(...all.map((r) => r.top))) return { target: this.layout.find((x) => x.id !== id).id, side: 'above' };
        return decision; // over the placeholder or a gap: keep the current choice
      }
      const { r } = hit;
      const wide = r.width > this.host.clientWidth * 0.6;
      const fx = (px - r.left) / r.width, fy = (py - r.top) / r.height;
      let side;
      if (wide) side = fx < 0.22 ? 'left' : fx > 0.78 ? 'right' : fy < 0.5 ? 'above' : 'below';
      else side = fy < 0.18 ? 'above' : fy > 0.82 ? 'below' : fx < 0.5 ? 'left' : 'right';
      return { target: hit.id, side };
    };

    const update = () => {
      const d = decide();
      if (!d || (decision && d.target === decision.target && d.side === decision.side)) return;
      const now = performance.now();
      if (now - lastChange < 140 || Math.hypot(px - lastX, py - lastY) < 8) return;
      decision = d;
      lastChange = now;
      lastX = px;
      lastY = py;
      const next = this.placed(base, id, d.target, d.side);
      const mine = next.find((x) => x.id === id);
      holder.dataset.w = mine.w;
      this.animate(() => this.apply(next, { id, el: holder }), wrap);
      rects = this.rectsExcept(wrap);
    };

    let scrollTimer = 0;
    const autoScroll = () => {
      if (!sc) return;
      const r = sc.getBoundingClientRect();
      const v = py < r.top + 60 ? -14 : py > r.bottom - 60 ? 14 : 0;
      if (v) { sc.scrollTop += v; update(); }
      scrollTimer = setTimeout(autoScroll, 16);
    };
    autoScroll();

    const move = (ev) => {
      px = ev.clientX;
      py = ev.clientY;
      follow();
      update();
    };
    const end = () => {
      grip.removeEventListener('pointermove', move);
      grip.removeEventListener('pointerup', end);
      grip.removeEventListener('pointercancel', end);
      sc && sc.removeEventListener('scroll', onScroll);
      clearTimeout(scrollTimer);
      document.body.classList.remove('board-dragging');
      const final = decision ? this.placed(base, id, decision.target, decision.side) : base;
      // Glide into the placeholder, then put the block back in the grid.
      const hr = holder.getBoundingClientRect();
      wrap.style.transition = 'transform 160ms cubic-bezier(.2, .8, .2, 1)';
      wrap.style.transformOrigin = '0 0';
      wrap.style.transform = `translate(${hr.left - rect.left}px, ${hr.top - rect.top}px) scale(${hr.width / rect.width})`;
      setTimeout(() => {
        wrap.classList.remove('lifted');
        for (const k of ['position', 'left', 'top', 'width', 'height', 'zIndex', 'transformOrigin', 'transition', 'willChange', 'transform']) wrap.style[k] = '';
        holder.remove();
        this.apply(final);
        if (JSON.stringify(final) !== JSON.stringify(base)) this.onChange(this.current());
      }, 170);
    };
    grip.addEventListener('pointermove', move);
    grip.addEventListener('pointerup', end);
    grip.addEventListener('pointercancel', end);
  }

  // Final (layout) positions of the other blocks. offsetLeft/Top ignore the
  // transforms of blocks that are still sliding into place.
  rectsExcept(wrap) {
    const hr = this.host.getBoundingClientRect();
    const m = new Map();
    for (const [id, el] of this.wraps) {
      if (el === wrap || el.hidden) continue;
      const left = hr.left + el.offsetLeft, top = hr.top + el.offsetTop;
      m.set(id, { left, top, right: left + el.offsetWidth, bottom: top + el.offsetHeight, width: el.offsetWidth, height: el.offsetHeight });
    }
    return m;
  }

  /** Hides or shows a block (e.g. screenshots on days without any). */
  setHidden(id, hidden) {
    const wrap = this.wraps.get(id);
    if (wrap) wrap.hidden = hidden;
  }
}
