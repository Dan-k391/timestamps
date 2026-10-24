// Hash router: #/view/arg?query
const handlers = [];

export function go(route, replace = false) {
  const hash = '#/' + route.replace(/^[#/]+/, '');
  if (replace) history.replaceState(null, '', hash);
  else if (location.hash !== hash) location.hash = hash;
  else return;
  if (replace) handlers.forEach((fn) => fn(parse()));
}

export function parse() {
  const raw = location.hash.replace(/^#\/?/, '');
  const [path, query = ''] = raw.split('?');
  const parts = path.split('/').filter(Boolean).map(decodeURIComponent);
  return { view: parts[0] || 'day', arg: parts[1] || '', params: Object.fromEntries(new URLSearchParams(query)) };
}

export function onRoute(fn) {
  handlers.push(fn);
  window.addEventListener('hashchange', () => fn(parse()));
}
