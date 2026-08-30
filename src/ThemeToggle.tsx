import { useEffect, useState } from 'react';

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

function documentTheme(): Theme {
  if (typeof document === 'undefined') return 'light';
  return storedTheme() ?? (document.documentElement.classList.contains('dark') ? 'dark' : 'light');
}

export function applyTheme(theme: Theme, persist = true) {
  if (typeof document === 'undefined') return;
  const dark = theme === 'dark';
  const root = document.documentElement;
  root.classList.toggle('dark', dark);
  root.style.colorScheme = theme;
  root.style.backgroundColor = dark ? '#16033c' : '#ffffff';
  if (persist) persistTheme(theme);
}

const SunIcon = () => (
  <svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
    <path d="M12 17a5 5 0 1 0 0-10 5 5 0 0 0 0 10Zm0-13a1 1 0 0 1 1 1v1a1 1 0 1 1-2 0V5a1 1 0 0 1 1-1Zm0 14a1 1 0 0 1 1 1v1a1 1 0 1 1-2 0v-1a1 1 0 0 1 1-1ZM5 11a1 1 0 1 1 0 2H4a1 1 0 1 1 0-2h1Zm15 0a1 1 0 1 1 0 2h-1a1 1 0 1 1 0-2h1ZM6.34 6.34a1 1 0 0 1 1.41 0l.71.71A1 1 0 0 1 7.05 8.46l-.71-.71a1 1 0 0 1 0-1.41Zm9.9 9.9a1 1 0 0 1 1.41 0l.71.71a1 1 0 0 1-1.41 1.41l-.71-.71a1 1 0 0 1 0-1.41ZM17.66 6.34a1 1 0 0 1 0 1.41l-.71.71a1 1 0 1 1-1.41-1.41l.71-.71a1 1 0 0 1 1.41 0ZM7.76 16.24a1 1 0 0 1 0 1.41l-.71.71a1 1 0 0 1-1.41-1.41l.71-.71a1 1 0 0 1 1.41 0Z" />
  </svg>
);

const MoonIcon = () => (
  <svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
    <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79Z" />
  </svg>
);

export type ThemeToggleProps = {
  className?: string;
};

export function ThemeToggle({ className = '' }: ThemeToggleProps) {
  const [theme, setTheme] = useState<Theme>(documentTheme);

  useEffect(() => {
    applyTheme(theme);
  }, [theme]);

  useEffect(() => {
    const synchronize = () => {
      const stored = storedTheme();
      if (stored) setTheme((current) => current === stored ? current : stored);
    };
    const onStorage = (event: StorageEvent) => {
      if (event.key === 'darkMode') synchronize();
    };
    const onVisibilityChange = () => {
      if (document.visibilityState === 'visible') synchronize();
    };
    const interval = window.setInterval(synchronize, 1000);
    window.addEventListener('focus', synchronize);
    window.addEventListener('storage', onStorage);
    document.addEventListener('visibilitychange', onVisibilityChange);
    return () => {
      window.clearInterval(interval);
      window.removeEventListener('focus', synchronize);
      window.removeEventListener('storage', onStorage);
      document.removeEventListener('visibilitychange', onVisibilityChange);
    };
  }, []);

  const dark = theme === 'dark';
  return (
    <button
      type="button"
      role="switch"
      aria-checked={dark}
      aria-label={`Color theme: ${theme} mode`}
      className={`biotron-theme-toggle ${dark ? 'biotron-theme-toggle--dark' : ''} ${className}`.trim()}
      onClick={() => setTheme(dark ? 'light' : 'dark')}
    >
      <span className="biotron-theme-toggle__thumb" aria-hidden="true" />
      <span className="biotron-theme-toggle__option"><SunIcon /></span>
      <span className="biotron-theme-toggle__option"><MoonIcon /></span>
    </button>
  );
}
