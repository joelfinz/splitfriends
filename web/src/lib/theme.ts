export type ThemePref = 'system' | 'light' | 'dark';
const KEY = 'fs_theme';
export const LIGHT = 'emerald';
export const DARK = 'dim';

export function getThemePref(): ThemePref {
  try {
    const v = localStorage.getItem(KEY);
    return v === 'light' || v === 'dark' ? v : 'system';
  } catch {
    return 'system';
  }
}

export function applyTheme(pref: ThemePref): void {
  const html = document.documentElement;
  if (pref === 'system') html.removeAttribute('data-theme');
  else html.setAttribute('data-theme', pref === 'light' ? LIGHT : DARK);
  try {
    if (pref === 'system') localStorage.removeItem(KEY);
    else localStorage.setItem(KEY, pref);
  } catch {
    /* ignore */
  }
}
