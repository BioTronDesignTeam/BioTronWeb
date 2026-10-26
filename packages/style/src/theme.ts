export type Theme = 'light' | 'dark';

const themeCookie = 'biotron-theme';
const themeCookieMaxAge = 60 * 60 * 24 * 365;

// Staging and production. Anywhere else, such as localhost, the cookie is
// host-only; cookies ignore ports, so every local app still shares it.
const sharedCookieDomains = ['uwbiotron.dev', 'uwbiotron.ca'];

function cookieDomain(hostname: string) {
  const domain = sharedCookieDomains.find(
    (candidate) => hostname === candidate || hostname.endsWith(`.${candidate}`),
  );
  return domain ? `; Domain=${domain}` : '';
}

// Reads the cookie as the bootstrap in every index.html does.
export function storedTheme(): Theme | null {
  if (typeof document === 'undefined') return null;
  const cookie = document.cookie.match(/(?:^|;\s*)biotron-theme=(dark|light)(?:;|$)/);
  return cookie ? cookie[1] as Theme : null;
}

function persistTheme(theme: Theme) {
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
