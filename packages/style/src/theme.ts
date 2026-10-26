export type Theme = 'light' | 'dark';

const themeCookie = 'biotron-theme';
const themeCookieMaxAge = 60 * 60 * 24 * 365;

// The cookie belongs to the page's domain, its hostname's last two labels, so
// calendar.uwbiotron.ca and uwbiotron.ca share one preference. BioTron's
// domains never sit under a multi-part suffix such as co.uk, so two labels is
// enough. localhost, any other name without a dot, and an IP address get a
// host-only cookie; cookies ignore ports, so every local app still shares it.
function cookieDomain(hostname: string) {
  if (!hostname.includes('.') || hostname.startsWith('[') || /^[\d.]+$/.test(hostname)) return '';
  return `; Domain=${hostname.split('.').slice(-2).join('.')}`;
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
