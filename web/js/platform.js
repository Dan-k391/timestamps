// The computer Timestamps runs on: the UI is always served from it.
export const isMac = /Mac/.test(navigator.platform || navigator.userAgent);

/** Whether the shortcut modifier is down: ⌘ on a Mac, Ctrl elsewhere. */
export const mod = (e) => (isMac ? e.metaKey : e.ctrlKey);

/** A shortcut label the platform's way: "Ctrl+C" becomes "⌘C" on a Mac. */
export const keys = (s) => (isMac ? s.replace(/Ctrl ?\+ ?/g, '⌘') : s);

/** Picks the Mac or the Windows wording. */
export const byOS = (mac, win) => (isMac ? mac : win);
