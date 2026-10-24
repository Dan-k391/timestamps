// Thin client for the local HTTP API.

async function request(method, path, body) {
  const opts = { method, headers: {}, credentials: 'same-origin' };
  if (method !== 'GET') {
    opts.headers['X-Requested-With'] = 'timestamps';
    opts.headers['Content-Type'] = 'application/json';
    opts.body = JSON.stringify(body ?? {});
  }
  const r = await fetch('/api/' + path, opts);
  if (r.status === 401) {
    location.reload();
    throw new Error('session expired');
  }
  const ct = r.headers.get('content-type') || '';
  if (!r.ok) {
    let msg = r.statusText;
    if (ct.includes('json')) msg = (await r.json()).error || msg;
    throw new Error(msg);
  }
  if (r.status === 204) return null;
  return ct.includes('json') ? r.json() : r;
}

const qs = (o) => new URLSearchParams(Object.entries(o).filter(([, v]) => v !== undefined && v !== null && v !== '')).toString();

export const get = (path, params) => request('GET', params ? `${path}?${qs(params)}` : path);
export const post = (path, body) => request('POST', path, body);

const dayCache = new Map();

/** Day data with ETag revalidation so unchanged days cost nothing. */
export async function day(date) {
  const cached = dayCache.get(date);
  const headers = cached ? { 'If-None-Match': cached.etag } : {};
  const r = await fetch('/api/day?' + qs({ date }), { headers, credentials: 'same-origin' });
  if (r.status === 304 && cached) return cached.data;
  if (r.status === 401) { location.reload(); throw new Error('session expired'); }
  if (!r.ok) throw new Error((await r.json()).error || r.statusText);
  const data = await r.json();
  if (dayCache.size > 40) dayCache.clear();
  dayCache.set(date, { etag: r.headers.get('ETag'), data });
  return data;
}

export const iconURL = (id, v) => `/api/icon?id=${id}${v ? '&v=' + v : ''}`;
export const siteIconURL = (host) => `/api/siteicon?d=${encodeURIComponent(host)}`;
export const shotURL = (ts, thumb) => `/api/shot?ts=${ts}${thumb ? '&thumb=1' : ''}`;

/** Download a server-generated file. */
export async function download(path, params, body) {
  const r = body
    ? await fetch('/api/' + path, { method: 'POST', headers: { 'X-Requested-With': 'timestamps', 'Content-Type': 'application/json' }, body: JSON.stringify(body) })
    : await fetch('/api/' + path + '?' + qs(params));
  if (!r.ok) throw new Error(r.statusText);
  const cd = r.headers.get('content-disposition') || '';
  const name = decodeURIComponent((cd.match(/filename="([^"]+)"/) || [])[1] || 'export');
  const blob = await r.blob();
  const a = document.createElement('a');
  a.href = URL.createObjectURL(blob);
  a.download = name;
  document.body.appendChild(a);
  a.click();
  setTimeout(() => { URL.revokeObjectURL(a.href); a.remove(); }, 2000);
}

/** Server-sent events with automatic reconnect. */
export function events(onEvent) {
  let es;
  const open = () => {
    es = new EventSource('/api/events');
    es.onmessage = (e) => { try { onEvent(JSON.parse(e.data)); } catch (_) { /* ignore */ } };
    es.onerror = () => { es.close(); setTimeout(open, 3000); };
  };
  open();
  return () => es && es.close();
}
