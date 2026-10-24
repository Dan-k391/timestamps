// Lightweight HTML/CSS charts (crisp at any DPI, no dependencies).
import { h, showTip, hideTip } from './ui.js';
import { escapeHTML } from './fmt.js';

/**
 * Stacked column chart.
 * opts: { labels[], series:[{name, color, values[]}], fmt(v), sub(i)?, onClick(i)?, height }
 */
export function columns(opts) {
  const { labels, series, fmt, height = 180 } = opts;
  const totals = labels.map((_, i) => series.reduce((n, s) => n + (s.values[i] || 0), 0));
  const max = Math.max(1, ...totals);
  const nice = niceMax(max);
  const grid = h('div', { class: 'ch-grid' }, [1, 0.75, 0.5, 0.25, 0].map((f) => h('div', { class: 'ch-gl' }, h('span', null, fmt(nice * f)))));
  const cols = h('div', { class: 'ch-cols' });
  labels.forEach((lab, i) => {
    const col = h('div', {
      class: 'ch-col' + (opts.onClick ? ' click' : ''),
      onClick: opts.onClick ? () => opts.onClick(i) : null,
      onMousemove: (e) => {
        const rows = series.map((s) => `<div class="tip-row"><span class="dot" style="background:${s.color}"></span>${escapeHTML(s.name)}<b>${fmt(s.values[i] || 0)}</b></div>`).join('');
        showTip(e.clientX, e.clientY, `<div class="tip-title">${escapeHTML(opts.tipLabel ? opts.tipLabel(i) : lab)}</div>${rows}`);
      },
      onMouseleave: hideTip,
    });
    const stack = h('div', { class: 'ch-stack', style: { height: (totals[i] / nice) * 100 + '%' } });
    series.forEach((s) => {
      const v = s.values[i] || 0;
      if (v > 0) stack.append(h('div', { class: 'ch-seg', style: { flexGrow: v, background: s.color } }));
    });
    col.append(stack);
    cols.append(col);
  });
  const step = Math.ceil(labels.length / 16);
  const axis = h('div', { class: 'ch-axis' }, labels.map((l, i) => h('span', { style: { visibility: i % step ? 'hidden' : 'visible' } }, l)));
  return h('div', { class: 'chart', style: { height: height + 'px' } }, h('div', { class: 'ch-body' }, grid, cols), axis);
}

function niceMax(v) {
  const hours = v / 3600;
  const steps = [0.5, 1, 2, 4, 6, 8, 10, 12, 16, 20, 24, 40, 60, 80, 100, 160, 200, 300, 400, 600, 800, 1000, 2000, 5000];
  for (const s of steps) if (hours <= s) return s * 3600;
  return Math.ceil(hours / 1000) * 1000 * 3600;
}

/** Weekday × hour heatmap. heat[7][24] seconds, Monday first. */
export function heatmap(heat, { days, fmt, weekStart = 1, hourLabel }) {
  const max = Math.max(1, ...heat.flat());
  const order = weekStart === 0 ? [6, 0, 1, 2, 3, 4, 5] : [0, 1, 2, 3, 4, 5, 6];
  const grid = h('div', { class: 'heat' });
  grid.append(h('span', null));
  for (let hr = 0; hr < 24; hr++) grid.append(h('span', { class: 'heat-h' }, hr % 3 === 0 ? String(hr).padStart(2, '0') : ''));
  for (const d of order) {
    grid.append(h('span', { class: 'heat-d' }, days[d]));
    for (let hr = 0; hr < 24; hr++) {
      const v = heat[d][hr];
      const a = v ? 0.12 + 0.88 * Math.sqrt(v / max) : 0;
      grid.append(h('span', {
        class: 'heat-c', style: { '--a': a },
        onMousemove: (e) => showTip(e.clientX, e.clientY, `<div class="tip-title">${escapeHTML(days[d])} ${hourLabel(hr)}</div><div class="tip-time">${fmt(v)}</div>`),
        onMouseleave: hideTip,
      }));
    }
  }
  return grid;
}

/** Horizontal share bar made of coloured parts. */
export function shareBar(parts) {
  const total = parts.reduce((n, p) => n + p.value, 0) || 1;
  return h('div', { class: 'share' }, parts.filter((p) => p.value > 0).map((p) => h('div', { style: { flexGrow: p.value / total, background: p.color }, title: p.label })));
}
