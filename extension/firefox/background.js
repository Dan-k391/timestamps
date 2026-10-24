// Timestamps browser extension: tells the Timestamps desktop app which page
// is open in the active tab. It only talks to 127.0.0.1 (this computer).
const api = globalThis.browser || globalThis.chrome;
const PORTS = [47613, 47614, 47615, 47616];
const HEADERS = { 'Content-Type': 'application/json', 'X-Timestamps-Ext': '1' };
let base = null;
let probing = null;

const brand = (() => {
  const ua = navigator.userAgent;
  if (/Edg\//.test(ua)) return 'edge';
  if (/OPR\//.test(ua)) return 'opera';
  if (/Vivaldi/.test(ua)) return 'vivaldi';
  if (/Firefox\//.test(ua)) return 'firefox';
  if (navigator.brave) return 'brave';
  return 'chrome';
})();

async function findBase() {
  for (const p of PORTS) {
    try {
      const r = await fetch(`http://127.0.0.1:${p}/ext/hello`, { headers: HEADERS });
      if (r.ok && (await r.json()).app === 'timestamps') return `http://127.0.0.1:${p}`;
    } catch (_) { /* app not running on this port */ }
  }
  return null;
}

async function send(tab) {
  if (!tab || !tab.url || !tab.active) return;
  if (!base) {
    probing = probing || findBase();
    base = await probing;
    probing = null;
    if (!base) return;
  }
  const body = JSON.stringify({
    browser: brand, window: tab.windowId, url: tab.url, title: tab.title || '', incognito: !!tab.incognito,
  });
  try {
    const r = await fetch(base + '/ext/report', { method: 'POST', headers: HEADERS, body });
    if (!r.ok) base = null;
  } catch (_) {
    base = null;
  }
}

async function reportActive() {
  try {
    const tabs = await api.tabs.query({ active: true });
    tabs.forEach(send);
  } catch (_) { /* ignore */ }
}

api.tabs.onActivated.addListener(({ tabId }) => {
  api.tabs.get(tabId).then(send, () => {});
});
api.tabs.onUpdated.addListener((_id, info, tab) => {
  if (tab.active && (info.url || info.title || info.status === 'complete')) send(tab);
});
api.windows.onFocusChanged.addListener(reportActive);
api.alarms.create('timestamps-beat', { periodInMinutes: 1 });
api.alarms.onAlarm.addListener(reportActive);
api.runtime.onStartup.addListener(reportActive);
api.runtime.onInstalled.addListener(reportActive);
reportActive();
