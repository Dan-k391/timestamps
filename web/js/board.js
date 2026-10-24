// Board: blocks the user can reorder by dragging and size to fixed widths
// (1/3, 1/2, 2/3, full) on a 6-column grid. No free resizing.
import { h, icon, dropdown } from './ui.js';
import { t } from './i18n.js';

const SPAN = { '1/3': 2, '1/2': 3, '2/3': 4, full: 6 };
const label = (w) => ({ '1/3': '⅓', '1/2': '½', '2/3': '⅔' }[w] || t('Full'));

export class Board {
  /**
   * blocks: [{ id, title, el, widths: ['1/2', 'full', ...], w: default width }]
   * layout: saved [{ id, w }] (may be empty or stale)
   * onChange(layout) is called after every drag or resize.
   */
  constructor(host, blocks, layout, onChange) {
    this.host = host;
    this.onChange = onChange;
    this.defaults = blocks.map((b) => ({ id: b.id, w: b.w }));
    this.blocks = new Map(blocks.map((b) => [b.id, b]));
    this.wraps = new Map();
    host.classList.add('board');
    for (const b of blocks) {
      const tools = h('div', { class: 'blk-tools' });
      const wrap = h('div', { class: 'blk', dataset: { id: b.id } }, tools, b.el);
      this.wraps.set(b.id, wrap);
      this.renderTools(b, tools);
    }
    this.apply(layout);
  }

  /** Puts blocks in the saved order with saved widths, adding new blocks at the end. */
  apply(layout) {
    const seen = new Set();
    const order = [];
    for (const item of layout || []) {
      const b = this.blocks.get(item.id);
      if (!b || seen.has(item.id)) continue;
      seen.add(item.id);
      order.push({ id: item.id, w: b.widths.includes(item.w) ? item.w : b.w });
    }
    for (const d of this.defaults) if (!seen.has(d.id)) order.push({ ...d });
    this.layout = order;
    for (const item of order) {
      const wrap = this.wraps.get(item.id);
      wrap.style.gridColumn = `span ${SPAN[item.w]}`;
      wrap.dataset.w = item.w;
      this.host.append(wrap);
    }
    for (const [id, b] of this.blocks) this.renderTools(b, this.wraps.get(id).querySelector('.blk-tools'));
  }

  current() {
    return [...this.host.children].filter((n) => n.classList.contains('blk')).map((n) => ({ id: n.dataset.id, w: n.dataset.w }));
  }

  renderTools(b, tools) {
    tools.replaceChildren();
    const grip = h('button', { type: 'button', class: 'blk-grip', title: t('Drag to move') }, icon('grip', 14), h('span', null, b.title));
    grip.addEventListener('pointerdown', (e) => this.startDrag(e, b.id, grip));
    const wrap = this.wraps.get(b.id);
    const w = (wrap && wrap.dataset.w) || b.w;
    const sizeBtn = h('button', { type: 'button', class: 'blk-size', title: t('Width') }, icon('width', 14), h('span', null, label(w)));
    sizeBtn.addEventListener('click', () => dropdown(sizeBtn, [
      ...b.widths.map((x) => ({ label: (x === w ? '✓ ' : '   ') + t('Width') + ' ' + label(x), onClick: () => this.resize(b.id, x) })),
      { separator: true },
      { label: t('Reset layout'), icon: 'refresh', onClick: () => { this.apply([]); this.onChange(this.current()); } },
    ]));
    tools.append(grip, sizeBtn);
  }

  resize(id, w) {
    const wrap = this.wraps.get(id);
    wrap.dataset.w = w;
    wrap.style.gridColumn = `span ${SPAN[w]}`;
    this.renderTools(this.blocks.get(id), wrap.querySelector('.blk-tools'));
    this.onChange(this.current());
  }

  startDrag(e, id, grip) {
    if (e.button !== 0) return;
    e.preventDefault();
    const wrap = this.wraps.get(id);
    try { grip.setPointerCapture(e.pointerId); } catch (_) { /* capture is best effort */ }
    const ghost = h('div', { class: 'blk-ghost' }, icon('grip', 14), this.blocks.get(id).title);
    document.getElementById('overlays').append(ghost);
    wrap.classList.add('dragging');
    document.body.classList.add('board-dragging');
    let lastX = e.clientX, lastY = e.clientY;
    const place = (ev) => {
      ghost.style.left = ev.clientX + 12 + 'px';
      ghost.style.top = ev.clientY + 12 + 'px';
      // Ignore tiny moves right after a reorder so blocks don't flip back and forth.
      if (Math.hypot(ev.clientX - lastX, ev.clientY - lastY) < 10) return;
      for (const other of this.wraps.values()) {
        if (other === wrap || other.hidden) continue;
        const r = other.getBoundingClientRect();
        if (ev.clientX < r.left || ev.clientX > r.right || ev.clientY < r.top || ev.clientY > r.bottom) continue;
        const wide = r.width > this.host.clientWidth * 0.8;
        const before = wide ? ev.clientY < r.top + r.height / 2 : ev.clientX < r.left + r.width / 2;
        const ref = before ? other : other.nextSibling;
        if (ref !== wrap && ref !== wrap.nextSibling) {
          this.host.insertBefore(wrap, ref);
          lastX = ev.clientX;
          lastY = ev.clientY;
        }
        break;
      }
      // Auto-scroll near the edges of the scrolling view.
      const sc = this.host.closest('.main');
      if (sc) {
        const r = sc.getBoundingClientRect();
        if (ev.clientY < r.top + 50) sc.scrollTop -= 18;
        else if (ev.clientY > r.bottom - 50) sc.scrollTop += 18;
      }
    };
    const end = () => {
      grip.removeEventListener('pointermove', place);
      grip.removeEventListener('pointerup', end);
      grip.removeEventListener('pointercancel', end);
      ghost.remove();
      wrap.classList.remove('dragging');
      document.body.classList.remove('board-dragging');
      this.onChange(this.current());
    };
    place(e);
    grip.addEventListener('pointermove', place);
    grip.addEventListener('pointerup', end);
    grip.addEventListener('pointercancel', end);
  }

  /** Hides or shows a block (e.g. screenshots on days without any). */
  setHidden(id, hidden) {
    const wrap = this.wraps.get(id);
    if (wrap) wrap.hidden = hidden;
  }
}
