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
   * Layout after dropping block id onto target, iPhone-style reflow: the
   * block takes the target's place in reading order (and its width, when
   * allowed). Moving up pushes the target and what follows down; moving down
   * lets them shift back into the gap. Within one row the two just swap.
   * Rows whose members changed re-fit to stay full; other rows (including
   * ones sized by hand) are left alone. target null means "at the end".
   */
  placed(base, id, target) {
    const rowsBase = toRows(base);
    const mates = (rows) => {
      const m = new Map();
      for (const r of rows) { const key = r.map((x) => x.id).join(','); for (const x of r) m.set(x.id, key); }
      return m;
    };
    const before = mates(rowsBase);
    const ri = rowsBase.findIndex((r) => r.some((x) => x.id === id));
    if (target && rowsBase[ri].some((x) => x.id === target)) {
      const row = rowsBase[ri].map((x) => ({ ...x }));
      const [me] = row.splice(row.findIndex((x) => x.id === id), 1);
      const forward = base.findIndex((x) => x.id === id) < base.findIndex((x) => x.id === target);
      row.splice(row.findIndex((x) => x.id === target) + (forward ? 1 : 0), 0, me);
      rowsBase[ri] = row;
      return fromRows(rowsBase);
    }
    const me = { id, w: base.find((x) => x.id === id).w };
    const flat = base.filter((x) => x.id !== id).map((x) => ({ id: x.id, w: x.w }));
    if (target) {
      const ti = base.findIndex((x) => x.id === target);
      const tw = SPAN[base[ti].w];
      const allowed = this.allowed(id);
      me.w = W_OF[allowed.includes(tw) ? tw : allowed.find((x) => x >= tw) || allowed[allowed.length - 1]];
      flat.splice(ti, 0, me); // before the target when moving up, after it when moving down
    } else {
      flat.push(me);
    }
    const rows = [];
    let used = 6;
    for (const it of flat) {
      const span = SPAN[it.w];
      if (used + span > 6) { rows.push([]); used = 0; }
      rows[rows.length - 1].push(it);
      used += span;
    }
    const after = mates(rows);
    return fromRows(rows.map((r) => {
      if (!r.some((x) => before.get(x.id) !== after.get(x.id))) return r;
      const lock = r.length > 1 && r.some((x) => x.id === id) ? new Set([id]) : new Set();
      return this.fit(r, lock) || this.fit(r, new Set()) || r;
    }));
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
    const els = layout.map((it) => {
      const el = holder && holder.id === it.id ? holder.el : this.wraps.get(it.id);
      const col = `${it.br ? '1 / ' : ''}span ${SPAN[it.w]}`;
      if (el.style.gridColumn !== col) el.style.gridColumn = col;
      el.dataset.w = it.w;
      return el;
    });
    // Only move elements that are out of place: moving big blocks around
    // the DOM is what makes dragging stutter.
    els.forEach((el, i) => {
      const at = this.host.children[i];
      if (at !== el) this.host.insertBefore(el, at || null);
    });
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

    let decision; // undefined: not moved yet; { target } once over a block
    // A new target is only taken once the pointer has moved a deliberate
    // distance (in page coordinates) since the last change, so blocks that
    // slide under a resting pointer don't make the layout flip back and forth.
    let lastChange = 0, lastX = px, lastY = py, lastScroll = 0;
    let rects = this.rectsExcept(wrap);
    const sc = this.host.closest('.main');
    const onScroll = () => { rects = this.rectsExcept(wrap); };
    sc && sc.addEventListener('scroll', onScroll);

    const decide = () => {
      for (const [oid, r] of rects) {
        if (px >= r.left && px <= r.right && py >= r.top && py <= r.bottom) return { target: oid };
      }
      const all = [...rects.values()];
      if (all.length && py > Math.max(...all.map((r) => r.bottom))) return { target: null };
      return decision; // over its own placeholder or a gap: keep the choice
    };

    const update = () => {
      const d = decide();
      if (!d || (decision && d.target === decision.target)) return;
      const now = performance.now();
      const scroll = sc ? sc.scrollTop : 0;
      if (decision !== undefined && (now - lastChange < 120 || Math.hypot(px - lastX, py + scroll - (lastY + lastScroll)) < 24)) return;
      decision = d;
      lastChange = now;
      lastX = px;
      lastY = py;
      lastScroll = scroll;
      const next = this.placed(base, id, d.target);
      holder.dataset.w = next.find((x) => x.id === id).w;
      this.animate(() => this.apply(next, { id, el: holder }), wrap);
      rects = this.rectsExcept(wrap);
    };

    let scrollTimer = 0;
    const autoScroll = () => {
      if (!sc) return;
      const r = sc.getBoundingClientRect();
      // Only once the block has really been dragged: grabbing a block near
      // the top of the window must not start scrolling by itself.
      const dragged = Math.hypot(px - sx, py - sy) > 30;
      const v = !dragged ? 0 : py < r.top + 60 ? -14 : py > r.bottom - 60 ? 14 : 0;
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
      const final = decision ? this.placed(base, id, decision.target) : base;
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
