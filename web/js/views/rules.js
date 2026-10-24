// Autotag rules: tag time automatically from apps, titles, URLs and files.
import { h, clear, button, toggle, select, empty, tagInput, toast, icon, confirmDialog, pickDate } from '../ui.js';
import { t } from '../i18n.js';
import { dur, clock, today, fmtDateLong, hashColor } from '../fmt.js';
import * as api from '../api.js';
import { state, loadTags } from '../state.js';

const uid = () => Math.random().toString(36).slice(2, 10);

export function mount(el) {
  el.className = 'view rules-view';
  let rules = [];
  let previewDate = today();
  const list = h('div', { class: 'rules' });
  const preview = h('section', { class: 'card preview-card' });
  const status = h('span', { class: 'muted save-status' });
  el.append(
    h('header', { class: 'toolbar' }, h('h2', null, t('Autotag rules')), status, h('div', { class: 'spacer' }),
      button(t('New rule'), { icon: 'plus', variant: 'primary', onClick: () => { rules.unshift({ id: uid(), tags: '', enabled: true, match: 'all', billable: false, conds: [{ field: 'app', op: 'contains', value: '' }] }); draw(); } })),
    h('p', { class: 'muted intro' }, t('Rules are checked from top to bottom; the first match decides the tag. Suggested tags appear in the Autotags lane of the day view, where you can apply them.')),
    h('div', { class: 'rules-layout' }, list, preview));

  let saveTimer;
  function save() {
    status.textContent = t('Saving…');
    clearTimeout(saveTimer);
    saveTimer = setTimeout(async () => {
      try {
        await api.post('rules', rules);
        status.textContent = t('All changes saved');
        runPreview();
      } catch (err) { status.textContent = err.message; }
    }, 500);
  }

  const fields = [['app', t('Application')], ['title', t('Window title')], ['url', t('URL')], ['domain', t('Website domain')], ['file', t('File path')], ['any', t('Anything')]];
  const ops = [['contains', t('contains')], ['notcontains', t('does not contain')], ['equals', t('equals')], ['starts', t('starts with')], ['ends', t('ends with')], ['regex', t('matches regex')]];

  function draw() {
    clear(list);
    if (!rules.length) {
      list.append(h('section', { class: 'card' }, empty('sparkles', t('No rules yet'), t('Example: tag everything in "Visual Studio Code" or on "github.com" as "Development".'))));
      return;
    }
    rules.forEach((r, idx) => {
      const ti = tagInput({ value: r.tags, names: state.tagNames, combos: state.tagCombos, placeholder: t('Tag to apply') });
      ti.input.addEventListener('input', () => { r.tags = ti.value; save(); });
      const conds = h('div', { class: 'conds' });
      const drawConds = () => {
        clear(conds);
        r.conds.forEach((c, ci) => {
          const v = h('input', { class: 'input', value: c.value, placeholder: t('Text') });
          v.addEventListener('input', () => { c.value = v.value; save(); });
          conds.append(h('div', { class: 'cond' },
            ci === 0 ? h('span', { class: 'cond-if' }, t('If')) : h('span', { class: 'cond-if' }, r.match === 'any' ? t('or') : t('and')),
            select(fields, c.field, (x) => { c.field = x; save(); }),
            select(ops, c.op, (x) => { c.op = x; save(); }),
            v,
            button('', { icon: 'x', small: true, variant: 'ghost', title: t('Remove condition'), onClick: () => { r.conds.splice(ci, 1); drawConds(); save(); } })));
        });
        conds.append(button(t('Add condition'), { icon: 'plus', small: true, variant: 'ghost', onClick: () => { r.conds.push({ field: 'title', op: 'contains', value: '' }); drawConds(); } }));
      };
      drawConds();
      list.append(h('section', { class: 'card rule' + (r.enabled ? '' : ' disabled') },
        h('div', { class: 'rule-head' },
          h('span', { class: 'rule-no' }, '#' + (idx + 1)),
          h('span', { class: 'dot', style: { background: hashColor(r.tags || '?') } }),
          ti,
          select([['all', t('All conditions')], ['any', t('Any condition')]], r.match, (x) => { r.match = x; save(); draw(); }),
          toggle(r.billable, (x) => { r.billable = x; save(); }, t('Billable')),
          toggle(r.enabled, (x) => { r.enabled = x; save(); draw(); }, t('On')),
          h('div', { class: 'btn-group' },
            button('', { icon: 'arrowUp', small: true, variant: 'ghost', disabled: idx === 0, onClick: () => { rules.splice(idx - 1, 0, rules.splice(idx, 1)[0]); draw(); save(); } }),
            button('', { icon: 'arrowDown', small: true, variant: 'ghost', disabled: idx === rules.length - 1, onClick: () => { rules.splice(idx + 1, 0, rules.splice(idx, 1)[0]); draw(); save(); } }),
            button('', { icon: 'trash', small: true, variant: 'ghost', onClick: async () => { if (await confirmDialog(t('Delete this rule?'), { danger: true, ok: t('Delete') })) { rules.splice(idx, 1); draw(); save(); } } }))),
        conds));
    });
  }

  async function runPreview() {
    clear(preview);
    const dateBtn = h('button', { class: 'btn date-btn', type: 'button' }, icon('day', 14), h('span', null, fmtDateLong(previewDate)));
    dateBtn.onclick = async () => { const d = await pickDate(dateBtn, previewDate); if (d) { previewDate = d; runPreview(); } };
    const overwrite = h('input', { type: 'checkbox' });
    preview.append(h('div', { class: 'card-head' }, icon('sparkles', 16), h('h3', null, t('Preview')), h('div', { class: 'spacer' }), dateBtn));
    const body = h('div', { class: 'list' }, h('div', { class: 'muted pad' }, t('Loading…')));
    preview.append(body);
    const res = await api.post('autotag/preview', { from: previewDate, to: previewDate, rules });
    clear(body);
    if (!res.length) body.append(empty('sparkles', t('No matches on this day')));
    let total = 0;
    for (const a of res) {
      total += a.e - a.s;
      body.append(h('div', { class: 'sub-row' }, h('span', { class: 'dot', style: { background: hashColor(a.tags) } }), h('span', { class: 'row-name' }, a.tags), h('span', { class: 'muted' }, `${clock(a.s)}–${clock(a.e)}`), h('span', { class: 'row-dur' }, dur(a.e - a.s))));
    }
    if (res.length) {
      preview.append(h('div', { class: 'preview-foot' },
        h('span', { class: 'muted' }, t('{0} segments · {1}', res.length, dur(total))),
        h('label', { class: 'inline' }, overwrite, t('Overwrite existing tags')),
        button(t('Apply to this day'), { icon: 'check', variant: 'primary', small: true, onClick: async () => {
          const r = await api.post('autotag/apply', { from: previewDate, to: previewDate, overwrite: overwrite.checked });
          toast(t('{0} tags added', r.count), { type: 'ok' });
          loadTags();
        } })));
    }
  }

  (async () => {
    await loadTags();
    rules = await api.get('rules');
    draw();
    runPreview();
  })();
  return {};
}
