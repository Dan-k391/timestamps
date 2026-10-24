// Screenshot gallery and lightbox.
import { h, clear, icon, button, empty, pickDate, toast } from '../ui.js';
import { t } from '../i18n.js';
import { clock, fmtDateLong, today, addDays, bytes, hourLabel } from '../fmt.js';
import * as api from '../api.js';
import { state } from '../state.js';
import { go } from '../router.js';

export function openLightbox(list, index) {
  let i = index;
  const img = h('img', { class: 'lb-img', alt: '' });
  const cap = h('div', { class: 'lb-cap' });
  const show = () => {
    img.src = api.shotURL(list[i], false);
    cap.textContent = `${clock(list[i], true)} · ${i + 1} / ${list.length}`;
  };
  const close = () => { wrap.remove(); document.removeEventListener('keydown', key, true); };
  const key = (e) => {
    if (e.key === 'Escape') { close(); e.stopPropagation(); }
    else if (e.key === 'ArrowLeft' && i > 0) { i--; show(); e.stopPropagation(); }
    else if (e.key === 'ArrowRight' && i < list.length - 1) { i++; show(); e.stopPropagation(); }
  };
  const wrap = h('div', { class: 'lightbox', onClick: (e) => { if (e.target === wrap || e.target === img) close(); } },
    button('', { icon: 'left', class: 'lb-prev', onClick: () => { if (i > 0) { i--; show(); } } }),
    img,
    button('', { icon: 'right', class: 'lb-next', onClick: () => { if (i < list.length - 1) { i++; show(); } } }),
    button('', { icon: 'x', class: 'lb-close', onClick: close }),
    cap);
  document.getElementById('overlays').append(wrap);
  document.addEventListener('keydown', key, true);
  show();
}

export function mount(el, params) {
  const date = params.date || today();
  el.className = 'view shots-view';
  const dateBtn = h('button', { class: 'btn date-btn', type: 'button' }, icon('day', 16), h('span', null, fmtDateLong(date)));
  dateBtn.onclick = async () => { const d = await pickDate(dateBtn, date); if (d) go('screenshots/' + d); };
  const grid = h('div', { class: 'shots-grid' });
  const info = h('span', { class: 'muted' });
  el.append(
    h('header', { class: 'toolbar' },
      h('div', { class: 'date-nav' },
        button('', { icon: 'left', title: t('Previous day'), onClick: () => go('screenshots/' + addDays(date, -1)) }),
        dateBtn,
        button('', { icon: 'right', title: t('Next day'), onClick: () => go('screenshots/' + addDays(date, 1)) }),
        button(t('Today'), { variant: 'ghost', onClick: () => go('screenshots/' + today()) })),
      info,
      h('div', { class: 'spacer' }),
      button(t('Take screenshot now'), { icon: 'camera', onClick: async () => { await api.post('shot/now'); toast(t('Screenshot taken')); setTimeout(load, 1500); } }),
      button(t('Settings'), { icon: 'settings', variant: 'ghost', onClick: () => go('settings/screenshots') })),
    grid);

  async function load() {
    const list = await api.get('shots', { from: date, to: date });
    clear(grid);
    const s = state.settings || {};
    info.textContent = list.length ? (list.length === 1 ? t('1 screenshot') : t('{0} screenshots', list.length)) : '';
    if (!list.length) {
      grid.append(empty('camera', t('No screenshots for this day'),
        s.shotsOn ? t('Screenshots are taken every {0} min while you are active.', s.shotsInterval) : t('Automatic screenshots are turned off.')));
      return;
    }
    let hour = -1, section;
    list.forEach((ts, i) => {
      const hr = new Date(ts * 1000).getHours();
      if (hr !== hour) {
        hour = hr;
        section = h('div', { class: 'shots-row' });
        grid.append(h('div', { class: 'shots-hour' }, hourLabel(hr)), section);
      }
      section.append(h('button', { class: 'shot', type: 'button', onClick: () => openLightbox(list, i) },
        h('img', { src: api.shotURL(ts, true), loading: 'lazy', alt: '' }),
        h('span', null, clock(ts))));
    });
  }
  load().then(() => api.get('info')).then((i) => { if (i.shotsSize) info.textContent += ` · ${t('Storage')} ${bytes(i.shotsSize)}`; });
  return { refresh: (ev) => { if (ev && ev.type === 'shot' && date === today()) load(); } };
}
