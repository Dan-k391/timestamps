// The keyboard heatmap. Keys go by scan code: where a key is on the keyboard,
// whatever letter its layout gives it (0x100 marks the E0 ones).
import { h } from './ui.js';
import { byOS } from './platform.js';

const letters = (s, code) => [...s].map((c, i) => [c, code + i]);
const fn = (a, b) => Array.from({ length: b - a + 1 }, (_, i) => ['F' + (a + i), 0x3B + a + i - 1]);

// A row: [label, code, width, tall] keys, numbers for gaps, in key widths.
const ROWS = [
  [['Esc', 0x01], 1, ...fn(1, 4), 0.5, ...fn(5, 8), 0.5, ...fn(9, 10), ['F11', 0x57], ['F12', 0x58], 0.25, ['PrtSc', 0x137], ['ScrLk', 0x46], ['Pause', 0x145]],
  [['`', 0x29], ...letters('1234567890', 0x02), ['-', 0x0C], ['=', 0x0D], ['⌫', 0x0E, 2], 0.25, ['Ins', 0x152], ['Home', 0x147], ['PgUp', 0x149], 0.25,
    ['Num', 0x45], ['/', 0x135], ['*', 0x37], ['-', 0x4A]],
  [['Tab', 0x0F, 1.5], ...letters('QWERTYUIOP', 0x10), ['[', 0x1A], [']', 0x1B], ['\\', 0x2B, 1.5], 0.25, ['Del', 0x153], ['End', 0x14F], ['PgDn', 0x151], 0.25,
    ['7', 0x47], ['8', 0x48], ['9', 0x49], ['+', 0x4E, 1, true]],
  [['Caps', 0x3A, 1.75], ...letters('ASDFGHJKL', 0x1E), [';', 0x27], ["'", 0x28], ['Enter', 0x1C, 2.25], 3.5, ['4', 0x4B], ['5', 0x4C], ['6', 0x4D]],
  [['Shift', 0x2A, 2.25], ...letters('ZXCVBNM', 0x2C), [',', 0x33], ['.', 0x34], ['/', 0x35], ['Shift', 0x36, 2.75], 1.25, ['↑', 0x148], 1.25,
    ['1', 0x4F], ['2', 0x50], ['3', 0x51], ['↵', 0x11C, 1, true]],
  [...byOS([['Ctrl', 0x1D, 1.25], ['⌥', 0x38, 1.25], ['⌘', 0x15B, 1.25], ['', 0x39, 6.25], ['⌘', 0x15C, 1.25], ['⌥', 0x138, 1.25], ['Ctrl', 0x11D, 2.5]],
    [['Ctrl', 0x1D, 1.25], ['Win', 0x15B, 1.25], ['Alt', 0x38, 1.25], ['', 0x39, 6.25], ['Alt', 0x138, 1.25], ['Win', 0x15C, 1.25], ['Menu', 0x15D, 1.25], ['Ctrl', 0x11D, 1.25]]), 0.25,
    ['←', 0x14B], ['↓', 0x150], ['→', 0x14D], 0.25, ['0', 0x52, 2], ['.', 0x53]],
];

// A key's full name: some keycaps come twice.
const RIGHT = [0x36, 0x11D, 0x138, 0x15C];
const NUMPAD = [0x135, 0x37, 0x4A, 0x47, 0x48, 0x49, 0x4E, 0x4B, 0x4C, 0x4D, 0x4F, 0x50, 0x51, 0x52, 0x53];
const NAMES = new Map(ROWS.flat().filter(Array.isArray).map(([label, code]) => [code,
  code === 0x39 ? 'Space' : code === 0x0E ? 'Backspace' : code === 0x11C ? 'Num Enter'
    : RIGHT.includes(code) ? 'Right ' + label : NUMPAD.includes(code) ? 'Num ' + label : label]));

/** Keys coloured by how often they were pressed; counts by scan code. */
export function keyboardHeat(counts) {
  const max = Math.max(1, ...Object.values(counts));
  const kb = h('div', { class: 'kb' });
  ROWS.forEach((row, y) => {
    let x = 0;
    for (const k of row) {
      if (typeof k === 'number') { x += k; continue; }
      const [label, code, w = 1, tall] = k;
      const n = counts[code] || 0;
      const p = n ? Math.round(12 + 88 * Math.sqrt(n / max)) : 0; // the root, or a few keys take all the colour
      kb.append(h('div', {
        class: 'kb-key' + (p > 55 ? ' hot' : ''), title: `${NAMES.get(code)} · ${n.toLocaleString()}`,
        style: { gridColumn: `${x * 4 + 1} / span ${w * 4}`, gridRow: `${y + 1} / span ${tall ? 2 : 1}`, background: `color-mix(in srgb, var(--accent) ${p}%, var(--surface-2))` },
      }, label));
      x += w;
    }
  });
  return kb;
}

/** The keys pressed as [{ name, value }], most first, and what the rest add
 *  up to, keys not on the keyboard above included. */
export function topKeys(counts, n) {
  const keys = [];
  let rest = 0;
  for (const [code, value] of Object.entries(counts)) {
    const name = NAMES.get(+code);
    if (name) keys.push({ name, value });
    else rest += value;
  }
  keys.sort((a, b) => b.value - a.value);
  for (const k of keys.slice(n)) rest += k.value;
  return { top: keys.slice(0, n), rest };
}
