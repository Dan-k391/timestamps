// Canvas timeline: stacked lanes of segments with zoom, pan, hover,
// range selection (with snapping) and a 24-hour overview strip.
import { clock } from './fmt.js';

const TICKS = [60, 120, 300, 600, 900, 1800, 3600, 7200, 10800, 21600];
const FONT = '"Segoe UI Variable Text", "Segoe UI", "Microsoft YaHei UI", system-ui, sans-serif';

function lastStartingBefore(segs, t) {
  let lo = 0, hi = segs.length - 1, ans = -1;
  while (lo <= hi) {
    const mid = (lo + hi) >> 1;
    if (segs[mid].s <= t) { ans = mid; lo = mid + 1; } else hi = mid - 1;
  }
  return ans;
}

function firstEndingAfter(segs, t) {
  // segments in a lane are sorted and (nearly) non-overlapping
  let i = lastStartingBefore(segs, t);
  if (i < 0) return 0;
  while (i > 0 && segs[i - 1].e > t) i--;
  return segs[i].e > t ? i : i + 1;
}

export class Timeline {
  constructor(host, opts = {}) {
    this.opts = opts;
    this.host = host;
    this.canvas = document.createElement('canvas');
    this.canvas.className = 'tl-canvas';
    this.mini = document.createElement('canvas');
    this.mini.className = 'tl-mini';
    host.append(this.canvas, this.mini);
    this.labelW = 112;
    this.padR = 14;
    this.axisH = 30;
    this.gap = 6;
    this.lanes = [];
    this.from = 0;
    this.to = 86400;
    this.view = [0, 86400];
    this.sel = null;
    this.hover = null;
    this.highlight = null;
    this.filter = '';
    this.now = 0;
    this.bounds = [];
    this.textW = new Map();
    this.drag = null;
    this.raf = 0;

    this.ro = new ResizeObserver(() => this.resize());
    this.ro.observe(host);
    this.canvas.addEventListener('wheel', (e) => this.onWheel(e), { passive: false });
    this.canvas.addEventListener('mousedown', (e) => this.onDown(e));
    this.canvas.addEventListener('mousemove', (e) => { if (!this.drag) this.onHoverMove(e); });
    this.canvas.addEventListener('mouseleave', () => { if (!this.drag) this.setHover(null); });
    this.canvas.addEventListener('dblclick', (e) => { const hit = this.hitAt(e); opts.onDblClick && opts.onDblClick(hit, e); });
    this.canvas.addEventListener('contextmenu', (e) => {
      e.preventDefault();
      const hit = this.hitAt(e);
      opts.onContext && opts.onContext(hit, e);
    });
    this.mini.addEventListener('mousedown', (e) => this.onMiniDown(e));
    this.onMove = (e) => this.onDragMove(e);
    this.onUp = (e) => this.onDragUp(e);
    this.resize();
  }

  destroy() {
    this.ro.disconnect();
    window.removeEventListener('mousemove', this.onMove);
    window.removeEventListener('mouseup', this.onUp);
    cancelAnimationFrame(this.raf);
  }

  // ---- data & view ------------------------------------------------------

  setData({ from, to, lanes, now }, keepView = true) {
    const dayChanged = from !== this.from || to !== this.to;
    this.from = from;
    this.to = to;
    this.lanes = lanes;
    this.now = now || 0;
    const b = [];
    for (const l of lanes) for (const s of l.segs) { b.push(s.s, s.e); }
    if (this.now) b.push(this.now);
    b.sort((x, y) => x - y);
    this.bounds = b;
    if (dayChanged || !keepView) this.view = this.defaultView();
    this.resize();
  }

  /** Active hours with padding, or the whole day when there is no data. */
  defaultView() {
    const usage = this.lanes.find((l) => l.id === 'usage');
    const segs = (usage && usage.segs.length ? usage.segs : this.lanes.flatMap((l) => l.segs)).filter((s) => s.e > this.from && s.s < this.to);
    if (!segs.length) return [this.from, this.to];
    let s = Math.min(...segs.map((x) => x.s)), e = Math.max(...segs.map((x) => x.e));
    const pad = Math.max(900, (e - s) * 0.04);
    s = Math.max(this.from, Math.floor((s - pad) / 1800) * 1800);
    e = Math.min(this.to, Math.ceil((e + pad) / 1800) * 1800);
    if (e - s < 3600) { s = Math.max(this.from, s - 1800); e = Math.min(this.to, e + 1800); }
    return [s, e];
  }

  setView(s, e, animate = false) {
    const min = 60, span = Math.max(min, Math.min(this.to - this.from, e - s));
    if (s < this.from) s = this.from;
    if (s + span > this.to) s = this.to - span;
    const target = [s, s + span];
    if (!animate) {
      this.view = target;
      this.schedule();
      this.opts.onView && this.opts.onView(this.view);
      return;
    }
    const start = [...this.view], t0 = performance.now(), dur = 180;
    const step = (now) => {
      const k = Math.min(1, (now - t0) / dur), ease = 1 - Math.pow(1 - k, 3);
      this.view = [start[0] + (target[0] - start[0]) * ease, start[1] + (target[1] - start[1]) * ease];
      this.draw();
      if (k < 1) this.raf = requestAnimationFrame(step);
      else this.opts.onView && this.opts.onView(this.view);
    };
    cancelAnimationFrame(this.raf);
    this.raf = requestAnimationFrame(step);
  }

  zoomBy(f, center) {
    const [s, e] = this.view;
    const c = center ?? (this.sel ? (this.sel.s + this.sel.e) / 2 : (s + e) / 2);
    const ns = c - (c - s) * f, ne = c + (e - c) * f;
    this.setView(ns, ne, true);
  }

  fit(full = false) {
    const [s, e] = full ? [this.from, this.to] : this.defaultView();
    this.setView(s, e, true);
  }

  zoomTo(s, e) {
    const pad = Math.max(60, (e - s) * 0.08);
    this.setView(s - pad, e + pad, true);
  }

  setSelection(sel, notify = true) {
    this.sel = sel && sel.e > sel.s ? { s: Math.max(this.from, sel.s), e: Math.min(this.to, sel.e) } : null;
    this.schedule();
    if (notify && this.opts.onSelect) this.opts.onSelect(this.sel);
  }

  /** fn(seg) returns true (emphasise), false (dim) or null (leave as is). */
  setHighlight(fn) { this.highlight = fn; this.schedule(); }
  setFilter(text) { this.filter = (text || '').toLowerCase(); this.schedule(); }
  setNow(now) { this.now = now; this.schedule(); }

  // ---- geometry ---------------------------------------------------------

  resize() {
    const w = this.host.clientWidth;
    let hgt = this.axisH + 4;
    for (const l of this.lanes) hgt += l.h + this.gap;
    hgt += 2;
    const dpr = window.devicePixelRatio || 1;
    this.W = w;
    this.H = hgt;
    this.dpr = dpr;
    for (const [c, ch] of [[this.canvas, hgt], [this.mini, 26]]) {
      c.style.width = w + 'px';
      c.style.height = ch + 'px';
      c.width = Math.round(w * dpr);
      c.height = Math.round(ch * dpr);
    }
    this.draw();
  }

  get plotW() { return Math.max(10, this.W - this.labelW - this.padR); }
  x(t) { return this.labelW + ((t - this.view[0]) / (this.view[1] - this.view[0])) * this.plotW; }
  tAt(x) { return this.view[0] + ((x - this.labelW) / this.plotW) * (this.view[1] - this.view[0]); }

  laneAt(y) {
    let top = this.axisH + 4;
    for (const l of this.lanes) {
      if (y >= top && y < top + l.h) return { lane: l, top };
      top += l.h + this.gap;
    }
    return null;
  }

  laneTop(id) {
    let top = this.axisH + 4;
    for (const l of this.lanes) { if (l.id === id) return top; top += l.h + this.gap; }
    return -1;
  }

  pos(e) {
    const r = this.canvas.getBoundingClientRect();
    return { x: e.clientX - r.left, y: e.clientY - r.top };
  }

  hitAt(e) {
    const { x, y } = this.pos(e);
    const t = this.tAt(x);
    if (x < this.labelW || x > this.labelW + this.plotW) return { t: null };
    if (y < this.axisH) return { t, axis: true };
    const la = this.laneAt(y);
    if (!la) return { t };
    const segs = la.lane.segs;
    let i = lastStartingBefore(segs, t);
    let seg = i >= 0 && segs[i].e > t ? segs[i] : null;
    if (!seg) {
      // Pick tiny neighbours within 4px.
      const tol = (4 / this.plotW) * (this.view[1] - this.view[0]);
      for (const j of [i, i + 1]) {
        const s = segs[j];
        if (s && s.s - tol <= t && s.e + tol >= t) { seg = s; break; }
      }
    }
    return { t, lane: la.lane, seg };
  }

  snap(t) {
    const b = this.bounds;
    if (!b.length) return Math.round(t);
    let lo = 0, hi = b.length - 1;
    while (lo < hi) { const m = (lo + hi) >> 1; if (b[m] < t) lo = m + 1; else hi = m; }
    let best = Math.round(t), bestD = 7;
    for (const j of [lo - 1, lo]) {
      if (j < 0 || j >= b.length) continue;
      const d = Math.abs(this.x(b[j]) - this.x(t));
      if (d < bestD) { bestD = d; best = b[j]; }
    }
    return best;
  }

  // ---- interaction ------------------------------------------------------

  onWheel(e) {
    e.preventDefault();
    const { x } = this.pos(e);
    const span = this.view[1] - this.view[0];
    if (e.shiftKey || Math.abs(e.deltaX) > Math.abs(e.deltaY)) {
      const d = (e.shiftKey ? e.deltaY : e.deltaX) * (span / this.plotW);
      this.setView(this.view[0] + d, this.view[1] + d);
      return;
    }
    const f = Math.exp(Math.max(-60, Math.min(60, e.deltaY)) * 0.004);
    const c = this.tAt(Math.max(this.labelW, Math.min(this.labelW + this.plotW, x)));
    this.setView(c - (c - this.view[0]) * f, c + (this.view[1] - c) * f);
  }

  onDown(e) {
    if (e.button === 2) return;
    const { x, y } = this.pos(e);
    const pan = e.button === 1 || y < this.axisH || x < this.labelW;
    let mode = pan ? 'pan' : 'pending';
    let edge = null;
    if (!pan && this.sel) {
      if (Math.abs(this.x(this.sel.s) - x) < 6) { mode = 'edge'; edge = 's'; }
      else if (Math.abs(this.x(this.sel.e) - x) < 6) { mode = 'edge'; edge = 'e'; }
    }
    this.drag = { mode, edge, x0: x, y0: y, view0: [...this.view], t0: this.snap(this.tAt(x)), shift: e.shiftKey, ev: e };
    window.addEventListener('mousemove', this.onMove);
    window.addEventListener('mouseup', this.onUp);
    e.preventDefault();
  }

  onDragMove(e) {
    const d = this.drag;
    if (!d) return;
    const { x } = this.pos(e);
    if (d.mode === 'pan') {
      const dt = ((x - d.x0) / this.plotW) * (d.view0[1] - d.view0[0]);
      this.setView(d.view0[0] - dt, d.view0[1] - dt);
      this.canvas.style.cursor = 'grabbing';
      return;
    }
    if (d.mode === 'pending' && Math.abs(x - d.x0) > 3) d.mode = 'select';
    const t = this.snap(Math.max(this.from, Math.min(this.to, this.tAt(x))));
    if (d.mode === 'select') {
      this.sel = { s: Math.min(d.t0, t), e: Math.max(d.t0, t) };
      this.schedule();
      this.opts.onSelecting && this.opts.onSelecting(this.sel);
    } else if (d.mode === 'edge') {
      const other = d.edge === 's' ? this.sel.e : this.sel.s;
      this.sel = { s: Math.min(other, t), e: Math.max(other, t) };
      this.schedule();
      this.opts.onSelecting && this.opts.onSelecting(this.sel);
    }
    this.setHover(null);
  }

  onDragUp(e) {
    const d = this.drag;
    this.drag = null;
    window.removeEventListener('mousemove', this.onMove);
    window.removeEventListener('mouseup', this.onUp);
    this.canvas.style.cursor = '';
    if (!d) return;
    if (d.mode === 'select' || d.mode === 'edge') {
      this.setSelection(this.sel);
      return;
    }
    if (d.mode === 'pending') {
      const hit = this.hitAt(d.ev);
      if (hit.seg) {
        const s = hit.seg;
        if (d.shift && this.sel) this.setSelection({ s: Math.min(this.sel.s, s.s), e: Math.max(this.sel.e, s.e) });
        else this.setSelection({ s: s.s, e: s.e });
        this.opts.onClickSeg && this.opts.onClickSeg(hit, e);
      } else {
        this.setSelection(null);
        this.opts.onClickSeg && this.opts.onClickSeg(hit, e);
      }
    }
  }

  onHoverMove(e) {
    const { x, y } = this.pos(e);
    if (this.sel && y >= this.axisH && (Math.abs(this.x(this.sel.s) - x) < 6 || Math.abs(this.x(this.sel.e) - x) < 6)) {
      this.canvas.style.cursor = 'ew-resize';
    } else if (y < this.axisH || x < this.labelW) {
      this.canvas.style.cursor = 'grab';
    } else {
      this.canvas.style.cursor = '';
    }
    const hit = this.hitAt(e);
    this.setHover(hit.seg ? hit : null, e);
  }

  setHover(hit, e) {
    const prev = this.hover && this.hover.seg;
    this.hover = hit;
    if ((hit && hit.seg) !== prev) this.schedule();
    this.opts.onHover && this.opts.onHover(hit, e);
  }

  onMiniDown(e) {
    const r = this.mini.getBoundingClientRect();
    const span = this.view[1] - this.view[0];
    const toT = (cx) => this.from + ((cx - r.left - this.labelW) / this.plotW) * (this.to - this.from);
    const t = toT(e.clientX);
    const inside = t >= this.view[0] && t <= this.view[1];
    const offset = inside ? t - this.view[0] : span / 2;
    if (!inside) this.setView(t - offset, t - offset + span);
    const move = (ev) => { const tt = toT(ev.clientX); this.setView(tt - offset, tt - offset + span); };
    const up = () => { window.removeEventListener('mousemove', move); window.removeEventListener('mouseup', up); };
    window.addEventListener('mousemove', move);
    window.addEventListener('mouseup', up);
    e.preventDefault();
  }

  // ---- drawing ----------------------------------------------------------

  schedule() {
    if (this.pending) return;
    this.pending = true;
    requestAnimationFrame(() => { this.pending = false; this.draw(); });
  }

  colors() {
    const cs = getComputedStyle(document.documentElement);
    const v = (n) => cs.getPropertyValue(n).trim();
    return {
      text: v('--text'), text2: v('--text-2'), text3: v('--text-3'), lane: v('--lane'), grid: v('--grid'),
      accent: v('--accent'), sel: v('--sel'), surface: v('--surface'), away: v('--away'), active: v('--active'), now: v('--now'),
    };
  }

  measure(ctx, s) {
    let w = this.textW.get(s);
    if (w === undefined) {
      w = ctx.measureText(s).width;
      if (this.textW.size > 6000) this.textW.clear();
      this.textW.set(s, w);
    }
    return w;
  }

  fitText(ctx, s, max) {
    if (this.measure(ctx, s) <= max) return s;
    let lo = 0, hi = s.length;
    while (lo < hi) {
      const m = (lo + hi + 1) >> 1;
      if (this.measure(ctx, s.slice(0, m) + '…') <= max) lo = m; else hi = m - 1;
    }
    return lo ? s.slice(0, lo) + '…' : '';
  }

  draw() {
    const ctx = this.canvas.getContext('2d');
    const { W, H, dpr } = this;
    if (!W) return;
    const C = this.colors();
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, W, H);
    const [vs, ve] = this.view;
    const span = ve - vs;
    const pw = this.plotW, L = this.labelW;

    // Ticks.
    let step = TICKS[TICKS.length - 1];
    for (const s of TICKS) { if ((s / span) * pw >= 78) { step = s; break; } }
    const minor = step >= 3600 ? step / 4 : step >= 600 ? step / 5 : step / 2;
    const tz = new Date(vs * 1000).getTimezoneOffset() * 60;
    const lanesTop = this.axisH + 4;
    ctx.font = `500 11px ${FONT}`;
    ctx.textBaseline = 'middle';
    ctx.lineWidth = 1;
    for (let t = Math.ceil((vs - tz) / minor) * minor + tz; t <= ve; t += minor) {
      const x = Math.round(this.x(t)) + 0.5;
      const major = Math.abs(((t - tz) % step)) < 1;
      ctx.strokeStyle = C.grid;
      ctx.globalAlpha = major ? 1 : 0.45;
      ctx.beginPath();
      ctx.moveTo(x, major ? lanesTop - 6 : lanesTop - 3);
      ctx.lineTo(x, H);
      ctx.stroke();
      ctx.globalAlpha = 1;
      if (major) {
        ctx.fillStyle = C.text3;
        ctx.textAlign = 'center';
        ctx.fillText(clock(t), x, 13);
      }
    }

    // Lanes.
    let top = lanesTop;
    ctx.textAlign = 'left';
    for (const lane of this.lanes) {
      ctx.fillStyle = C.lane;
      roundRect(ctx, L, top, pw, lane.h, 6);
      ctx.fill();
      ctx.fillStyle = C.text2;
      ctx.font = `600 12px ${FONT}`;
      ctx.fillText(lane.label, 4, top + lane.h / 2);
      if (lane.sub) {
        ctx.fillStyle = C.text3;
        ctx.font = `500 10.5px ${FONT}`;
      }
      ctx.save();
      ctx.beginPath();
      ctx.rect(L, top, pw, lane.h);
      ctx.clip();
      this.drawLane(ctx, lane, top, C);
      ctx.restore();
      top += lane.h + this.gap;
    }

    // Selection.
    if (this.sel) {
      const x0 = Math.max(L, this.x(this.sel.s)), x1 = Math.min(L + pw, this.x(this.sel.e));
      if (x1 > x0 - 1) {
        ctx.fillStyle = C.sel;
        ctx.fillRect(x0, lanesTop - 4, Math.max(1, x1 - x0), H - lanesTop + 4);
        ctx.strokeStyle = C.accent;
        ctx.lineWidth = 1.5;
        ctx.beginPath();
        ctx.moveTo(x0, lanesTop - 4); ctx.lineTo(x0, H);
        ctx.moveTo(x1, lanesTop - 4); ctx.lineTo(x1, H);
        ctx.stroke();
        ctx.lineWidth = 1;
      }
    }

    // Now indicator.
    if (this.now && this.now >= vs && this.now <= ve) {
      const x = Math.round(this.x(this.now)) + 0.5;
      ctx.strokeStyle = C.now;
      ctx.lineWidth = 1.5;
      ctx.beginPath();
      ctx.moveTo(x, lanesTop - 2);
      ctx.lineTo(x, H);
      ctx.stroke();
      ctx.fillStyle = C.now;
      ctx.beginPath();
      ctx.moveTo(x - 5, lanesTop - 9); ctx.lineTo(x + 5, lanesTop - 9); ctx.lineTo(x, lanesTop - 3);
      ctx.fill();
      ctx.lineWidth = 1;
    }
    this.drawMini(C);
  }

  drawLane(ctx, lane, top, C) {
    const [vs, ve] = this.view;
    const segs = lane.segs;
    const hov = this.hover && this.hover.seg;
    const pad = lane.style === 'usage' ? 3 : 3;
    const y = top + pad, hh = lane.h - pad * 2;
    if (lane.style === 'shots') {
      ctx.fillStyle = C.accent;
      for (let i = firstEndingAfter(segs, vs); i < segs.length && segs[i].s < ve; i++) {
        const x = Math.round(this.x(segs[i].s));
        ctx.globalAlpha = segs[i] === hov ? 1 : 0.75;
        roundRect(ctx, x - 2, y + 2, 4, hh - 4, 2);
        ctx.fill();
      }
      ctx.globalAlpha = 1;
      return;
    }
    ctx.font = `500 11.5px ${FONT}`;
    let lastPx = -10, lastColor = '';
    const filter = this.filter, hl = this.highlight;
    for (let i = firstEndingAfter(segs, vs); i < segs.length && segs[i].s < ve; i++) {
      const sg = segs[i];
      let x0 = this.x(sg.s), x1 = this.x(sg.e);
      let w = x1 - x0;
      const dim = (hl && hl(sg, lane) === false) || (filter && !(sg.search || sg.label || '').toLowerCase().includes(filter));
      if (w < 1) {
        const px = Math.floor(x0);
        if (px === lastPx && sg.color === lastColor && !dim) continue;
        lastPx = px; lastColor = sg.color;
        x0 = px; w = 1;
      }
      ctx.globalAlpha = dim ? 0.16 : lane.style === 'auto' ? 0.55 : 1;
      ctx.fillStyle = sg.color;
      if (w > 5) { roundRect(ctx, x0 + 0.5, y, w - 1, hh, Math.min(4, w / 3)); ctx.fill(); }
      else ctx.fillRect(x0, y, w, hh);
      if (lane.style === 'auto' && w > 4) {
        ctx.strokeStyle = sg.color;
        ctx.globalAlpha = dim ? 0.2 : 0.9;
        ctx.setLineDash([3, 2]);
        roundRect(ctx, x0 + 1, y + 0.5, w - 2, hh - 1, 4);
        ctx.stroke();
        ctx.setLineDash([]);
      }
      if (sg === hov) {
        ctx.globalAlpha = 1;
        ctx.strokeStyle = C.text;
        ctx.lineWidth = 1.5;
        roundRect(ctx, x0 + 0.5, y - 0.5, Math.max(2, w - 1), hh + 1, 4);
        ctx.stroke();
        ctx.lineWidth = 1;
      }
      if (w > 34 && sg.label && lane.style !== 'usage') {
        const vx0 = Math.max(x0, this.labelW) + 6, vx1 = Math.min(x1, this.labelW + this.plotW) - 6;
        const txt = this.fitText(ctx, sg.label, vx1 - vx0);
        if (txt) {
          ctx.globalAlpha = dim ? 0.35 : 1;
          ctx.fillStyle = sg.ink || '#fff';
          ctx.fillText(txt, vx0, top + lane.h / 2 + 0.5);
        }
      }
    }
    ctx.globalAlpha = 1;
  }

  drawMini(C) {
    const ctx = this.mini.getContext('2d');
    const { W, dpr } = this;
    const H = 26;
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, W, H);
    const L = this.labelW, pw = this.plotW, span = this.to - this.from;
    const mx = (t) => L + ((t - this.from) / span) * pw;
    ctx.fillStyle = C.lane;
    roundRect(ctx, L, 4, pw, 14, 4);
    ctx.fill();
    const usage = this.lanes.find((l) => l.id === 'usage');
    if (usage) {
      for (const s of usage.segs) {
        ctx.fillStyle = s.color;
        ctx.globalAlpha = 0.9;
        ctx.fillRect(mx(s.s), 7, Math.max(1, mx(s.e) - mx(s.s)), 8);
      }
      ctx.globalAlpha = 1;
    }
    ctx.font = `500 10px ${FONT}`;
    ctx.fillStyle = C.text3;
    ctx.textBaseline = 'middle';
    ctx.textAlign = 'center';
    for (let hr = 0; hr <= 24; hr += 3) {
      const x = mx(this.from + hr * 3600);
      if (hr > 0 && hr < 24) ctx.fillText(String(hr).padStart(2, '0'), x, 22.5);
    }
    const v0 = mx(this.view[0]), v1 = mx(this.view[1]);
    ctx.strokeStyle = C.accent;
    ctx.fillStyle = C.sel;
    ctx.lineWidth = 1.5;
    roundRect(ctx, v0, 2, Math.max(4, v1 - v0), 18, 4);
    ctx.fill();
    ctx.stroke();
    ctx.lineWidth = 1;
    if (this.now >= this.from && this.now <= this.to) {
      ctx.fillStyle = C.now;
      ctx.fillRect(mx(this.now) - 0.75, 2, 1.5, 18);
    }
    ctx.textAlign = 'left';
    ctx.fillStyle = C.text3;
    ctx.fillText('', 4, 11);
  }
}

function roundRect(ctx, x, y, w, h, r) {
  ctx.beginPath();
  if (ctx.roundRect) { ctx.roundRect(x, y, w, h, r); return; }
  ctx.rect(x, y, w, h);
}
