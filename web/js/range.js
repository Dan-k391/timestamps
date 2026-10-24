// Period picker shared by statistics, timesheets and search.
import { h, button, segmented, icon, pickDate } from './ui.js';
import { t } from './i18n.js';
import { today, addDays, startOfWeek, startOfMonth, endOfMonth, fmtRange, parseKey, dateKey } from './fmt.js';

export function resolve(p, d, from, to) {
  d = d || today();
  switch (p) {
    case 'day': return [d, d];
    case 'week': { const s = startOfWeek(d); return [s, addDays(s, 6)]; }
    case 'month': return [startOfMonth(d), endOfMonth(d)];
    case 'year': return [d.slice(0, 4) + '-01-01', d.slice(0, 4) + '-12-31'];
    case 'last7': return [addDays(today(), -6), today()];
    case 'last30': return [addDays(today(), -29), today()];
    case 'all': return ['2000-01-01', today()];
    case 'custom': return [from || addDays(today(), -6), to || today()];
  }
  return [d, d];
}

export function shift(p, d, dir) {
  const date = parseKey(d || today());
  if (p === 'day') date.setDate(date.getDate() + dir);
  else if (p === 'week') date.setDate(date.getDate() + 7 * dir);
  else if (p === 'month') { date.setDate(1); date.setMonth(date.getMonth() + dir); }
  else if (p === 'year') date.setFullYear(date.getFullYear() + dir);
  return dateKey(date);
}

/**
 * Renders a period toolbar. state: {p, d, from, to}; onChange(state).
 */
export function rangeBar(state, onChange, periods = ['day', 'week', 'month', 'year', 'custom']) {
  const labels = { day: t('Day'), week: t('Week'), month: t('Month'), year: t('Year'), custom: t('Custom'), last7: t('7 days'), last30: t('30 days'), all: t('All time') };
  const [from, to] = resolve(state.p, state.d, state.from, state.to);
  const steppable = ['day', 'week', 'month', 'year'].includes(state.p);
  const wrap = h('div', { class: 'range-bar' },
    segmented(periods.map((p) => [p, labels[p]]), state.p, (p) => onChange({ ...state, p, d: state.d || today() })));
  if (steppable) {
    wrap.append(h('div', { class: 'date-nav' },
      button('', { icon: 'left', onClick: () => onChange({ ...state, d: shift(state.p, state.d, -1) }) }),
      h('span', { class: 'range-label' }, fmtRange(from, to)),
      button('', { icon: 'right', onClick: () => onChange({ ...state, d: shift(state.p, state.d, 1) }), disabled: to >= today() }),
      button(t('Current'), { variant: 'ghost', small: true, onClick: () => onChange({ ...state, d: today() }) })));
  } else if (state.p === 'custom') {
    const a = h('button', { class: 'btn date-btn', type: 'button' }, icon('day', 14), h('span', null, from));
    const b = h('button', { class: 'btn date-btn', type: 'button' }, icon('day', 14), h('span', null, to));
    a.onclick = async () => { const v = await pickDate(a, from); if (v) onChange({ ...state, from: v, to: v > to ? v : to }); };
    b.onclick = async () => { const v = await pickDate(b, to); if (v) onChange({ ...state, to: v, from: v < from ? v : from }); };
    wrap.append(h('div', { class: 'date-nav' }, a, h('span', { class: 'muted' }, '–'), b));
  } else {
    wrap.append(h('span', { class: 'range-label' }, state.p === 'all' ? t('All time') : fmtRange(from, to)));
  }
  return wrap;
}
