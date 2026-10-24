// Dialog for adding or editing a tag segment.
import { h, modal, tagInput, toggle, button, toast, confirmDialog } from './ui.js';
import { t } from './i18n.js';
import { dur, clock } from './fmt.js';
import * as api from './api.js';
import { state, loadTags } from './state.js';

const pad = (n) => String(n).padStart(2, '0');

function timeValue(ts) {
  const d = new Date(ts * 1000);
  return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
}

function applyTime(ts, value) {
  const d = new Date(ts * 1000);
  const [hh, mm, ss] = value.split(':').map(Number);
  d.setHours(hh || 0, mm || 0, ss || 0, 0);
  return Math.floor(d.getTime() / 1000);
}

/**
 * Opens the tag editor. seg: {id?, s, e, tags?, notes?, billable?}.
 * Resolves true when something was saved or deleted.
 */
export function openTagDialog(seg, { title, subtitle } = {}) {
  return new Promise((resolve) => {
    let changed = false;
    const ti = tagInput({ value: seg.tags || '', names: state.tagNames, combos: state.tagCombos, autofocus: true, onEnter: () => save() });
    const notes = h('textarea', { class: 'input', rows: 2, placeholder: t('Notes (optional)') }, seg.notes || '');
    let billable = !!seg.billable;
    const start = h('input', { class: 'input time', type: 'time', step: 1, value: timeValue(seg.s) });
    const end = h('input', { class: 'input time', type: 'time', step: 1, value: timeValue(seg.e) });
    const len = h('span', { class: 'muted' }, dur(seg.e - seg.s));
    const updLen = () => {
      const s = applyTime(seg.s, start.value), e = applyTime(seg.e, end.value);
      len.textContent = e > s ? dur(e - s) : '—';
    };
    start.addEventListener('input', updLen);
    end.addEventListener('input', updLen);

    const recent = state.recentTags.slice(0, 10);
    const chips = recent.length ? h('div', { class: 'chips' }, h('span', { class: 'chips-label' }, t('Recent')),
      recent.map((r) => h('button', { type: 'button', class: 'chip', onClick: () => { ti.value = r; ti.input.focus(); } }, r))) : null;

    const body = h('div', { class: 'form' },
      subtitle ? h('p', { class: 'muted' }, subtitle) : null,
      h('div', { class: 'field-row' },
        h('label', { class: 'field' }, h('span', null, t('Start')), start),
        h('label', { class: 'field' }, h('span', null, t('End')), end),
        h('div', { class: 'field grow' }, h('span', null, t('Duration')), len)),
      h('label', { class: 'field' }, h('span', null, t('Tags')), ti),
      chips,
      h('label', { class: 'field' }, h('span', null, t('Notes')), notes),
      toggle(billable, (v) => (billable = v), t('Billable')),
      h('p', { class: 'hint' }, t('Separate tags with commas to build a hierarchy, e.g. "Client, Project, Task".')));

    const save = async () => {
      const s = applyTime(seg.s, start.value), e = applyTime(seg.e, end.value);
      const tags = ti.value.trim();
      if (!tags) { ti.input.focus(); toast(t('Enter at least one tag'), { type: 'error' }); return; }
      if (e <= s) { toast(t('End must be after start'), { type: 'error' }); return; }
      try {
        if (seg.id) await api.post('tags/update', { id: seg.id, s, e, tags, notes: notes.value, billable });
        else await api.post('tags/add', { s, e, tags, notes: notes.value, billable });
        changed = true;
        loadTags();
        m.close();
      } catch (err) { toast(err.message, { type: 'error' }); }
    };
    const actions = [];
    if (seg.id) {
      actions.push({
        label: t('Delete'), danger: true, icon: 'trash', left: true, onClick: async (close) => {
          if (!(await confirmDialog(t('Delete this tag?'), { danger: true, ok: t('Delete') }))) return;
          await api.post('tags/delete', { ids: [seg.id] });
          changed = true;
          close();
        },
      });
    }
    actions.push({ label: t('Cancel') }, { label: seg.id ? t('Save') : t('Add tag'), primary: true, onClick: save });
    const m = modal({
      title: title || (seg.id ? t('Edit tag') : t('Tag time')) + ` · ${clock(seg.s)}–${clock(seg.e)}`,
      body, actions, width: 520, onClose: () => resolve(changed),
    });
    m.el.addEventListener('keydown', (e) => { if (e.key === 'Enter' && e.ctrlKey) save(); });
  });
}

export { button };
