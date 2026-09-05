export type Theme = 'light' | 'dark';

const themeCookie = 'biotron-theme';
const themeCookieMaxAge = 60 * 60 * 24 * 365;
const sharedCookieDomains = ['biotron.ca', 'biotron-dev.com'];

function cookieDomain(hostname: string) {
  const domain = sharedCookieDomains.find(
    (candidate) => hostname === candidate || hostname.endsWith(`.${candidate}`),
  );
  return domain ? `; Domain=${domain}` : '';
}

export function storedTheme(): Theme | null {
  if (typeof document === 'undefined') return null;
  const cookie = document.cookie.match(/(?:^|;\s*)biotron-theme=(dark|light)(?:;|$)/);
  if (cookie) return cookie[1] as Theme;

  try {
    const legacy = localStorage.getItem('darkMode');
    if (legacy === 'true') return 'dark';
    if (legacy === 'false') return 'light';
  } catch {
    // Storage can be unavailable in private or hardened browser contexts.
  }
  return null;
}

function persistTheme(theme: Theme) {
  try {
    localStorage.setItem('darkMode', String(theme === 'dark'));
  } catch {
    // Storage can be unavailable in private or hardened browser contexts.
  }

  const secure = window.location.protocol === 'https:' ? '; Secure' : '';
  document.cookie = `${themeCookie}=${theme}; Path=/; Max-Age=${themeCookieMaxAge}; SameSite=Lax${cookieDomain(window.location.hostname)}${secure}`;
}

export function documentTheme(): Theme {
  if (typeof document === 'undefined') return 'light';
  return storedTheme() ?? (document.documentElement.classList.contains('dark') ? 'dark' : 'light');
}

export function applyTheme(theme: Theme, persist = true) {
  if (typeof document === 'undefined') return;
  const dark = theme === 'dark';
  const root = document.documentElement;
  root.classList.toggle('dark', dark);
  root.style.colorScheme = theme;
  // Must match --biotron-page in styles.css, and the literal each consumer's
  // index.html bootstrap paints before this module loads.
  root.style.backgroundColor = dark ? '#070b0e' : '#ffffff';
  if (persist) persistTheme(theme);
}
