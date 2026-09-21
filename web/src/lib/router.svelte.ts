// Tiny history router built on Svelte 5 runes.
export type Params = Record<string, string>;

let path = $state(normalize(window.location.pathname));
let search = $state(window.location.search);

function normalize(p: string): string {
  if (p.length > 1 && p.endsWith('/')) p = p.slice(0, -1);
  return p || '/';
}

export const router = {
  get path() {
    return path;
  },
  get search() {
    return search;
  },
};

export function navigate(to: string, opts: { replace?: boolean } = {}): void {
  const url = new URL(to, window.location.origin);
  if (opts.replace) history.replaceState(null, '', url);
  else history.pushState(null, '', url);
  path = normalize(url.pathname);
  search = url.search;
  window.scrollTo({ top: 0 });
}

export function back(fallback = '/'): void {
  if (history.length > 1) history.back();
  else navigate(fallback, { replace: true });
}

/** Match `/groups/:id/expenses/:eid` style patterns. Returns params or null. */
export function match(pattern: string, p: string = path): Params | null {
  const a = pattern.split('/').filter(Boolean);
  const b = p.split('/').filter(Boolean);
  if (a.length !== b.length) return null;
  const params: Params = {};
  for (let i = 0; i < a.length; i++) {
    if (a[i].startsWith(':')) params[a[i].slice(1)] = decodeURIComponent(b[i]);
    else if (a[i] !== b[i]) return null;
  }
  return params;
}

window.addEventListener('popstate', () => {
  path = normalize(window.location.pathname);
  search = window.location.search;
});

// Intercept same-origin anchor clicks so <a href> works without full reloads.
document.addEventListener('click', (e) => {
  if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
  const a = (e.target as Element | null)?.closest?.('a');
  if (!a || a.target === '_blank' || a.hasAttribute('download') || a.getAttribute('rel') === 'external') return;
  const href = a.getAttribute('href');
  if (!href || href.startsWith('#') || href.startsWith('mailto:') || href.startsWith('tel:')) return;
  const url = new URL(href, window.location.origin);
  if (url.origin !== window.location.origin) return;
  e.preventDefault();
  navigate(url.pathname + url.search);
});
