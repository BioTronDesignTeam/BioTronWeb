export type Theme = 'light' | 'dark';

const themeCookie = 'biotron-theme';
const themeCookieMaxAge = 60 * 60 * 24 * 365;
const sharedCookieDomains = ['biotron.ca', 'biotron-dev.com', 'uwbiotron.dev'];
let hostCookieMigrated = false;

function cookieDomain(hostname: string) {
  const domain = sharedCookieDomains.find(
    (candidate) => hostname === candidate || hostname.endsWith(`.${candidate}`),
  );
  return domain ? `; Domain=${domain}` : '';
}

function readThemeCookie(): Theme | null {
  const cookie = document.cookie.match(/(?:^|;\s*)biotron-theme=(dark|light)(?:;|$)/);
  return cookie ? cookie[1] as Theme : null;
}

function clearHostThemeCookie() {
  const secure = window.location.protocol === 'https:' ? '; Secure' : '';
  document.cookie = `${themeCookie}=; Path=/; Max-Age=0${secure}`;
}

function writeThemeCookie(theme: Theme) {
  const secure = window.location.protocol === 'https:' ? '; Secure' : '';
  document.cookie = `${themeCookie}=${theme}; Path=/; Max-Age=${themeCookieMaxAge}; SameSite=Lax${cookieDomain(window.location.hostname)}${secure}`;
}

export function storedTheme(): Theme | null {
  if (typeof document === 'undefined') return null;
  let theme = readThemeCookie();
  if (cookieDomain(window.location.hostname) && !hostCookieMigrated) {
    hostCookieMigrated = true;
    if (theme) {
      // A former host-only cookie can shadow a sibling app's shared preference.
      // Remove it first, preferring the shared value that remains afterward.
      clearHostThemeCookie();
      const sharedTheme = readThemeCookie();
      if (sharedTheme) theme = sharedTheme;
      else writeThemeCookie(theme);
    }
  }
  if (theme) return theme;

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

  if (cookieDomain(window.location.hostname) && !hostCookieMigrated) {
    clearHostThemeCookie();
    hostCookieMigrated = true;
  }
  writeThemeCookie(theme);
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
